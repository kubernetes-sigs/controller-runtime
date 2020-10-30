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
	"errors"
	"net/http"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gomodules.xyz/jsonpatch/v2"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Admission Webhook Response Helpers", func() {
	Describe("Allowed", func() {
		It("should return an 'allowed' response", func() {
			Expect(AllowedLegacy("")).To(Equal(
				ResponseLegacy{
					AdmissionResponse: admissionv1beta1.AdmissionResponse{
						Allowed: true,
						Result: &metav1.Status{
							Code: http.StatusOK,
						},
					},
				},
			))
		})

		It("should populate a status with a reason when a reason is given", func() {
			Expect(AllowedLegacy("acceptable")).To(Equal(
				ResponseLegacy{
					AdmissionResponse: admissionv1beta1.AdmissionResponse{
						Allowed: true,
						Result: &metav1.Status{
							Code:   http.StatusOK,
							Reason: "acceptable",
						},
					},
				},
			))
		})
	})

	Describe("Denied", func() {
		It("should return a 'not allowed' response", func() {
			Expect(DeniedLegacy("")).To(Equal(
				ResponseLegacy{
					AdmissionResponse: admissionv1beta1.AdmissionResponse{
						Allowed: false,
						Result: &metav1.Status{
							Code: http.StatusForbidden,
						},
					},
				},
			))
		})

		It("should populate a status with a reason when a reason is given", func() {
			Expect(DeniedLegacy("UNACCEPTABLE!")).To(Equal(
				ResponseLegacy{
					AdmissionResponse: admissionv1beta1.AdmissionResponse{
						Allowed: false,
						Result: &metav1.Status{
							Code:   http.StatusForbidden,
							Reason: "UNACCEPTABLE!",
						},
					},
				},
			))
		})
	})

	Describe("Patched", func() {
		ops := []jsonpatch.JsonPatchOperation{
			{
				Operation: "replace",
				Path:      "/spec/selector/matchLabels",
				Value:     map[string]string{"foo": "bar"},
			},
			{
				Operation: "delete",
				Path:      "/spec/replicas",
			},
		}
		It("should return an 'allowed' response with the given patches", func() {
			Expect(PatchedLegacy("", ops...)).To(Equal(
				ResponseLegacy{
					AdmissionResponse: admissionv1beta1.AdmissionResponse{
						Allowed: true,
						Result: &metav1.Status{
							Code: http.StatusOK,
						},
					},
					Patches: ops,
				},
			))
		})
		It("should populate a status with a reason when a reason is given", func() {
			Expect(PatchedLegacy("some changes", ops...)).To(Equal(
				ResponseLegacy{
					AdmissionResponse: admissionv1beta1.AdmissionResponse{
						Allowed: true,
						Result: &metav1.Status{
							Code:   http.StatusOK,
							Reason: "some changes",
						},
					},
					Patches: ops,
				},
			))
		})
	})

	Describe("Errored", func() {
		It("should return a denied response with an error", func() {
			err := errors.New("this is an error")
			expected := ResponseLegacy{
				AdmissionResponse: admissionv1beta1.AdmissionResponse{
					Allowed: false,
					Result: &metav1.Status{
						Code:    http.StatusBadRequest,
						Message: err.Error(),
					},
				},
			}
			resp := ErroredLegacy(http.StatusBadRequest, err)
			Expect(resp).To(Equal(expected))
		})
	})

	Describe("ValidationResponse", func() {
		It("should populate a status with a reason when a reason is given", func() {
			By("checking that a message is populated for 'allowed' responses")
			Expect(ValidationResponseLegacy(true, "acceptable")).To(Equal(
				ResponseLegacy{
					AdmissionResponse: admissionv1beta1.AdmissionResponse{
						Allowed: true,
						Result: &metav1.Status{
							Code:   http.StatusOK,
							Reason: "acceptable",
						},
					},
				},
			))

			By("checking that a message is populated for 'denied' responses")
			Expect(ValidationResponseLegacy(false, "UNACCEPTABLE!")).To(Equal(
				ResponseLegacy{
					AdmissionResponse: admissionv1beta1.AdmissionResponse{
						Allowed: false,
						Result: &metav1.Status{
							Code:   http.StatusForbidden,
							Reason: "UNACCEPTABLE!",
						},
					},
				},
			))
		})

		It("should return an admission decision", func() {
			By("checking that it returns an 'allowed' response when allowed is true")
			Expect(ValidationResponseLegacy(true, "")).To(Equal(
				ResponseLegacy{
					AdmissionResponse: admissionv1beta1.AdmissionResponse{
						Allowed: true,
						Result: &metav1.Status{
							Code: http.StatusOK,
						},
					},
				},
			))

			By("checking that it returns an 'denied' response when allowed is false")
			Expect(ValidationResponseLegacy(false, "")).To(Equal(
				ResponseLegacy{
					AdmissionResponse: admissionv1beta1.AdmissionResponse{
						Allowed: false,
						Result: &metav1.Status{
							Code: http.StatusForbidden,
						},
					},
				},
			))
		})
	})

	Describe("PatchResponseFromRaw", func() {
		It("should return an 'allowed' response with a patch of the diff between two sets of serialized JSON", func() {
			expected := ResponseLegacy{
				Patches: []jsonpatch.JsonPatchOperation{
					{Operation: "replace", Path: "/a", Value: "bar"},
				},
				AdmissionResponse: admissionv1beta1.AdmissionResponse{
					Allowed:   true,
					PatchType: func() *admissionv1beta1.PatchType { pt := admissionv1beta1.PatchTypeJSONPatch; return &pt }(),
				},
			}
			resp := PatchResponseFromRawLegacy([]byte(`{"a": "foo"}`), []byte(`{"a": "bar"}`))
			Expect(resp).To(Equal(expected))
		})
	})

	Describe("WithWarnings", func() {
		It("should add the warnings to the existing response without removing any existing warnings", func() {
			initialResponse := ResponseLegacy{
				AdmissionResponse: admissionv1beta1.AdmissionResponse{
					Allowed: true,
					Result: &metav1.Status{
						Code: http.StatusOK,
					},
					Warnings: []string{"existing-warning"},
				},
			}
			warnings := []string{"additional-warning-1", "additional-warning-2"}
			expectedResponse := ResponseLegacy{
				AdmissionResponse: admissionv1beta1.AdmissionResponse{
					Allowed: true,
					Result: &metav1.Status{
						Code: http.StatusOK,
					},
					Warnings: []string{"existing-warning", "additional-warning-1", "additional-warning-2"},
				},
			}

			Expect(initialResponse.WithWarnings(warnings...)).To(Equal(expectedResponse))
		})
	})
})
