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

package webhookauth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/webhook-auth/verify"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/webhookauth"
)

const (
	testIssuer   = "https://issuer.example.com"
	testAudience = "webhook.example.com"
)

// fakeKeySet treats the raw token string as the already-signature-verified JSON
// claims payload and returns it verbatim. This is the stdlib-only fake from the
// k8s.io/webhook-auth examples: "signing" is just JSON marshaling, so the test
// carries no JOSE/OIDC dependency.
type fakeKeySet struct{}

func (fakeKeySet) VerifySignature(_ context.Context, rawToken string) ([]byte, error) {
	return []byte(rawToken), nil
}

// mkToken mints a token string (== JSON claims payload) matching the KEP-6060
// contract shape used in the library's admissionhttp example_test.go.
func mkToken(aud []string, exp time.Time, group string) string {
	claims := map[string]interface{}{
		"iss": testIssuer,
		"aud": aud,
		"exp": exp.Unix(),
		"kubernetes.io": map[string]interface{}{
			"validatingWebhookConfiguration": map[string]string{
				"name": "my-webhook",
				"uid":  "webhook-uid",
			},
			"attestationClaims": map[string][]string{
				verify.AllowedAPIGroupClaimKey: {group},
			},
		},
	}
	payload, _ := json.Marshal(claims)
	return string(payload)
}

func newVerifier(t *testing.T) *verify.Verifier {
	t.Helper()
	v, err := verify.NewVerifier(fakeKeySet{}, testIssuer, []string{testAudience})
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	return v
}

func reqForGroup(group string) admission.Request {
	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID:      "req-uid",
			Resource: metav1.GroupVersionResource{Group: group, Version: "v1", Resource: "deployments"},
			Name:     "my-deploy",
		},
	}
}

func TestAuthenticate(t *testing.T) {
	future := time.Now().Add(5 * time.Minute)
	past := time.Now().Add(-5 * time.Minute)

	tests := []struct {
		name        string
		setAuthHdr  bool
		authHeader  string
		token       string
		reqGroup    string
		wantAllowed bool
	}{
		{
			name:        "valid token, matching group and audience, future exp",
			setAuthHdr:  true,
			token:       mkToken([]string{testAudience}, future, "apps"),
			reqGroup:    "apps",
			wantAllowed: true,
		},
		{
			name:        "missing token",
			setAuthHdr:  false,
			reqGroup:    "apps",
			wantAllowed: false,
		},
		{
			name:        "wrong scheme",
			setAuthHdr:  true,
			authHeader:  "Basic abc",
			reqGroup:    "apps",
			wantAllowed: false,
		},
		{
			name:        "expired token",
			setAuthHdr:  true,
			token:       mkToken([]string{testAudience}, past, "apps"),
			reqGroup:    "apps",
			wantAllowed: false,
		},
		{
			name:        "wrong audience",
			setAuthHdr:  true,
			token:       mkToken([]string{"someone.else"}, future, "apps"),
			reqGroup:    "apps",
			wantAllowed: false,
		},
		{
			name:        "group mismatch",
			setAuthHdr:  true,
			token:       mkToken([]string{testAudience}, future, "apps"),
			reqGroup:    "batch",
			wantAllowed: false,
		},
	}

	auth := webhookauth.NewAuthenticator(newVerifier(t))

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			httpReq := httptest.NewRequest(http.MethodPost, "/", nil)
			if tc.setAuthHdr {
				header := tc.authHeader
				if header == "" {
					header = "Bearer " + tc.token
				}
				httpReq.Header.Set("Authorization", header)
			}

			resp := auth.Authenticate(context.Background(), httpReq, reqForGroup(tc.reqGroup))
			if resp.Allowed != tc.wantAllowed {
				t.Fatalf("Authenticate allowed=%v, want %v", resp.Allowed, tc.wantAllowed)
			}
			if !resp.Allowed {
				if resp.Result == nil || resp.Result.Code != http.StatusUnauthorized {
					t.Fatalf("denied response = %#v, want 401", resp.Result)
				}
				if strings.Contains(resp.Result.Message, tc.token) && tc.token != "" {
					t.Fatalf("response leaked token material: %q", resp.Result.Message)
				}
			}
		})
	}
}

// TestNilVerifierFailsClosed ensures a misconfigured authenticator (nil verifier)
// denies rather than panicking or allowing.
func TestNilVerifierFailsClosed(t *testing.T) {
	auth := webhookauth.NewAuthenticator(nil)
	httpReq := httptest.NewRequest(http.MethodPost, "/", nil)
	httpReq.Header.Set("Authorization", "Bearer "+mkToken([]string{testAudience}, time.Now().Add(time.Minute), "apps"))

	resp := auth.Authenticate(context.Background(), httpReq, reqForGroup("apps"))
	if resp.Allowed {
		t.Fatal("nil verifier must deny")
	}
}

// TestAuthenticatorEndToEnd drives the authenticator through the real
// admission.Webhook.ServeHTTP pipeline (decode -> Authenticate -> Handle),
// proving the hook short-circuits an unauthenticated request before the handler
// and reuses controller-runtime's single AdmissionReview decode.
func TestAuthenticatorEndToEnd(t *testing.T) {
	v := newVerifier(t)

	var reached bool
	spy := admission.HandlerFunc(func(_ context.Context, _ admission.Request) admission.Response {
		reached = true
		return admission.Allowed("")
	})

	wh := &admission.Webhook{
		Handler:       spy,
		Authenticator: webhookauth.NewAuthenticator(v),
	}
	srv := httptest.NewServer(wh)
	defer srv.Close()

	review := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"},
		Request: &admissionv1.AdmissionRequest{
			UID:      "req-uid",
			Resource: metav1.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
			Name:     "my-deploy",
		},
	}
	body, err := json.Marshal(review)
	if err != nil {
		t.Fatalf("marshal review: %v", err)
	}

	post := func(withToken bool) admissionv1.AdmissionReview {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(body))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if withToken {
			req.Header.Set("Authorization", "Bearer "+mkToken([]string{testAudience}, time.Now().Add(5*time.Minute), "apps"))
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		defer resp.Body.Close()
		var out admissionv1.AdmissionReview
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return out
	}

	t.Run("valid token reaches handler and is allowed", func(t *testing.T) {
		reached = false
		out := post(true)
		if out.Response == nil || !out.Response.Allowed {
			t.Fatalf("expected Allowed=true, got %+v", out.Response)
		}
		if !reached {
			t.Fatal("expected downstream handler to be reached")
		}
	})

	t.Run("missing token is denied before handler", func(t *testing.T) {
		reached = false
		out := post(false)
		if out.Response == nil || out.Response.Allowed {
			t.Fatalf("expected Allowed=false, got %+v", out.Response)
		}
		if reached {
			t.Fatal("expected downstream handler NOT to be reached")
		}
	})
}
