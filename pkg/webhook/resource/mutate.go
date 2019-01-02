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

	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

type MutateResourceHandler struct {
	Defaulter Defaulter
	Decoder   types.Decoder
}

var _ inject.Decoder = &MutateResourceHandler{}

// InjectDecoder injects the decoder into the FirstMateDeleteHandler
func (h *MutateResourceHandler) InjectDecoder(d types.Decoder) error {
	h.Decoder = d
	return nil
}

// Handle handles admission requests.
func (h *MutateResourceHandler) Handle(ctx context.Context, req types.Request) types.Response {
	// Get the object in the request
	obj := h.Defaulter.DeepCopyObject().(Defaulter)
	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	// Default the object
	copy := obj.DeepCopyObject().(Defaulter)
	copy.Default()

	// Create the patch
	return admission.PatchResponse(obj, copy)
}
