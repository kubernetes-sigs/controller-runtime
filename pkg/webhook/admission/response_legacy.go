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

	"gomodules.xyz/jsonpatch/v2"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AllowedLegacy constructs a response indicating that the given operation
// is allowed (without any patches).
func AllowedLegacy(reason string) ResponseLegacy {
	return ValidationResponseLegacy(true, reason)
}

// DeniedLegacy constructs a response indicating that the given operation
// is not allowed.
func DeniedLegacy(reason string) ResponseLegacy {
	return ValidationResponseLegacy(false, reason)
}

// PatchedLegacy constructs a response indicating that the given operation is
// allowed, and that the target object should be modified by the given
// JSONPatch operations.
func PatchedLegacy(reason string, patches ...jsonpatch.JsonPatchOperation) ResponseLegacy {
	resp := AllowedLegacy(reason)
	resp.Patches = patches

	return resp
}

// ErroredLegacy creates a new ResponseLegacy for error-handling a request.
func ErroredLegacy(code int32, err error) ResponseLegacy {
	return ResponseLegacy{
		AdmissionResponse: admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Code:    code,
				Message: err.Error(),
			},
		},
	}
}

// ValidationResponseLegacy returns a response for admitting a request.
func ValidationResponseLegacy(allowed bool, reason string) ResponseLegacy {
	code := http.StatusForbidden
	if allowed {
		code = http.StatusOK
	}
	resp := ResponseLegacy{
		AdmissionResponse: admissionv1beta1.AdmissionResponse{
			Allowed: allowed,
			Result: &metav1.Status{
				Code: int32(code),
			},
		},
	}
	if len(reason) > 0 {
		resp.Result.Reason = metav1.StatusReason(reason)
	}
	return resp
}

// PatchResponseFromRawLegacy takes 2 byte arrays and returns a new response with json patch.
// The original object should be passed in as raw bytes to avoid the roundtripping problem
// described in https://github.com/kubernetes-sigs/kubebuilder/issues/510.
func PatchResponseFromRawLegacy(original, current []byte) ResponseLegacy {
	patches, err := jsonpatch.CreatePatch(original, current)
	if err != nil {
		return ErroredLegacy(http.StatusInternalServerError, err)
	}
	return ResponseLegacy{
		Patches: patches,
		AdmissionResponse: admissionv1beta1.AdmissionResponse{
			Allowed:   true,
			PatchType: func() *admissionv1beta1.PatchType { pt := admissionv1beta1.PatchTypeJSONPatch; return &pt }(),
		},
	}
}

// validationResponseFromStatusLegacy returns a response for admitting a request with provided Status object.
func validationResponseFromStatusLegacy(allowed bool, status metav1.Status) ResponseLegacy {
	resp := ResponseLegacy{
		AdmissionResponse: admissionv1beta1.AdmissionResponse{
			Allowed: allowed,
			Result:  &status,
		},
	}
	return resp
}

// WithWarnings adds the given warnings to the ResponseLegacy.
// If any warnings were already given, they will not be overwritten.
func (r ResponseLegacy) WithWarnings(warnings ...string) ResponseLegacy {
	r.AdmissionResponse.Warnings = append(r.AdmissionResponse.Warnings, warnings...)
	return r
}
