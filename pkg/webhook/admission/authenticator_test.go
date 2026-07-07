/*
Copyright 2026 The Kubernetes Authors.

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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("Admission webhook authenticators", func() {
	const (
		gvkJSONv1      = `"kind":"AdmissionReview","apiVersion":"admission.k8s.io/v1"`
		gvkJSONv1beta1 = `"kind":"AdmissionReview","apiVersion":"admission.k8s.io/v1beta1"`
	)

	It("preserves existing behavior when no authenticator is configured", func() {
		body := fmt.Sprintf(`{%s,"request":{}}`, gvkJSONv1)
		newWebhook := func() *Webhook {
			return &Webhook{
				Handler: &fakeHandler{},
			}
		}

		reqWithoutAuth := newAdmissionReviewHTTPRequest(body)
		reqWithAuth := newAdmissionReviewHTTPRequest(body)
		reqWithAuth.Header.Set("Authorization", "Bearer ignored")

		withoutAuthRecorder := httptest.NewRecorder()
		withAuthRecorder := httptest.NewRecorder()

		newWebhook().ServeHTTP(withoutAuthRecorder, reqWithoutAuth)
		newWebhook().ServeHTTP(withAuthRecorder, reqWithAuth)

		Expect(withAuthRecorder.Body.String()).To(Equal(withoutAuthRecorder.Body.String()))
	})

	It("runs the authenticator after decode and before the handler", func() {
		events := []string{}
		handler := &fakeHandler{
			fn: func(ctx context.Context, req Request) Response {
				events = append(events, "handler")
				return Allowed("handled")
			},
		}
		webhook := &Webhook{
			Handler: handler,
			Authenticator: authenticatorFunc(func(ctx context.Context, httpReq *http.Request, req Request) Response {
				events = append(events, "authenticator")
				Expect(httpReq.Header.Get("Authorization")).To(Equal("Bearer token"))
				Expect(req.Resource.Group).To(Equal("apps"))
				return Allowed("")
			}),
		}

		req := newAdmissionReviewHTTPRequest(fmt.Sprintf(`{%s,"request":{"resource":{"group":"apps","version":"v1","resource":"deployments"}}}`, gvkJSONv1))
		req.Header.Set("Authorization", "Bearer token")
		respRecorder := httptest.NewRecorder()

		webhook.ServeHTTP(respRecorder, req)

		Expect(events).To(Equal([]string{"authenticator", "handler"}))
		Expect(handler.invoked).To(BeTrue())
	})

	DescribeTable("short-circuits denied authentication responses with the request AdmissionReview version",
		func(authResponse Response, expectedCode int32, expectedReason metav1.StatusReason, expectedMessage string) {
			handler := &fakeHandler{}
			webhook := &Webhook{
				Handler: handler,
				Authenticator: authenticatorFunc(func(ctx context.Context, httpReq *http.Request, req Request) Response {
					Expect(req.Resource.Group).To(Equal("apps"))
					return authResponse
				}),
			}
			req := newAdmissionReviewHTTPRequest(fmt.Sprintf(`{%s,"request":{"uid":"auth-uid","resource":{"group":"apps","version":"v1","resource":"deployments"}}}`, gvkJSONv1beta1))
			respRecorder := httptest.NewRecorder()

			webhook.ServeHTTP(respRecorder, req)

			Expect(handler.invoked).To(BeFalse())
			var review admissionv1.AdmissionReview
			Expect(json.Unmarshal(respRecorder.Body.Bytes(), &review)).To(Succeed())
			Expect(review.APIVersion).To(Equal("admission.k8s.io/v1beta1"))
			Expect(review.Kind).To(Equal("AdmissionReview"))
			Expect(review.Response).NotTo(BeNil())
			Expect(string(review.Response.UID)).To(Equal("auth-uid"))
			Expect(review.Response.Allowed).To(BeFalse())
			Expect(review.Response.Result).NotTo(BeNil())
			Expect(review.Response.Result.Code).To(Equal(expectedCode))
			Expect(review.Response.Result.Reason).To(Equal(expectedReason))
			Expect(review.Response.Result.Message).To(Equal(expectedMessage))
		},
		Entry("unauthenticated", Unauthenticated("missing bearer token"), int32(http.StatusUnauthorized), metav1.StatusReasonUnauthorized, "missing bearer token"),
		Entry("unauthorized", Unauthorized("api group is not allowed"), int32(http.StatusForbidden), metav1.StatusReasonForbidden, "api group is not allowed"),
	)

	It("keeps typed, multi-handler, and standalone webhooks compatible", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		constructors := map[string]func(Authenticator) http.Handler{
			"WithValidator": func(authenticator Authenticator) http.Handler {
				webhook := WithValidator[*corev1.Pod](scheme, podValidator{})
				webhook.Authenticator = authenticator
				return webhook
			},
			"WithDefaulter": func(authenticator Authenticator) http.Handler {
				webhook := WithDefaulter[*corev1.Pod](scheme, podDefaulter{})
				webhook.Authenticator = authenticator
				return webhook
			},
			"WithCustomDefaulter": func(authenticator Authenticator) http.Handler {
				webhook := WithCustomDefaulter(scheme, &corev1.Pod{}, customPodDefaulter{})
				webhook.Authenticator = authenticator
				return webhook
			},
			"MultiValidatingHandler": func(authenticator Authenticator) http.Handler {
				return &Webhook{
					Handler:       MultiValidatingHandler(&fakeHandler{}),
					Authenticator: authenticator,
				}
			},
			"MultiMutatingHandler": func(authenticator Authenticator) http.Handler {
				return &Webhook{
					Handler:       MultiMutatingHandler(&fakeHandler{}),
					Authenticator: authenticator,
				}
			},
			"StandaloneWebhook": func(authenticator Authenticator) http.Handler {
				handler, err := StandaloneWebhook(&Webhook{
					Handler:       &fakeHandler{},
					Authenticator: authenticator,
				}, StandaloneOptions{})
				Expect(err).NotTo(HaveOccurred())
				return handler
			},
		}

		for name, constructor := range constructors {
			By(name)
			called := false
			handler := constructor(authenticatorFunc(func(ctx context.Context, httpReq *http.Request, req Request) Response {
				called = true
				return Unauthorized("blocked")
			}))
			req := newAdmissionReviewHTTPRequest(fmt.Sprintf(`{%s,"request":{"uid":"compat-uid"}}`, gvkJSONv1))
			respRecorder := httptest.NewRecorder()

			handler.ServeHTTP(respRecorder, req)

			Expect(called).To(BeTrue())
			var review admissionv1.AdmissionReview
			Expect(json.Unmarshal(respRecorder.Body.Bytes(), &review)).To(Succeed())
			Expect(review.Response).NotTo(BeNil())
			Expect(review.Response.Allowed).To(BeFalse())
			Expect(review.Response.Result.Code).To(Equal(int32(http.StatusForbidden)))
		}
	})
})

type authenticatorFunc func(context.Context, *http.Request, Request) Response

func (f authenticatorFunc) Authenticate(ctx context.Context, httpReq *http.Request, req Request) Response {
	return f(ctx, httpReq, req)
}

func newAdmissionReviewHTTPRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/admission", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

type podValidator struct{}

func (podValidator) ValidateCreate(context.Context, *corev1.Pod) (Warnings, error) {
	return nil, nil
}

func (podValidator) ValidateUpdate(context.Context, *corev1.Pod, *corev1.Pod) (Warnings, error) {
	return nil, nil
}

func (podValidator) ValidateDelete(context.Context, *corev1.Pod) (Warnings, error) {
	return nil, nil
}

type podDefaulter struct{}

func (podDefaulter) Default(context.Context, *corev1.Pod) error {
	return nil
}

type customPodDefaulter struct{}

func (customPodDefaulter) Default(context.Context, runtime.Object) error {
	return nil
}
