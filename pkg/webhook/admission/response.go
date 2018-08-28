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
	"net/http"

	"github.com/mattbaird/jsonpatch"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/patch"
)

// Response is the output of admission.Handler
type Response struct {
	// Patches are the JSON patches for mutating webhooks.
	// Using this instead of setting Response.Patch to minimize the overhead of serialization and deserialization.
	Patches []jsonpatch.JsonPatchOperation
	// Response is the admission response. Don't set the Patch field in it.
	Response *admissionv1beta1.AdmissionResponse
}

// ErrorResponse creates a new Response for an error handling the request
func ErrorResponse(code int32, err error) Response {
	return Response{
		Response: &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Code:    code,
				Message: err.Error(),
			},
		},
	}
}

// ValidationResponse returns a response for admitting a request
func ValidationResponse(allowed bool, reason string) Response {
	resp := Response{
		Response: &admissionv1beta1.AdmissionResponse{
			Allowed: allowed,
		},
	}
	if len(reason) > 0 {
		resp.Response.Result = &metav1.Status{
			Reason: metav1.StatusReason(reason),
		}
	}
	return resp
}

// PatchResponse returns a new response with json patch
func PatchResponse(original, current runtime.Object) Response {
	patches, err := patch.NewJSONPatch(original, current)
	if err != nil {
		return ErrorResponse(http.StatusInternalServerError, err)
	}
	return Response{
		Patches: patches,
		Response: &admissionv1beta1.AdmissionResponse{
			Allowed:   true,
			PatchType: func() *admissionv1beta1.PatchType { pt := admissionv1beta1.PatchTypeJSONPatch; return &pt }(),
		},
	}
}
