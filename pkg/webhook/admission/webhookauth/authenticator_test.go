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

package webhookauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type fakeVerifier struct {
	called bool
	token  string
	err    error
}

func (f *fakeVerifier) Verify(_ context.Context, token string, _ admission.Request) (*Result, error) {
	f.called = true
	f.token = token
	if f.err != nil {
		return nil, f.err
	}
	return &Result{}, nil
}

func TestAuthenticatorAuthorizationHeaderParsing(t *testing.T) {
	tests := []struct {
		name       string
		headers    []string
		wantCalled bool
		wantToken  string
	}{
		{name: "missing"},
		{name: "wrong scheme", headers: []string{"Basic abc"}},
		{name: "empty bearer", headers: []string{"Bearer"}},
		{name: "extra fields", headers: []string{"Bearer abc def"}},
		{name: "multiple values", headers: []string{"Bearer abc", "Bearer def"}},
		{name: "valid bearer", headers: []string{"Bearer abc.def.ghi"}, wantCalled: true, wantToken: "abc.def.ghi"},
		{name: "case-insensitive scheme", headers: []string{"bearer abc.def.ghi"}, wantCalled: true, wantToken: "abc.def.ghi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifier := &fakeVerifier{}
			authenticator := NewAuthenticator(verifier)
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			for _, header := range tt.headers {
				req.Header.Add("Authorization", header)
			}

			resp := authenticator.Authenticate(context.Background(), req, admission.Request{})

			if verifier.called != tt.wantCalled {
				t.Fatalf("verifier called = %t, want %t", verifier.called, tt.wantCalled)
			}
			if verifier.token != tt.wantToken {
				t.Fatalf("verifier token = %q, want %q", verifier.token, tt.wantToken)
			}
			if tt.wantCalled && !resp.Allowed {
				t.Fatalf("Authenticate() denied unexpectedly: %#v", resp.Result)
			}
			if !tt.wantCalled && (resp.Allowed || resp.Result.Code != http.StatusUnauthorized) {
				t.Fatalf("Authenticate() = %#v, want 401 denial", resp)
			}
		})
	}
}

func TestAuthenticatorMapsVerifierErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		code   int32
		reason metav1.StatusReason
	}{
		{name: "unauthenticated", err: unauthenticated("bad token"), code: http.StatusUnauthorized, reason: metav1.StatusReasonUnauthorized},
		{name: "unauthorized", err: unauthorized("wrong group"), code: http.StatusForbidden, reason: metav1.StatusReasonForbidden},
		{name: "unknown", err: errors.New("boom"), code: http.StatusUnauthorized, reason: metav1.StatusReasonUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifier := &fakeVerifier{err: tt.err}
			authenticator := NewAuthenticator(verifier)
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("Authorization", "Bearer secret-token")

			resp := authenticator.Authenticate(context.Background(), req, admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{UID: "uid"},
			})

			if resp.Allowed || resp.Result.Code != tt.code || resp.Result.Reason != tt.reason {
				t.Fatalf("Authenticate() = %#v, want %d %s denial", resp, tt.code, tt.reason)
			}
			if strings.Contains(resp.Result.Message, "secret-token") {
				t.Fatalf("response leaked token material: %q", resp.Result.Message)
			}
		})
	}
}

func TestVerifierAllowsCoreAPIGroup(t *testing.T) {
	f := newFixture(t)
	token := f.token(t, tokenOptions{allowedAPIGroup: []string{""}})

	result, err := f.verifier.Verify(context.Background(), token, admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Resource: metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		},
	})

	if err != nil {
		t.Fatalf("Verify() unexpected error: %v", err)
	}
	if result.AllowedAPIGroup != "" {
		t.Fatalf("AllowedAPIGroup = %q, want core API group", result.AllowedAPIGroup)
	}
}
