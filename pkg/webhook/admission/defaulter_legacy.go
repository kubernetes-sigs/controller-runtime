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
	"net/http"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
)

// DefaultingWebhookForLegacy creates a new Webhook for Defaulting the provided type.
func DefaultingWebhookForLegacy(defaulter Defaulter) *WebhookLegacy {
	return &WebhookLegacy{
		Handler: &mutatingHandlerLegacy{defaulter: defaulter},
	}
}

type mutatingHandlerLegacy struct {
	defaulter Defaulter
	decoder   *Decoder
}

var _ DecoderInjector = &mutatingHandlerLegacy{}

// InjectDecoder injects the decoder into a mutatingHandler.
func (h *mutatingHandlerLegacy) InjectDecoder(d *Decoder) error {
	h.decoder = d
	return nil
}

// Handle handles admission requests.
func (h *mutatingHandlerLegacy) Handle(ctx context.Context, req RequestLegacy) ResponseLegacy {
	if h.defaulter == nil {
		panic("defaulter should never be nil")
	}

	// Get the object in the request
	obj := h.defaulter.DeepCopyObject().(Defaulter)
	err := h.decoder.DecodeLegacy(req, obj)
	if err != nil {
		return ErroredLegacy(http.StatusBadRequest, err)
	}

	// Default the object
	obj.Default()
	marshalled, err := json.Marshal(obj)
	if err != nil {
		return ErroredLegacy(http.StatusInternalServerError, err)
	}

	// Create the patch
	return PatchResponseFromRawLegacy(req.Object.Raw, marshalled)
}

type multiValidatingLegacy []HandlerLegacy

func (hs multiValidatingLegacy) Handle(ctx context.Context, req RequestLegacy) ResponseLegacy {
	for _, handler := range hs {
		resp := handler.Handle(ctx, req)
		if !resp.Allowed {
			return resp
		}
	}
	return ResponseLegacy{
		AdmissionResponse: admissionv1beta1.AdmissionResponse{
			Allowed: true,
			Result: &metav1.Status{
				Code: http.StatusOK,
			},
		},
	}
}

// MultiValidatingHandlerLegacy combines multiple validating webhook handlers into a single
// validating webhook handler.  Handlers are called in sequential order, and the first
// `allowed: false`	response may short-circuit the rest.
func MultiValidatingHandlerLegacy(handlers ...HandlerLegacy) HandlerLegacy {
	return multiValidatingLegacy(handlers)
}

// InjectFunc injects the field setter into the handlers.
func (hs multiValidatingLegacy) InjectFunc(f inject.Func) error {
	// inject directly into the handlers.  It would be more correct
	// to do this in a sync.Once in Handle (since we don't have some
	// other start/finalize-type method), but it's more efficient to
	// do it here, presumably.
	for _, handler := range hs {
		if err := f(handler); err != nil {
			return err
		}
	}

	return nil
}
