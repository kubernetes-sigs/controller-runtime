/*
Copyright 2021 The Kubernetes Authors.

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

package authorization

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	authorizationv1 "k8s.io/api/authorization/v1"
	authorizationv1beta1 "k8s.io/api/authorization/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

var authorizationScheme = runtime.NewScheme()
var authorizationCodecs = serializer.NewCodecFactory(authorizationScheme)

func init() {
	utilruntime.Must(authorizationv1.AddToScheme(authorizationScheme))
	utilruntime.Must(authorizationv1beta1.AddToScheme(authorizationScheme))
}

var _ http.Handler = &Webhook{}

func (wh *Webhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body []byte
	var err error
	ctx := r.Context()
	if wh.WithContextFunc != nil {
		ctx = wh.WithContextFunc(ctx, r)
	}

	var reviewResponse Response
	if r.Body == nil {
		err = errors.New("request body is empty")
		wh.log.Error(err, "bad request")
		reviewResponse = Errored(err)
		wh.writeResponse(w, nil, reviewResponse)
		return
	}

	defer r.Body.Close()
	if body, err = ioutil.ReadAll(r.Body); err != nil {
		wh.log.Error(err, "unable to read the body from the incoming request")
		reviewResponse = Errored(err)
		wh.writeResponse(w, nil, reviewResponse)
		return
	}

	// verify the content type is accurate
	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		err = fmt.Errorf("contentType=%s, expected application/json", contentType)
		wh.log.Error(err, "unable to process a request with an unknown content type", "content type", contentType)
		reviewResponse = Errored(err)
		wh.writeResponse(w, nil, reviewResponse)
		return
	}

	// Decode request body into authorizationv1.SubjectAccessReviewSpec structure
	sar, actualTokRevGVK, err := wh.decodeRequestBody(body)
	if err != nil {
		wh.log.Error(err, "unable to decode the request")
		reviewResponse = Errored(err)
		wh.writeResponse(w, actualTokRevGVK, reviewResponse)
		return
	}
	wh.log.V(1).Info("received request", "UID", sar.UID, "kind", sar.Kind)

	reviewResponse = wh.Handle(ctx, Request(sar))
	wh.writeResponse(w, actualTokRevGVK, reviewResponse)
}

func (wh *Webhook) writeResponse(w io.Writer, gvk *schema.GroupVersionKind, response Response) {
	ar := response.SubjectAccessReview

	// Default to a v1 SubjectAccessReview, otherwise the API server may not recognize the request
	// if multiple SubjectAccessReview versions are permitted by the webhook config.
	if gvk == nil || *gvk == (schema.GroupVersionKind{}) {
		ar.SetGroupVersionKind(authorizationv1.SchemeGroupVersion.WithKind("SubjectAccessReview"))
	} else {
		ar.SetGroupVersionKind(*gvk)
	}

	if err := json.NewEncoder(w).Encode(ar); err != nil {
		wh.log.Error(err, "unable to encode the response")
		wh.writeResponse(w, gvk, Errored(err))
		return
	}

	wh.log.
		V(1).
		WithValues(
			"uid", ar.UID,
			"allowed", ar.Status.Allowed,
			"denied", ar.Status.Denied,
			"reason", ar.Status.Reason,
			"error", ar.Status.EvaluationError,
		).
		Info("wrote response")
}

func (wh *Webhook) decodeRequestBody(body []byte) (unversionedSubjectAccessReview, *schema.GroupVersionKind, error) {
	// v1 and v1beta1 SubjectAccessReview types are almost exactly the same (the only difference is the JSON key for the
	// 'Groups' field).The v1beta1 api is deprecated as of 1.19 and will be removed in authorization as of v1.22. We
	// decode the object into a v1 type and "manually" convert the 'Groups' field (see below).
	// However, the runtime codec's decoder guesses which type to decode into by type name if an Object's TypeMeta
	// isn't set. By setting TypeMeta of an unregistered type to the v1 GVK, the decoder will coerce a v1beta1
	// SubjectAccessReview to v1.
	var obj unversionedSubjectAccessReview
	obj.SetGroupVersionKind(authorizationv1.SchemeGroupVersion.WithKind("SubjectAccessReview"))

	_, gvk, err := authorizationCodecs.UniversalDeserializer().Decode(body, nil, &obj)
	if err != nil {
		return obj, nil, err
	}
	if gvk == nil {
		return obj, nil, fmt.Errorf("could not determine GVK for object in the request body")
	}

	// The only difference in v1beta1 is that the JSON key name of the 'Groups' field is different. Hence, when we
	// detect that v1beta1 was sent, we decode it once again into the "correct" type and manually "convert" the 'Groups'
	// information.
	switch *gvk {
	case authorizationv1beta1.SchemeGroupVersion.WithKind("SubjectAccessReview"):
		var tmp authorizationv1beta1.SubjectAccessReview
		if _, _, err := authorizationCodecs.UniversalDeserializer().Decode(body, nil, &tmp); err != nil {
			return obj, gvk, err
		}
		obj.Spec.Groups = tmp.Spec.Groups
	}

	return obj, gvk, nil
}

// unversionedSubjectAccessReview is used to decode both v1 and v1beta1 SubjectAccessReview types.
type unversionedSubjectAccessReview struct {
	authorizationv1.SubjectAccessReview
}

var _ runtime.Object = &unversionedSubjectAccessReview{}
