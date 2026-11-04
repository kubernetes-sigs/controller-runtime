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

package authentication

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/prometheus/client_golang/prometheus/testutil"
	authenticationv1 "k8s.io/api/authentication/v1"
	machinerytypes "k8s.io/apimachinery/pkg/types"

	authenticationmetrics "sigs.k8s.io/controller-runtime/pkg/webhook/authentication/metrics"
)

var _ = Describe("Authentication Webhooks", func() {
	allowHandler := func() *Webhook {
		handler := &fakeHandler{
			fn: func(ctx context.Context, req Request) Response {
				return Response{
					Status: authenticationv1.TokenReviewStatus{
						Authenticated: true,
					},
				}
			},
		}
		webhook := &Webhook{
			Handler: handler,
		}

		return webhook
	}

	It("should invoke the handler to get a response", func(ctx SpecContext) {
		By("setting up a webhook with an allow handler")
		webhook := allowHandler()

		By("invoking the webhook")
		resp := webhook.Handle(ctx, Request{})

		By("checking that it allowed the request")
		Expect(resp.Status.Authenticated).To(BeTrue())
	})

	It("should ensure that the response's UID is set to the request's UID", func(ctx SpecContext) {
		By("setting up a webhook")
		webhook := allowHandler()

		By("invoking the webhook")
		resp := webhook.Handle(ctx, Request{UID: "foobar"})

		By("checking that the response share's the request's UID")
		Expect(resp.UID).To(Equal(machinerytypes.UID("foobar")))
	})

	It("should populate the status on a response if one is not provided", func(ctx SpecContext) {
		By("setting up a webhook")
		webhook := allowHandler()

		By("invoking the webhook")
		resp := webhook.Handle(ctx, Request{})

		By("checking that the response share's the request's UID")
		Expect(resp.Status).To(Equal(authenticationv1.TokenReviewStatus{Authenticated: true}))
	})

	It("shouldn't overwrite the status on a response", func(ctx SpecContext) {
		By("setting up a webhook that sets a status")
		webhook := &Webhook{
			Handler: HandlerFunc(func(ctx context.Context, req Request) Response {
				return Response{
					Status: authenticationv1.TokenReviewStatus{
						Authenticated: true,
						Error:         "Ground Control to Major Tom",
					},
				}
			}),
		}

		By("invoking the webhook")
		resp := webhook.Handle(ctx, Request{})

		By("checking that the message is intact")
		Expect(resp.Status).NotTo(BeNil())
		Expect(resp.Status.Authenticated).To(BeTrue())
		Expect(resp.Status.Error).To(Equal("Ground Control to Major Tom"))
	})

	Describe("panic recovery", func() {
		panicWebhook := func(recoverPanic *bool) *Webhook {
			return &Webhook{
				Handler: &fakeHandler{
					fn: func(ctx context.Context, req Request) Response {
						panic("fake panic test")
					},
				},
				RecoverPanic: recoverPanic,
			}
		}

		It("should recover panic if RecoverPanic is unset (default)", func(ctx SpecContext) {
			before := testutil.ToFloat64(authenticationmetrics.WebhookPanics)

			resp := panicWebhook(nil).Handle(ctx, Request{UID: "test-uid"})
			Expect(resp.Status.Authenticated).To(BeFalse())
			Expect(resp.Status.Error).To(Equal("panic: fake panic test [recovered]"))
			Expect(resp.UID).To(Equal(machinerytypes.UID("test-uid")))
			Expect(testutil.ToFloat64(authenticationmetrics.WebhookPanics)).To(Equal(before + 1))
		})

		It("should recover panic if RecoverPanic is true", func(ctx SpecContext) {
			before := testutil.ToFloat64(authenticationmetrics.WebhookPanics)

			resp := panicWebhook(new(true)).Handle(ctx, Request{UID: "test-uid"})
			Expect(resp.Status.Authenticated).To(BeFalse())
			Expect(resp.Status.Error).To(Equal("panic: fake panic test [recovered]"))
			Expect(resp.UID).To(Equal(machinerytypes.UID("test-uid")))
			Expect(testutil.ToFloat64(authenticationmetrics.WebhookPanics)).To(Equal(before + 1))
		})

		It("should not recover panic if RecoverPanic is false", func(ctx SpecContext) {
			before := testutil.ToFloat64(authenticationmetrics.WebhookPanics)

			Expect(func() {
				panicWebhook(new(false)).Handle(ctx, Request{})
			}).To(PanicWith("fake panic test"))

			Expect(testutil.ToFloat64(authenticationmetrics.WebhookPanics)).To(Equal(before + 1))
		})
	})
})
