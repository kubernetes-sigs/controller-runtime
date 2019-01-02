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

package resource

import (
	"context"
	"net/http"

	"k8s.io/api/admission/v1beta1"

	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

type ValidateResourceHandler struct {
	Validator Validator
	Decoder   types.Decoder
}

var _ inject.Decoder = &ValidateResourceHandler{}

// InjectDecoder injects the decoder into the FirstMateDeleteHandler
func (h *ValidateResourceHandler) InjectDecoder(d types.Decoder) error {
	h.Decoder = d
	return nil
}

// Handle handles admission requests.
func (h *ValidateResourceHandler) Handle(ctx context.Context, req types.Request) types.Response {
	// Get the object in the request
	obj := h.Validator.DeepCopyObject().(Validator)

	if req.AdmissionRequest.Operation == v1beta1.Create {
		err := h.Decoder.Decode(req, obj)
		if err != nil {
			return admission.ErrorResponse(http.StatusBadRequest, err)
		}

		err = obj.ValidateCreate()
		if err != nil {
			return admission.ValidationResponse(false, err.Error())
		}
	}

	if req.AdmissionRequest.Operation == v1beta1.Delete {
		err := h.Decoder.DecodeOld(req, obj)
		if err != nil {
			return admission.ErrorResponse(http.StatusBadRequest, err)
		}

		err = obj.ValidateDelete()
		if err != nil {
			return admission.ValidationResponse(false, err.Error())
		}
	}

	if req.AdmissionRequest.Operation == v1beta1.Update {
		oldObj := obj.DeepCopyObject()

		err := h.Decoder.Decode(req, obj)
		if err != nil {
			return admission.ErrorResponse(http.StatusBadRequest, err)
		}
		err = h.Decoder.DecodeOld(req, oldObj)
		if err != nil {
			return admission.ErrorResponse(http.StatusBadRequest, err)
		}

		err = obj.ValidateUpdate(oldObj)
		if err != nil {
			return admission.ValidationResponse(false, err.Error())
		}
	}

	return admission.ValidationResponse(true, "")
}
