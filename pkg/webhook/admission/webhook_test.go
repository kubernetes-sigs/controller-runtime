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

	"github.com/appscode/jsonpatch"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	machinerytypes "k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
)

var _ = Describe("Admission Webhooks", func() {
	allowHandler := func() *Webhook {
		handler := &fakeHandler{
			fn: func(ctx context.Context, req Request) Response {
				return Response{
					AdmissionResponse: admissionv1beta1.AdmissionResponse{
						Allowed: true,
					},
				}
			},
		}
		webhook := &Webhook{
			Handler: handler,
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
		resp := webhook.Handle(context.Background(), Request{AdmissionRequest: admissionv1beta1.AdmissionRequest{UID: "foobar"}})

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
					AdmissionResponse: admissionv1beta1.AdmissionResponse{
						Allowed: true,
						Result:  &metav1.Status{Message: "Ground Control to Major Tom"},
					},
				}
			}),
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
		}

		By("invoking the webhoook")
		resp := webhook.Handle(context.Background(), Request{})

		By("checking that a JSON patch is populated on the response")
		patchType := admissionv1beta1.PatchTypeJSONPatch
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
