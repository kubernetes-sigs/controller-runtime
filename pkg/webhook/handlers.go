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

package webhook

import (
	"context"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	"k8s.io/api/admission/v1beta1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/builder"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

// Validator defines functions for validating an operation
type Validator interface {
	runtime.Object
	ValidateCreate() error
	ValidateUpdate(old runtime.Object) error
}

// Defaultor defines functions for setting defaults on resources
type Defaulter interface {
	runtime.Object
	Default()
}

// NewResourceWebhook creates a new WebhookBuilder for Validation and Defaulting the provided type.  Returns nil
// if o does not implement either Validator or Defaulter.
func NewResourceWebhook(o runtime.Object, manager manager.Manager) (*admission.Webhook, error) {
	// Create the handler
	h := &resourceHandler{}
	found := false
	if v, ok := o.(Validator); ok {
		h.Validator = v
		found = true
	}
	if d, ok := o.(Defaulter); ok {
		h.Defaulter = d
		found = true
	}
	if !found {
		// Type doesn't implement functions for webhooks
		return nil, nil
	}

	// Create the webhook
	return builder.NewWebhookBuilder().
		Mutating(). // Always use a mutating webhook in case defaulting is implemented later
		Operations(
			admissionregistrationv1beta1.Create,
			admissionregistrationv1beta1.Update).
		WithManager(manager).
		ForType(o).
		Handlers(h).
		Build()
}

type resourceHandler struct {
	Validator Validator
	Defaulter Defaulter
	Decoder   types.Decoder
}

var _ inject.Decoder = &resourceHandler{}

// InjectDecoder injects the decoder into the FirstMateDeleteHandler
func (h *resourceHandler) InjectDecoder(d types.Decoder) error {
	h.Decoder = d
	return nil
}

// Handle handles admission requests.
func (h *resourceHandler) Handle(ctx context.Context, req types.Request) types.Response {
	// Get the object in the request
	if resp, send := h.doValidate(ctx, req); send {
		return resp
	}

	if resp, send := h.doDefault(ctx, req); send {
		return resp
	}

	return admission.ValidationResponse(true, "")
}

func (h *resourceHandler) doDefault(ctx context.Context, req types.Request) (types.Response, bool) {
	if h.Defaulter == nil {
		return types.Response{}, false
	}

	// Get the object in the request
	obj := h.Defaulter.DeepCopyObject().(Defaulter)
	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err), true
	}

	// Default the object
	copy := obj.DeepCopyObject().(Defaulter)
	copy.Default()

	// Create the patch
	return admission.PatchResponse(obj, copy), true
}

func (h *resourceHandler) doValidate(ctx context.Context, req types.Request) (types.Response, bool) {
	if h.Validator == nil {
		return types.Response{}, false
	}

	// Get the object in the request
	obj := h.Validator.DeepCopyObject().(Validator)
	if req.AdmissionRequest.Operation == v1beta1.Create {
		err := h.Decoder.Decode(req, obj)
		if err != nil {
			return admission.ErrorResponse(http.StatusBadRequest, err), true
		}

		err = obj.ValidateCreate()
		if err != nil {
			return admission.ValidationResponse(false, err.Error()), true
		}
	}

	if req.AdmissionRequest.Operation == v1beta1.Update {
		oldObj := obj.DeepCopyObject()

		err := h.Decoder.Decode(req, obj)
		if err != nil {
			return admission.ErrorResponse(http.StatusBadRequest, err), true
		}
		err = h.Decoder.DecodeOld(req, oldObj)
		if err != nil {
			return admission.ErrorResponse(http.StatusBadRequest, err), true
		}

		err = obj.ValidateUpdate(oldObj)
		if err != nil {
			return admission.ValidationResponse(false, err.Error()), true
		}
	}

	return types.Response{}, false
}
