/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package conversion

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	apix "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	log = logf.Log.WithName("conversion_webhook")
)

// Webhook implements a CRD conversion webhook HTTP handler.
type Webhook struct {
	scheme  *runtime.Scheme
	decoder *Decoder
}

// InjectScheme injects a scheme into the webhook, in order to construct a Decoder.
func (wh *Webhook) InjectScheme(s *runtime.Scheme) error {
	var err error
	wh.scheme = s
	wh.decoder, err = NewDecoder(s)
	if err != nil {
		return err
	}

	// inject the decoder here too, just in case the order of calling this is not
	// scheme first, then inject func
	// if w.Handler != nil {
	// 	if _, err := InjectDecoderInto(w.GetDecoder(), w.Handler); err != nil {
	// 		return err
	// 	}
	// }

	return nil
}

// ensure Webhook implements http.Handler
var _ http.Handler = &Webhook{}

func (wh *Webhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	convertReview, err := wh.readRequest(r)
	if err != nil {
		log.Error(err, "failed to read conversion request")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// TODO(droot): may be move the conversion logic to a separate module to
	// decouple it from the http layer ?
	resp, err := wh.handleConvertRequest(convertReview.Request)
	if err != nil {
		log.Error(err, "failed to convert", "request", convertReview.Request.UID)
		convertReview.Response = errored(err)
		convertReview.Response.UID = convertReview.Request.UID
	} else {
		convertReview.Response = resp
	}

	err = json.NewEncoder(w).Encode(convertReview)
	if err != nil {
		log.Error(err, "failed to write response")
		return
	}
}

func (wh *Webhook) readRequest(r *http.Request) (*apix.ConversionReview, error) {

	var body []byte
	if r.Body == nil {
		return nil, fmt.Errorf("nil request body")
	}
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	body = data

	convertReview := &apix.ConversionReview{}
	// TODO(droot): figure out if we want to split decoder for conversion
	// request from the objects contained in the request
	err = wh.decoder.DecodeInto(body, convertReview)
	if err != nil {
		return nil, err
	}
	return convertReview, nil
}

// handles a version conversion request.
func (wh *Webhook) handleConvertRequest(req *apix.ConversionRequest) (*apix.ConversionResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("conversion request is nil")
	}
	var objects []runtime.RawExtension

	for _, obj := range req.Objects {
		src, gvk, err := wh.decoder.Decode(obj.Raw)
		if err != nil {
			return nil, err
		}
		dst, err := wh.allocateDstObject(req.DesiredAPIVersion, gvk.Kind)
		if err != nil {
			return nil, err
		}
		err = wh.convertObject(src, dst)
		if err != nil {
			return nil, err
		}
		objects = append(objects, runtime.RawExtension{Object: dst})
	}
	return &apix.ConversionResponse{
		UID:              req.UID,
		ConvertedObjects: objects,
	}, nil
}

// convertObject will convert given a src object to dst object.
// Note(droot): couldn't find a way to reduce the cyclomatic complexity under 10
// without compromising readability, so disabling gocyclo linter
// nolint: gocyclo
func (wh *Webhook) convertObject(src, dst runtime.Object) error {
	srcGVK := src.GetObjectKind().GroupVersionKind()
	dstGVK := dst.GetObjectKind().GroupVersionKind()

	if srcGVK.GroupKind().String() != dstGVK.GroupKind().String() {
		return fmt.Errorf("src %T and dst %T does not belong to same API Group", src, dst)
	}

	if srcGVK.String() == dstGVK.String() {
		return fmt.Errorf("conversion is not allowed between same type %T", src)
	}

	srcIsHub, dstIsHub := isHub(src), isHub(dst)
	srcIsConvertible, dstIsConvertible := isConvertible(src), isConvertible(dst)

	if srcIsHub {
		if dstIsConvertible {
			return dst.(conversion.Convertible).ConvertFrom(src.(conversion.Hub))
		}
		return fmt.Errorf("%T is not convertible to %T", src, dst)
	}

	if dstIsHub {
		if srcIsConvertible {
			return src.(conversion.Convertible).ConvertTo(dst.(conversion.Hub))
		}
		return fmt.Errorf("%T is not convertible %T", dst, src)
	}

	// neither src nor dst are Hub, means both of them are spoke, so lets get the hub
	// version type.
	hub, err := wh.getHub(src)
	if err != nil {
		return err
	}

	if hub == nil {
		return fmt.Errorf("API Group %s does not have any Hub defined", srcGVK)
	}

	// src and dst needs to be convertable for it to work
	if !srcIsConvertible || !dstIsConvertible {
		return fmt.Errorf("%T and %T needs to be convertable", src, dst)
	}

	err = src.(conversion.Convertible).ConvertTo(hub)
	if err != nil {
		return fmt.Errorf("%T failed to convert to hub version %T : %v", src, hub, err)
	}

	err = dst.(conversion.Convertible).ConvertFrom(hub)
	if err != nil {
		return fmt.Errorf("%T failed to convert from hub version %T : %v", dst, hub, err)
	}

	return nil
}

// getHub returns an instance of the Hub for passed-in object's group/kind.
func (wh *Webhook) getHub(obj runtime.Object) (conversion.Hub, error) {
	gvks, _, err := wh.scheme.ObjectKinds(obj)
	if err != nil {
		return nil, fmt.Errorf("error retriving object kinds for given object : %v", err)
	}

	var hub conversion.Hub
	var isHub, hubFoundAlready bool
	for _, gvk := range gvks {
		instance, err := wh.scheme.New(gvk)
		if err != nil {
			return nil, fmt.Errorf("failed to allocate an instance for gvk %v %v", gvk, err)
		}
		if hub, isHub = instance.(conversion.Hub); isHub {
			if hubFoundAlready {
				return nil, fmt.Errorf("multiple hub version defined for %T", obj)
			}
			hubFoundAlready = true
		}
	}
	return hub, nil
}

// allocateDstObject returns an instance for a given GVK.
func (wh *Webhook) allocateDstObject(apiVersion, kind string) (runtime.Object, error) {
	gvk := schema.FromAPIVersionAndKind(apiVersion, kind)

	obj, err := wh.scheme.New(gvk)
	if err != nil {
		return obj, err
	}

	t, err := meta.TypeAccessor(obj)
	if err != nil {
		return obj, err
	}

	t.SetAPIVersion(apiVersion)
	t.SetKind(kind)

	return obj, nil
}

// isHub determines if passed-in object is a Hub or not.
func isHub(obj runtime.Object) bool {
	_, yes := obj.(conversion.Hub)
	return yes
}

// isConvertible determines if passed-in object is a convertible.
func isConvertible(obj runtime.Object) bool {
	_, yes := obj.(conversion.Convertible)
	return yes
}
