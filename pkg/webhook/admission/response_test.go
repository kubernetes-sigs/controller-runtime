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

	"github.com/appscode/jsonpatch"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Admission Webhook Response Helpers", func() {
	Describe("Allowed", func() {
		It("should return an 'allowed' response", func() {
			Expect(Allowed("")).To(Equal(
				Response{
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
			Expect(Allowed("acceptable")).To(Equal(
				Response{
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
			Expect(Denied("")).To(Equal(
				Response{
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
			Expect(Denied("UNACCEPTABLE!")).To(Equal(
				Response{
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
			Expect(Patched("", ops...)).To(Equal(
				Response{
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
			Expect(Patched("some changes", ops...)).To(Equal(
				Response{
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
			expected := Response{
				AdmissionResponse: admissionv1beta1.AdmissionResponse{
					Allowed: false,
					Result: &metav1.Status{
						Code:    http.StatusBadRequest,
						Message: err.Error(),
					},
				},
			}
			resp := Errored(http.StatusBadRequest, err)
			Expect(resp).To(Equal(expected))
		})
	})

	Describe("ValidationResponse", func() {
		It("should populate a status with a reason when a reason is given", func() {
			By("checking that a message is populated for 'allowed' responses")
			Expect(ValidationResponse(true, "acceptable")).To(Equal(
				Response{
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
			Expect(ValidationResponse(false, "UNACCEPTABLE!")).To(Equal(
				Response{
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
			Expect(ValidationResponse(true, "")).To(Equal(
				Response{
					AdmissionResponse: admissionv1beta1.AdmissionResponse{
						Allowed: true,
						Result: &metav1.Status{
							Code: http.StatusOK,
						},
					},
				},
			))

			By("checking that it returns an 'denied' response when allowed is false")
			Expect(ValidationResponse(false, "")).To(Equal(
				Response{
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
			expected := Response{
				Patches: []jsonpatch.JsonPatchOperation{
					{Operation: "replace", Path: "/a", Value: "bar"},
				},
				AdmissionResponse: admissionv1beta1.AdmissionResponse{
					Allowed:   true,
					PatchType: func() *admissionv1beta1.PatchType { pt := admissionv1beta1.PatchTypeJSONPatch; return &pt }(),
				},
			}
			resp := PatchResponseFromRaw([]byte(`{"a": "foo"}`), []byte(`{"a": "bar"}`))
			Expect(resp).To(Equal(expected))
		})
	})
})
