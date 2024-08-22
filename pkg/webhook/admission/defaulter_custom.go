/*
Copyright 2021 The Kubernetes Authors.

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
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// CustomDefaulter defines functions for setting defaults on resources.
type CustomDefaulter interface {
	Default(ctx context.Context, obj runtime.Object) error
}

// WithCustomDefaulter creates a new Webhook for a CustomDefaulter interface.
func WithCustomDefaulter(scheme *runtime.Scheme, obj runtime.Object, defaulter CustomDefaulter) *Webhook {
	return &Webhook{
		Handler: &defaulterForType{object: obj, defaulter: defaulter, decoder: NewDecoder(scheme)},
	}
}

type defaulterForType struct {
	defaulter CustomDefaulter
	object    runtime.Object
	decoder   Decoder
}

// Handle handles admission requests.
func (h *defaulterForType) Handle(ctx context.Context, req Request) Response {
	if h.decoder == nil {
		panic("decoder should never be nil")
	}
	if h.defaulter == nil {
		panic("defaulter should never be nil")
	}
	if h.object == nil {
		panic("object should never be nil")
	}

	// Always skip when a DELETE operation received in custom mutation handler.
	if req.Operation == admissionv1.Delete {
		return Response{AdmissionResponse: admissionv1.AdmissionResponse{
			Allowed: true,
			Result: &metav1.Status{
				Code: http.StatusOK,
			},
		}}
	}

	ctx = NewContextWithRequest(ctx, req)

	// Get the object in the request
	original := h.object.DeepCopyObject()
	if err := h.decoder.Decode(req, original); err != nil {
		return Errored(http.StatusBadRequest, err)
	}

	// Default the object
	updated := original.DeepCopyObject()
	if err := h.defaulter.Default(ctx, updated); err != nil {
		var apiStatus apierrors.APIStatus
		if errors.As(err, &apiStatus) {
			return validationResponseFromStatus(false, apiStatus.Status())
		}
		return Denied(err.Error())
	}

	// Create the patch.
	// We need to decode and marshall the original because the type registered in the
	// decoder might not match the latest version of the API.
	// Creating a diff from the raw object might cause new fields to be dropped.
	marshalledOriginal, err := json.Marshal(original)
	if err != nil {
		return Errored(http.StatusInternalServerError, err)
	}
	marshalledUpdated, err := json.Marshal(updated)
	if err != nil {
		return Errored(http.StatusInternalServerError, err)
	}
	return PatchResponseFromRaw(marshalledOriginal, marshalledUpdated)
}
