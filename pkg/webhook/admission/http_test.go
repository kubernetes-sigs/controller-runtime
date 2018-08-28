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
	"io"
	"net/http"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/types"
)

var _ = Describe("admission webhook http handler", func() {
	Describe("empty request body", func() {
		req := &http.Request{Body: nil}
		wh := &Webhook{
			Handlers: []Handler{},
		}
		w := &fakeResponseWriter{}
		expected := []byte(`{"response":{"uid":"","allowed":false,"status":{"metadata":{},"message":"request body is empty","code":400}}}
`)
		It("should return a response with an error", func() {
			wh.ServeHTTP(w, req)
			Expect(w.response).NotTo(BeNil())
			Expect(w.response).To(Equal(expected))
		})
	})

	Describe("wrong content type", func() {
		req := &http.Request{
			Header: http.Header{"Content-Type": []string{"application/foo"}},
			Body:   nopCloser{Reader: bytes.NewBuffer(nil)},
		}
		wh := &Webhook{
			Handlers: []Handler{},
		}
		w := &fakeResponseWriter{}
		expected := []byte(`{"response":{"uid":"","allowed":false,"status":{"metadata":{},"message":"contentType=application/foo, expect application/json","code":400}}}
`)
		It("should return a response with an error", func() {
			wh.ServeHTTP(w, req)
			Expect(w.response).NotTo(BeNil())
			Expect(w.response).To(Equal(expected))
		})
	})

	Describe("can't decode body", func() {
		req := &http.Request{
			Header: http.Header{"Content-Type": []string{"application/json"}},
			Body:   nopCloser{Reader: bytes.NewBufferString("{")},
		}
		wh := &Webhook{
			Type:     types.WebhookTypeMutating,
			Handlers: []Handler{},
		}
		w := &fakeResponseWriter{}
		expected := []byte(
			`{"response":{"uid":"","allowed":false,"status":{"metadata":{},"message":"couldn't get version/kind; json parse error: unexpected end of JSON input","code":400}}}
`)
		It("should return a response with an error", func() {
			wh.ServeHTTP(w, req)
			Expect(w.response).NotTo(BeNil())
			Expect(w.response).To(Equal(expected))
		})
	})

	Describe("empty body after decoding", func() {
		req := &http.Request{
			Header: http.Header{"Content-Type": []string{"application/json"}},
			Body:   nopCloser{Reader: bytes.NewBuffer(nil)},
		}
		wh := &Webhook{
			Type:     types.WebhookTypeMutating,
			Handlers: []Handler{},
		}
		w := &fakeResponseWriter{}
		expected := []byte(`{"response":{"uid":"","allowed":false,"status":{"metadata":{},"message":"got an empty AdmissionRequest","code":400}}}
`)
		It("should return a response with an error", func() {
			wh.ServeHTTP(w, req)
			Expect(w.response).NotTo(BeNil())
			Expect(w.response).To(Equal(expected))
		})
	})

	Describe("no webhook type", func() {
		req := &http.Request{
			Header: http.Header{"Content-Type": []string{"application/json"}},
			Body:   nopCloser{Reader: bytes.NewBufferString(`{"request":{}}`)},
		}
		wh := &Webhook{
			Handlers: []Handler{},
		}
		w := &fakeResponseWriter{}
		expected := []byte(`{"response":{"uid":"","allowed":false,"status":{"metadata":{},"message":"you must specify your webhook type","code":400}}}
`)
		It("should return a response with an error", func() {
			wh.ServeHTTP(w, req)
			Expect(w.response).NotTo(BeNil())
			Expect(w.response).To(Equal(expected))
		})
	})

	Describe("handler can be invoked", func() {
		req := &http.Request{
			Header: http.Header{"Content-Type": []string{"application/json"}},
			Body:   nopCloser{Reader: bytes.NewBufferString(`{"request":{}}`)},
		}
		h := &fakeHandler{}
		wh := &Webhook{
			Type:     types.WebhookTypeValidating,
			Handlers: []Handler{h},
			KVMap:    map[string]interface{}{"foo": "bar"},
		}
		w := &fakeResponseWriter{}
		expected := []byte(`{"response":{"uid":"","allowed":true}}
`)
		It("should return a response successfully", func() {
			wh.ServeHTTP(w, req)
			Expect(w.response).NotTo(BeNil())
			Expect(w.response).To(Equal(expected))
			Expect(h.invoked).To(BeTrue())
			Expect(h.valueFromContext).To(Equal("bar"))
		})
	})
})

type fakeResponseWriter struct {
	response []byte
}

var _ http.ResponseWriter = &fakeResponseWriter{}

func (w *fakeResponseWriter) Header() http.Header {
	return nil
}

func (w *fakeResponseWriter) Write(resp []byte) (int, error) {
	w.response = append(w.response, resp...)
	return len(resp), nil
}

func (w *fakeResponseWriter) WriteHeader(statusCode int) {}

type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

type fakeHandler struct {
	invoked          bool
	valueFromContext string
	fn               func(context.Context, Request) Response
}

func (h *fakeHandler) Handle(ctx context.Context, req Request) Response {
	v := ctx.Value(ContextKey("foo"))
	if v != nil {
		typed, ok := v.(string)
		if ok {
			h.valueFromContext = typed
		}
	}
	h.invoked = true
	if h.fn != nil {
		return h.fn(ctx, req)
	}
	return Response{Response: &admissionv1beta1.AdmissionResponse{
		Allowed: true,
	}}
}
