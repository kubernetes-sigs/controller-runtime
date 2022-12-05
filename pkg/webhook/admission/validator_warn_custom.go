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
	"errors"
	"fmt"
	"net/http"

	v1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

// CustomValidatorWarn works like CustomValidator, but it allows to return warnings.
type CustomValidatorWarn interface {
	ValidateCreate(ctx context.Context, obj runtime.Object) (warnings []string, err error)
	ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (warnings []string, err error)
	ValidateDelete(ctx context.Context, obj runtime.Object) (warnings []string, err error)
}

// WithCustomValidatorWarn creates a new Webhook for validating the provided type.
func WithCustomValidatorWarn(obj runtime.Object, validatorWarn CustomValidatorWarn) *Webhook {
	return &Webhook{
		Handler: &validatorWarnForType{object: obj, validatorWarn: validatorWarn},
	}
}

var _ Handler = (*validatorWarnForType)(nil)
var _ DecoderInjector = (*validatorWarnForType)(nil)

type validatorWarnForType struct {
	validatorWarn CustomValidatorWarn
	object        runtime.Object
	decoder       *Decoder
}

func (h *validatorWarnForType) InjectDecoder(d *Decoder) error {
	h.decoder = d
	return nil
}

func (h *validatorWarnForType) Handle(ctx context.Context, req Request) Response {
	if h.validatorWarn == nil {
		panic("validatorWarn should never be nil")
	}
	if h.object == nil {
		panic("object should never be nil")
	}

	ctx = NewContextWithRequest(ctx, req)

	obj := h.object.DeepCopyObject()

	var err error
	var warnings []string
	switch req.Operation {
	case v1.Create:
		if err := h.decoder.Decode(req, obj); err != nil {
			return Errored(http.StatusBadRequest, err)
		}

		warnings, err = h.validatorWarn.ValidateCreate(ctx, obj)
	case v1.Update:
		oldObj := obj.DeepCopyObject()
		if err := h.decoder.DecodeRaw(req.Object, obj); err != nil {
			return Errored(http.StatusBadRequest, err)
		}
		if err := h.decoder.DecodeRaw(req.OldObject, oldObj); err != nil {
			return Errored(http.StatusBadRequest, err)
		}

		warnings, err = h.validatorWarn.ValidateUpdate(ctx, oldObj, obj)
	case v1.Delete:
		// In reference to PR: https://github.com/kubernetes/kubernetes/pull/76346
		// OldObject contains the object being deleted
		if err := h.decoder.DecodeRaw(req.OldObject, obj); err != nil {
			return Errored(http.StatusBadRequest, err)
		}

		warnings, err = h.validatorWarn.ValidateDelete(ctx, obj)
	default:
		return Errored(http.StatusBadRequest, fmt.Errorf("unknown operation request %q", req.Operation))
	}

	// Check the error message first.
	if err != nil {
		var apiStatus apierrors.APIStatus
		if errors.As(err, &apiStatus) {
			return validationResponseFromStatus(false, apiStatus.Status())
		}
		return Denied(err.Error()).WithWarnings(warnings...)
	}

	// Return allowed if everything succeeded.
	return Allowed("").WithWarnings(warnings...)
}
