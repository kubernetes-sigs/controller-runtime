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

	"k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Validator defines functions for validating an operation
type Validator interface {
	runtime.Object
	ValidateCreate() error
	ValidateUpdate(old runtime.Object) error
	ValidateDelete() error
}

// ValidatorService can be implemented and injected into the webhook to validate an operation
type ValidatorService interface {
	ValidateCreate(new runtime.Object) error
	ValidateUpdate(new runtime.Object, old runtime.Object) error
	ValidateDelete(existing runtime.Object) error
}

// ValidatingWebhookFor creates a new Webhook for validating the provided type.
func ValidatingWebhookFor(validator Validator) *Webhook {
	return &Webhook{
		Handler: &validatingHandler{validator: validator},
	}
}

// ValidatingWebhookServiceFor creates a new Webhook for validating the provided type.
func ValidatingWebhookServiceFor(validatorService ValidatorService, runtimeObject runtime.Object) *Webhook {
	return &Webhook{
		Handler: &validatingServiceHandler{
			validatorService: validatorService,
			runtimeObject:    runtimeObject,
		},
	}
}

type validatingServiceHandler struct {
	runtimeObject    runtime.Object
	validatorService ValidatorService
	decoder          *Decoder
}

func (v *validatingServiceHandler) InjectDecoder(d *Decoder) error {
	v.decoder = d
	return nil
}

type validatingHandler struct {
	validator Validator
	decoder   *Decoder
}

var _ DecoderInjector = &validatingHandler{}

// InjectDecoder injects the decoder into a validatingHandler.
func (h *validatingHandler) InjectDecoder(d *Decoder) error {
	h.decoder = d
	return nil
}

// Handle handles admission requests.
func (h *validatingHandler) Handle(ctx context.Context, req Request) Response {
	if h.validator == nil {
		panic("validator should never be nil")
	}

	// Get the object in the request
	obj := h.validator.DeepCopyObject().(Validator)
	if req.Operation == v1beta1.Create {
		err := h.decoder.Decode(req, obj)
		if err != nil {
			return Errored(http.StatusBadRequest, err)
		}

		err = obj.ValidateCreate()
		if err != nil {
			return Denied(err.Error())
		}
	}

	if req.Operation == v1beta1.Update {
		oldObj := obj.DeepCopyObject()

		err := h.decoder.DecodeRaw(req.Object, obj)
		if err != nil {
			return Errored(http.StatusBadRequest, err)
		}
		err = h.decoder.DecodeRaw(req.OldObject, oldObj)
		if err != nil {
			return Errored(http.StatusBadRequest, err)
		}

		err = obj.ValidateUpdate(oldObj)
		if err != nil {
			return Denied(err.Error())
		}
	}

	if req.Operation == v1beta1.Delete {
		// In reference to PR: https://github.com/kubernetes/kubernetes/pull/76346
		// OldObject contains the object being deleted
		err := h.decoder.DecodeRaw(req.OldObject, obj)
		if err != nil {
			return Errored(http.StatusBadRequest, err)
		}

		err = obj.ValidateDelete()
		if err != nil {
			return Denied(err.Error())
		}
	}

	return Allowed("")
}

func (v *validatingServiceHandler) Handle(ctx context.Context, req Request) Response {
	if v.validatorService == nil {
		panic("validator service should never be nil")
	}

	// Get the object in the request
	obj := v.runtimeObject.DeepCopyObject()
	if req.Operation == v1beta1.Create {
		err := v.decoder.Decode(req, obj)
		if err != nil {
			return Errored(http.StatusBadRequest, err)
		}

		err = v.validatorService.ValidateCreate(obj)
		if err != nil {
			return Denied(err.Error())
		}
	}

	if req.Operation == v1beta1.Update {
		oldObj := obj.DeepCopyObject()

		err := v.decoder.DecodeRaw(req.Object, obj)
		if err != nil {
			return Errored(http.StatusBadRequest, err)
		}
		err = v.decoder.DecodeRaw(req.OldObject, oldObj)
		if err != nil {
			return Errored(http.StatusBadRequest, err)
		}

		err = v.validatorService.ValidateUpdate(obj, oldObj)
		if err != nil {
			return Denied(err.Error())
		}
	}

	if req.Operation == v1beta1.Delete {
		// In reference to PR: https://github.com/kubernetes/kubernetes/pull/76346
		// OldObject contains the object being deleted
		err := v.decoder.DecodeRaw(req.OldObject, obj)
		if err != nil {
			return Errored(http.StatusBadRequest, err)
		}

		err = v.validatorService.ValidateDelete(obj)
		if err != nil {
			return Denied(err.Error())
		}
	}

	return Allowed("")
}
