/*
Copyright 2018 The Kubernetes Authors.

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

package admission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"k8s.io/api/admission/v1beta1"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

var admissionv1beta1scheme = runtime.NewScheme()
var admissionv1beta1schemecodecs = serializer.NewCodecFactory(admissionv1beta1scheme)

func init() {
	addToScheme(admissionv1beta1scheme)
}

func addToScheme(scheme *runtime.Scheme) {
	err := admissionv1beta1.AddToScheme(scheme)
	// TODO: switch to use Must in
	// https://github.com/kubernetes/kubernetes/blob/fbb2dfcc6ad345b0ca3fe09cb4bc2a23ec0781d5/staging/src/k8s.io/apimachinery/pkg/util/runtime/runtime.go#L164-L169
	// after the apimachinery repo in vendor has been updated to 1.11 or later.
	if err != nil {
		panic(err)
	}
}

var _ http.Handler = &Webhook{}

func (wh *Webhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body []byte
	var err error

	var reviewResponse Response
	if r.Body != nil {
		if body, err = ioutil.ReadAll(r.Body); err != nil {
			log.Error(err, "unable to read the body from the incoming request")
			reviewResponse = ErrorResponse(http.StatusBadRequest, err)
			writeResponse(w, reviewResponse)
			return
		}
	} else {
		err = errors.New("request body is empty")
		log.Error(err, "bad request")
		reviewResponse = ErrorResponse(http.StatusBadRequest, err)
		writeResponse(w, reviewResponse)
		return
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		err = fmt.Errorf("contentType=%s, expect application/json", contentType)
		log.Error(err, "unable to process a request with an unknown content type")
		reviewResponse = ErrorResponse(http.StatusBadRequest, err)
		writeResponse(w, reviewResponse)
		return
	}

	ar := v1beta1.AdmissionReview{}
	if _, _, err := admissionv1beta1schemecodecs.UniversalDeserializer().Decode(body, nil, &ar); err != nil {
		log.Error(err, "unable to decode the request")
		reviewResponse = ErrorResponse(http.StatusBadRequest, err)
		writeResponse(w, reviewResponse)
		return
	}

	// TODO: add panic-recovery for Handle
	type contextKey string
	ctx := context.Background()
	for k := range wh.KVMap {
		ctx = context.WithValue(ctx, contextKey(k), wh.KVMap[k])
	}
	reviewResponse = wh.Handle(ctx, Request{AdmissionRequest: ar.Request})
	reviewResponse.Response.UID = ar.Request.UID
	writeResponse(w, reviewResponse)
}

func writeResponse(w io.Writer, response Response) {
	encoder := json.NewEncoder(w)
	responseAdmissionReview := v1beta1.AdmissionReview{
		Response: response.Response,
	}
	err := encoder.Encode(responseAdmissionReview)
	if err != nil {
		log.Error(err, "unable to encode the response")
		writeResponse(w, ErrorResponse(http.StatusInternalServerError, err))
	}
}
