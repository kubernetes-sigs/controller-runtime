/*
Copyright 2022 The Kubernetes Authors.

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
	goerrors "errors"
	"net/http"

	v1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

// ValidatorWarn works like Validator, but it allows to return warnings.
type ValidatorWarn interface {
	runtime.Object
	ValidateCreate() (err error, warnings []string)
	ValidateUpdate(old runtime.Object) (err error, warnings []string)
	ValidateDelete() (err error, warnings []string)
}

func ValidatingWebhookWithWarningFor(validatorWarning ValidatorWarn) *Webhook {
	return nil
}

var _ Handler = &validatingWarnHandler{}
var _ DecoderInjector = &validatingWarnHandler{}

type validatingWarnHandler struct {
	validatorWarn ValidatorWarn
	decoder       *Decoder
}

// InjectDecoder injects the decoder into a validatingWarnHandler.
func (h *validatingWarnHandler) InjectDecoder(decoder *Decoder) error {
	h.decoder = decoder
	return nil
}

// Handle handles admission requests.
func (h *validatingWarnHandler) Handle(ctx context.Context, req Request) Response {
	if h.validatorWarn == nil {
		panic("validatorWarn should never be nil")
	}

	var allWarnings []string

	// Get the object in the request
	obj := h.validatorWarn.DeepCopyObject().(ValidatorWarn)
	if req.Operation == v1.Create {
		err := h.decoder.Decode(req, obj)
		if err != nil {
			return Errored(http.StatusBadRequest, err)
		}

		err, warnings := obj.ValidateCreate()
		allWarnings = append(allWarnings, warnings...)
		if err != nil {
			var apiStatus apierrors.APIStatus
			if goerrors.As(err, &apiStatus) {
				return validationResponseFromStatus(false, apiStatus.Status())
			}
			return Denied(err.Error()).WithWarnings(allWarnings...)
		}
	}

	if req.Operation == v1.Update {
		oldObj := obj.DeepCopyObject()

		err := h.decoder.DecodeRaw(req.Object, obj)
		if err != nil {
			return Errored(http.StatusBadRequest, err)
		}
		err = h.decoder.DecodeRaw(req.OldObject, oldObj)
		if err != nil {
			return Errored(http.StatusBadRequest, err)
		}
		err, warnings := obj.ValidateUpdate(oldObj)
		allWarnings = append(allWarnings, warnings...)
		if err != nil {
			var apiStatus apierrors.APIStatus
			if goerrors.As(err, &apiStatus) {
				return validationResponseFromStatus(false, apiStatus.Status())
			}
			return Denied(err.Error()).WithWarnings(allWarnings...)
		}
	}

	if req.Operation == v1.Delete {
		// In reference to PR: https://github.com/kubernetes/kubernetes/pull/76346
		// OldObject contains the object being deleted
		err := h.decoder.DecodeRaw(req.OldObject, obj)
		if err != nil {
			return Errored(http.StatusBadRequest, err)
		}

		err, warnings := obj.ValidateDelete()
		allWarnings = append(allWarnings, warnings...)
		if err != nil {
			var apiStatus apierrors.APIStatus
			if goerrors.As(err, &apiStatus) {
				return validationResponseFromStatus(false, apiStatus.Status())
			}
			return Denied(err.Error()).WithWarnings(allWarnings...)
		}
	}
	return Allowed("").WithWarnings(allWarnings...)
}
