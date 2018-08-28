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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/mattbaird/jsonpatch"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/types"
)

var _ = Describe("admission webhook", func() {
	Describe("validating webhook", func() {
		var alwaysAllow, alwaysDeny *fakeHandler
		var req *http.Request
		var w *fakeResponseWriter
		var wh *Webhook
		BeforeEach(func(done Done) {
			alwaysAllow = &fakeHandler{
				fn: func(ctx context.Context, req Request) Response {
					return Response{
						Response: &admissionv1beta1.AdmissionResponse{
							Allowed: true,
						},
					}
				},
			}
			alwaysDeny = &fakeHandler{
				fn: func(ctx context.Context, req Request) Response {
					return Response{
						Response: &admissionv1beta1.AdmissionResponse{
							Allowed: false,
						},
					}
				},
			}
			req = &http.Request{
				Header: http.Header{"Content-Type": []string{"application/json"}},
				Body:   nopCloser{Reader: bytes.NewBufferString(`{"request":{}}`)},
			}

			w = &fakeResponseWriter{}
			close(done)
		})

		Context("multiple handlers can be invoked", func() {
			BeforeEach(func(done Done) {
				wh = &Webhook{
					Type:     types.WebhookTypeValidating,
					Handlers: []Handler{alwaysAllow, alwaysDeny},
					KVMap:    map[string]interface{}{"foo": "bar"},
				}
				close(done)
			})

			It("should deny the request", func() {
				expected := []byte(`{"response":{"uid":"","allowed":false}}
`)
				wh.ServeHTTP(w, req)
				Expect(w.response).NotTo(BeNil())
				Expect(w.response).To(Equal(expected))
				Expect(alwaysAllow.invoked).To(BeTrue())
				Expect(alwaysDeny.invoked).To(BeTrue())
				Expect(alwaysAllow.valueFromContext).To(Equal("bar"))
				Expect(alwaysDeny.valueFromContext).To(Equal("bar"))
			})
		})

		Context("validating webhook should return if one of the handler denies", func() {
			BeforeEach(func(done Done) {
				wh = &Webhook{
					Type:     types.WebhookTypeValidating,
					Handlers: []Handler{alwaysDeny, alwaysAllow},
					KVMap:    map[string]interface{}{"foo": "bar"},
				}
				close(done)
			})

			It("should deny the request", func() {
				expected := []byte(`{"response":{"uid":"","allowed":false}}
`)
				wh.ServeHTTP(w, req)
				Expect(w.response).NotTo(BeNil())
				Expect(w.response).To(Equal(expected))
				Expect(alwaysDeny.invoked).To(BeTrue())
				Expect(alwaysDeny.valueFromContext).To(Equal("bar"))
				Expect(alwaysAllow.invoked).To(BeFalse())
			})
		})
	})

	Describe("mutating webhook", func() {
		Context("patch handler not returning the right patch type", func() {
			req := &http.Request{
				Header: http.Header{"Content-Type": []string{"application/json"}},
				Body:   nopCloser{Reader: bytes.NewBufferString(`{"request":{}}`)},
			}
			errPatcher := &fakeHandler{
				fn: func(ctx context.Context, req Request) Response {
					return Response{
						Patches: []jsonpatch.JsonPatchOperation{
							{
								Operation: "add",
								Path:      "/metadata/annotation/new-key",
								Value:     "new-value",
							},
						},
						Response: &admissionv1beta1.AdmissionResponse{
							Allowed: true,
						},
					}
				},
			}
			wh := &Webhook{
				Type:     types.WebhookTypeMutating,
				Handlers: []Handler{errPatcher},
				KVMap:    map[string]interface{}{"foo": "bar"},
			}
			w := &fakeResponseWriter{}
			expected := []byte(
				`{"response":{"uid":"","allowed":false,"status":{"metadata":{},"message":"unexpected patch type ` +
					`returned by the handler: \u003cnil\u003e, only allow: JSONPatch","code":500}}}
`)
			It("should return a response with error", func() {
				wh.ServeHTTP(w, req)
				Expect(w.response).NotTo(BeNil())
				Expect(w.response).To(Equal(expected))
				Expect(errPatcher.invoked).To(BeTrue())
				Expect(errPatcher.valueFromContext).To(Equal("bar"))
			})
		})

		Context("multiple patch handlers", func() {
			req := &http.Request{
				Header: http.Header{"Content-Type": []string{"application/json"}},
				Body:   nopCloser{Reader: bytes.NewBufferString(`{"request":{}}`)},
			}
			patcher1 := &fakeHandler{
				fn: func(ctx context.Context, req Request) Response {
					return Response{
						Patches: []jsonpatch.JsonPatchOperation{
							{
								Operation: "add",
								Path:      "/metadata/annotation/new-key",
								Value:     "new-value",
							},
							{
								Operation: "replace",
								Path:      "/spec/replicas",
								Value:     "2",
							},
						},
						Response: &admissionv1beta1.AdmissionResponse{
							Allowed:   true,
							PatchType: func() *admissionv1beta1.PatchType { pt := admissionv1beta1.PatchTypeJSONPatch; return &pt }(),
						},
					}
				},
			}
			patcher2 := &fakeHandler{
				fn: func(ctx context.Context, req Request) Response {
					return Response{
						Patches: []jsonpatch.JsonPatchOperation{
							{
								Operation: "add",
								Path:      "/metadata/annotation/hello",
								Value:     "world",
							},
						},
						Response: &admissionv1beta1.AdmissionResponse{
							Allowed:   true,
							PatchType: func() *admissionv1beta1.PatchType { pt := admissionv1beta1.PatchTypeJSONPatch; return &pt }(),
						},
					}
				},
			}
			wh := &Webhook{
				Type:     types.WebhookTypeMutating,
				Handlers: []Handler{patcher1, patcher2},
				KVMap:    map[string]interface{}{"foo": "bar"},
			}
			w := &fakeResponseWriter{}
			expected := []byte(
				`{"response":{"uid":"","allowed":false,"patch":` +
					`"W3sib3AiOiJhZGQiLCJwYXRoIjoiL21ldGFkYXRhL2Fubm90YXRpb24vbmV3LWtleSIsInZhbHVlIjoibmV3LXZhbHVlIn0` +
					`seyJvcCI6InJlcGxhY2UiLCJwYXRoIjoiL3NwZWMvcmVwbGljYXMiLCJ2YWx1ZSI6IjIifSx7Im9wIjoiYWRkIiwicGF0aCI6Ii9tZXRhZGF0YS9hbm5vdGF0aW9uL2hlbGxvIiwidmFsdWUiOiJ3b3JsZCJ9XQ=="}}
`)
			patches := []jsonpatch.JsonPatchOperation{
				{
					Operation: "add",
					Path:      "/metadata/annotation/new-key",
					Value:     "new-value",
				},
				{
					Operation: "replace",
					Path:      "/spec/replicas",
					Value:     "2",
				},
				{
					Operation: "add",
					Path:      "/metadata/annotation/hello",
					Value:     "world",
				},
			}
			j, _ := json.Marshal(patches)
			base64encoded := base64.StdEncoding.EncodeToString(j)
			It("should aggregates patches from multiple handlers", func() {
				wh.ServeHTTP(w, req)
				Expect(w.response).NotTo(BeNil())
				Expect(w.response).To(Equal(expected))
				Expect(string(w.response)).To(ContainSubstring(base64encoded))
				Expect(patcher1.invoked).To(BeTrue())
				Expect(patcher2.invoked).To(BeTrue())
				Expect(patcher1.valueFromContext).To(Equal("bar"))
				Expect(patcher2.valueFromContext).To(Equal("bar"))
			})
		})
	})

	Describe("webhook defaulting", func() {
		Context("patch handler not returning the right patch type", func() {
			req := &http.Request{
				Header: http.Header{"Content-Type": []string{"application/json"}},
				Body:   nopCloser{Reader: bytes.NewBufferString(`{"request":{}}`)},
			}
			errPatcher := &fakeHandler{
				fn: func(ctx context.Context, req Request) Response {
					return Response{
						Patches: []jsonpatch.JsonPatchOperation{
							{
								Operation: "add",
								Path:      "/metadata/annotation/new-key",
								Value:     "new-value",
							},
						},
						Response: &admissionv1beta1.AdmissionResponse{
							Allowed: true,
						},
					}
				},
			}
			wh := &Webhook{
				Type:     types.WebhookTypeMutating,
				Handlers: []Handler{errPatcher},
				KVMap:    map[string]interface{}{"foo": "bar"},
			}
			w := &fakeResponseWriter{}
			expected := []byte(
				`{"response":{"uid":"","allowed":false,"status":{"metadata":{},"message":"unexpected patch type ` +
					`returned by the handler: \u003cnil\u003e, only allow: JSONPatch","code":500}}}
`)
			It("should return a response with error", func() {
				wh.ServeHTTP(w, req)
				Expect(w.response).NotTo(BeNil())
				Expect(w.response).To(Equal(expected))
				Expect(errPatcher.invoked).To(BeTrue())
				Expect(errPatcher.valueFromContext).To(Equal("bar"))
			})
		})
	})

	Describe("webhook validation", func() {
		Context("valid mutating webhook", func() {
			wh := &Webhook{
				Type:     types.WebhookTypeMutating,
				Handlers: []Handler{&fakeHandler{}},
				Rules: []admissionregistrationv1beta1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1beta1.OperationType{},
						Rule: admissionregistrationv1beta1.Rule{
							APIGroups:   []string{"apps"},
							APIVersions: []string{"v1"},
							Resources:   []string{"deployments"}},
					},
				},
			}
			It("should pass validation", func() {
				err := wh.Validate()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("valid validating webhook", func() {
			wh := &Webhook{
				Type:     types.WebhookTypeValidating,
				Handlers: []Handler{&fakeHandler{}},
				Rules: []admissionregistrationv1beta1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1beta1.OperationType{},
						Rule: admissionregistrationv1beta1.Rule{
							APIGroups:   []string{"apps"},
							APIVersions: []string{"v1"},
							Resources:   []string{"deployments"}},
					},
				},
			}
			It("should pass validation", func() {
				err := wh.Validate()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("missing webhook type", func() {
			wh := &Webhook{
				Handlers: []Handler{&fakeHandler{}},
				Rules: []admissionregistrationv1beta1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1beta1.OperationType{},
						Rule: admissionregistrationv1beta1.Rule{
							APIGroups:   []string{"apps"},
							APIVersions: []string{"v1"},
							Resources:   []string{"deployments"}},
					},
				},
			}
			It("should fail validation", func() {
				err := wh.Validate()
				Expect(err.Error()).To(ContainSubstring("only WebhookTypeMutating and WebhookTypeValidating are supported"))
			})
		})

		Context("missing Rules", func() {
			wh := &Webhook{
				Type:     types.WebhookTypeValidating,
				Handlers: []Handler{&fakeHandler{}},
			}
			It("should fail validation", func() {
				err := wh.Validate()
				Expect(err).To(Equal(errors.New("field Rules should not be empty")))

			})
		})

		Context("missing Handlers", func() {
			wh := &Webhook{
				Type:     types.WebhookTypeValidating,
				Handlers: []Handler{},
				Rules: []admissionregistrationv1beta1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1beta1.OperationType{},
						Rule: admissionregistrationv1beta1.Rule{
							APIGroups:   []string{"apps"},
							APIVersions: []string{"v1"},
							Resources:   []string{"deployments"}},
					},
				},
			}
			It("should fail validation", func() {
				err := wh.Validate()
				Expect(err).To(Equal(errors.New("field Handler should not be empty")))
			})
		})

	})
})
