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
	"net/http"

	goerrors "errors"

	"k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
)

// ValidatingWebhookForLegacy creates a new Webhook for validating the provided type.
func ValidatingWebhookForLegacy(validator Validator) *WebhookLegacy {
	return &WebhookLegacy{
		Handler: &validatingHandlerLegacy{validator: validator},
	}
}

type validatingHandlerLegacy struct {
	validator Validator
	decoder   *Decoder
}

var _ DecoderInjector = &validatingHandlerLegacy{}

// InjectDecoder injects the decoder into a validatingHandler.
func (h *validatingHandlerLegacy) InjectDecoder(d *Decoder) error {
	h.decoder = d
	return nil
}

// Handle handles admission requests.
func (h *validatingHandlerLegacy) Handle(ctx context.Context, req RequestLegacy) ResponseLegacy {
	if h.validator == nil {
		panic("validator should never be nil")
	}

	// Get the object in the request
	obj := h.validator.DeepCopyObject().(Validator)
	if req.Operation == v1beta1.Create {
		err := h.decoder.DecodeLegacy(req, obj)
		if err != nil {
			return ErroredLegacy(http.StatusBadRequest, err)
		}

		err = obj.ValidateCreate()
		if err != nil {
			var apiStatus errors.APIStatus
			if goerrors.As(err, &apiStatus) {
				return validationResponseFromStatusLegacy(false, apiStatus.Status())
			}
			return DeniedLegacy(err.Error())
		}
	}

	if req.Operation == v1beta1.Update {
		oldObj := obj.DeepCopyObject()

		err := h.decoder.DecodeRaw(req.Object, obj)
		if err != nil {
			return ErroredLegacy(http.StatusBadRequest, err)
		}
		err = h.decoder.DecodeRaw(req.OldObject, oldObj)
		if err != nil {
			return ErroredLegacy(http.StatusBadRequest, err)
		}

		err = obj.ValidateUpdate(oldObj)
		if err != nil {
			var apiStatus errors.APIStatus
			if goerrors.As(err, &apiStatus) {
				return validationResponseFromStatusLegacy(false, apiStatus.Status())
			}
			return DeniedLegacy(err.Error())
		}
	}

	if req.Operation == v1beta1.Delete {
		// In reference to PR: https://github.com/kubernetes/kubernetes/pull/76346
		// OldObject contains the object being deleted
		err := h.decoder.DecodeRaw(req.OldObject, obj)
		if err != nil {
			return ErroredLegacy(http.StatusBadRequest, err)
		}

		err = obj.ValidateDelete()
		if err != nil {
			var apiStatus errors.APIStatus
			if goerrors.As(err, &apiStatus) {
				return validationResponseFromStatusLegacy(false, apiStatus.Status())
			}
			return DeniedLegacy(err.Error())
		}
	}

	return AllowedLegacy("")
}
