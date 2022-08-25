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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	jsonpatch "gomodules.xyz/jsonpatch/v2"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	machinerytypes "k8s.io/apimachinery/pkg/types"

	logf "sigs.k8s.io/controller-runtime/pkg/internal/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
)

var _ = Describe("Admission Webhooks", func() {
	allowHandler := func() *Webhook {
		handler := &fakeHandler{
			fn: func(ctx context.Context, req Request) Response {
				return Response{
					AdmissionResponse: admissionv1.AdmissionResponse{
						Allowed: true,
					},
				}
			},
		}
		webhook := &Webhook{
			Handler: handler,
			log:     logf.RuntimeLog.WithName("webhook"),
		}

		return webhook
	}

	It("should invoke the handler to get a response", func() {
		By("setting up a webhook with an allow handler")
		webhook := allowHandler()

		By("invoking the webhook")
		resp := webhook.Handle(context.Background(), Request{})

		By("checking that it allowed the request")
		Expect(resp.Allowed).To(BeTrue())
	})

	It("should ensure that the response's UID is set to the request's UID", func() {
		By("setting up a webhook")
		webhook := allowHandler()

		By("invoking the webhook")
		resp := webhook.Handle(context.Background(), Request{AdmissionRequest: admissionv1.AdmissionRequest{UID: "foobar"}})

		By("checking that the response share's the request's UID")
		Expect(resp.UID).To(Equal(machinerytypes.UID("foobar")))
	})

	It("should populate the status on a response if one is not provided", func() {
		By("setting up a webhook")
		webhook := allowHandler()

		By("invoking the webhook")
		resp := webhook.Handle(context.Background(), Request{})

		By("checking that the response share's the request's UID")
		Expect(resp.Result).To(Equal(&metav1.Status{Code: http.StatusOK}))
	})

	It("shouldn't overwrite the status on a response", func() {
		By("setting up a webhook that sets a status")
		webhook := &Webhook{
			Handler: HandlerFunc(func(ctx context.Context, req Request) Response {
				return Response{
					AdmissionResponse: admissionv1.AdmissionResponse{
						Allowed: true,
						Result:  &metav1.Status{Message: "Ground Control to Major Tom"},
					},
				}
			}),
			log: logf.RuntimeLog.WithName("webhook"),
		}

		By("invoking the webhook")
		resp := webhook.Handle(context.Background(), Request{})

		By("checking that the message is intact")
		Expect(resp.Result).NotTo(BeNil())
		Expect(resp.Result.Message).To(Equal("Ground Control to Major Tom"))
	})

	It("should serialize patch operations into a single jsonpatch blob", func() {
		By("setting up a webhook with a patching handler")
		webhook := &Webhook{
			Handler: HandlerFunc(func(ctx context.Context, req Request) Response {
				return Patched("", jsonpatch.Operation{Operation: "add", Path: "/a", Value: 2}, jsonpatch.Operation{Operation: "replace", Path: "/b", Value: 4})
			}),
			log: logf.RuntimeLog.WithName("webhook"),
		}

		By("invoking the webhook")
		resp := webhook.Handle(context.Background(), Request{})

		By("checking that a JSON patch is populated on the response")
		patchType := admissionv1.PatchTypeJSONPatch
		Expect(resp.PatchType).To(Equal(&patchType))
		Expect(resp.Patch).To(Equal([]byte(`[{"op":"add","path":"/a","value":2},{"op":"replace","path":"/b","value":4}]`)))
	})

	Describe("dependency injection", func() {
		It("should set dependencies passed in on the handler", func() {
			By("setting up a webhook and injecting it with a injection func that injects a string")
			setFields := func(target interface{}) error {
				inj, ok := target.(stringInjector)
				if !ok {
					return nil
				}

				return inj.InjectString("something")
			}
			handler := &fakeHandler{}
			webhook := &Webhook{
				Handler: handler,
				log:     logf.RuntimeLog.WithName("webhook"),
			}
			Expect(setFields(webhook)).To(Succeed())
			Expect(inject.InjectorInto(setFields, webhook)).To(BeTrue())

			By("checking that the string was injected")
			Expect(handler.injectedString).To(Equal("something"))
		})

		It("should inject a decoder into the handler", func() {
			By("setting up a webhook and injecting it with a injection func that injects a scheme")
			setFields := func(target interface{}) error {
				if _, err := inject.SchemeInto(runtime.NewScheme(), target); err != nil {
					return err
				}
				return nil
			}
			handler := &fakeHandler{}
			webhook := &Webhook{
				Handler: handler,
				log:     logf.RuntimeLog.WithName("webhook"),
			}
			Expect(setFields(webhook)).To(Succeed())
			Expect(inject.InjectorInto(setFields, webhook)).To(BeTrue())

			By("checking that the decoder was injected")
			Expect(handler.decoder).NotTo(BeNil())
		})

		It("should pass a setFields that also injects a decoder into sub-dependencies", func() {
			By("setting up a webhook and injecting it with a injection func that injects a scheme")
			setFields := func(target interface{}) error {
				if _, err := inject.SchemeInto(runtime.NewScheme(), target); err != nil {
					return err
				}
				return nil
			}
			handler := &handlerWithSubDependencies{
				Handler: HandlerFunc(func(ctx context.Context, req Request) Response {
					return Response{}
				}),
				dep: &subDep{},
			}
			webhook := &Webhook{
				Handler: handler,
			}
			Expect(setFields(webhook)).To(Succeed())
			Expect(inject.InjectorInto(setFields, webhook)).To(BeTrue())

			By("checking that setFields sets the decoder as well")
			Expect(handler.dep.decoder).NotTo(BeNil())
		})
	})

	Describe("panic recovery", func() {
		It("should recover panic if RecoverPanic is true", func() {
			panicHandler := func() *Webhook {
				handler := &fakeHandler{
					fn: func(ctx context.Context, req Request) Response {
						panic("injected panic")
					},
				}
				webhook := &Webhook{
					Handler:      handler,
					RecoverPanic: true,
					log:          logf.RuntimeLog.WithName("webhook"),
				}

				return webhook
			}

			By("setting up a webhook with a panicking handler")
			webhook := panicHandler()

			By("invoking the webhook")
			resp := webhook.Handle(context.Background(), Request{})

			By("checking that it errored the request")
			Expect(resp.Allowed).To(BeFalse())
			Expect(resp.Result.Code).To(Equal(int32(http.StatusInternalServerError)))
			Expect(resp.Result.Message).To(Equal("panic: injected panic [recovered]"))
		})

		It("should not recover panic if RecoverPanic is false by default", func() {
			panicHandler := func() *Webhook {
				handler := &fakeHandler{
					fn: func(ctx context.Context, req Request) Response {
						panic("injected panic")
					},
				}
				webhook := &Webhook{
					Handler: handler,
					log:     logf.RuntimeLog.WithName("webhook"),
				}

				return webhook
			}

			By("setting up a webhook with a panicking handler")
			defer func() {
				Expect(recover()).ShouldNot(BeNil())
			}()
			webhook := panicHandler()

			By("invoking the webhook")
			webhook.Handle(context.Background(), Request{})
		})
	})
})

var _ = Describe("Should be able to write/read admission.Request to/from context", func() {
	ctx := context.Background()
	testRequest := Request{
		admissionv1.AdmissionRequest{
			UID: "test-uid",
		},
	}

	ctx = NewContextWithRequest(ctx, testRequest)

	gotRequest, err := RequestFromContext(ctx)
	Expect(err).To(Not(HaveOccurred()))
	Expect(gotRequest).To(Equal(testRequest))
})

type stringInjector interface {
	InjectString(s string) error
}

type handlerWithSubDependencies struct {
	Handler
	dep *subDep
}

func (h *handlerWithSubDependencies) InjectFunc(f inject.Func) error {
	return f(h.dep)
}

type subDep struct {
	decoder *Decoder
}

func (d *subDep) InjectDecoder(dec *Decoder) error {
	d.decoder = dec
	return nil
}
