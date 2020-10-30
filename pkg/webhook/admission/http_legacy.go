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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"k8s.io/api/admission/v1beta1"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

func init() {
	utilruntime.Must(admissionv1beta1.AddToScheme(admissionScheme))
}

var _ http.Handler = &WebhookLegacy{}

func (wh *WebhookLegacy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body []byte
	var err error

	var reviewResponse ResponseLegacy
	if r.Body != nil {
		if body, err = ioutil.ReadAll(r.Body); err != nil {
			wh.log.Error(err, "unable to read the body from the incoming request")
			reviewResponse = ErroredLegacy(http.StatusBadRequest, err)
			wh.writeResponseLegacy(w, reviewResponse)
			return
		}
	} else {
		err = errors.New("request body is empty")
		wh.log.Error(err, "bad request")
		reviewResponse = ErroredLegacy(http.StatusBadRequest, err)
		wh.writeResponseLegacy(w, reviewResponse)
		return
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		err = fmt.Errorf("contentType=%s, expected application/json", contentType)
		wh.log.Error(err, "unable to process a request with an unknown content type", "content type", contentType)
		reviewResponse = ErroredLegacy(http.StatusBadRequest, err)
		wh.writeResponseLegacy(w, reviewResponse)
		return
	}

	req := RequestLegacy{}
	ar := v1beta1.AdmissionReview{
		// avoid an extra copy
		Request: &req.AdmissionRequest,
	}
	if _, _, err := admissionCodecs.UniversalDeserializer().Decode(body, nil, &ar); err != nil {
		wh.log.Error(err, "unable to decode the request")
		reviewResponse = ErroredLegacy(http.StatusBadRequest, err)
		wh.writeResponseLegacy(w, reviewResponse)
		return
	}
	wh.log.V(1).Info("received request", "UID", req.UID, "kind", req.Kind, "resource", req.Resource)

	// TODO: add panic-recovery for Handle
	reviewResponse = wh.Handle(r.Context(), req)
	wh.writeResponseLegacy(w, reviewResponse)
}

func (wh *WebhookLegacy) writeResponseLegacy(w io.Writer, response ResponseLegacy) {
	encoder := json.NewEncoder(w)
	responseAdmissionReview := v1beta1.AdmissionReview{
		Response: &response.AdmissionResponse,
	}
	err := encoder.Encode(responseAdmissionReview)
	if err != nil {
		wh.log.Error(err, "unable to encode the response")
		wh.writeResponseLegacy(w, ErroredLegacy(http.StatusInternalServerError, err))
	} else {
		res := responseAdmissionReview.Response
		if log := wh.log; log.V(1).Enabled() {
			if res.Result != nil {
				log = log.WithValues("code", res.Result.Code, "reason", res.Result.Reason)
			}
			log.V(1).Info("wrote response", "UID", res.UID, "allowed", res.Allowed)
		}
	}
}
