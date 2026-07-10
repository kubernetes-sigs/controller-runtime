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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/webhook-auth/verify"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/webhookauth"
)

const (
	testIssuer   = "https://issuer.example.com"
	testAudience = "webhook.example.com"

	// allowedAPIGroupClaimKey mirrors the (unexported) claim key in
	// k8s.io/webhook-auth/verify; external consumers must hardcode the literal.
	allowedAPIGroupClaimKey = "webhook-authentication.k8s.io/allowedAPIGroup"

	// authFailureToken is a raw token that is NOT a decodable claims payload, so
	// fakeAuthenticator's json.Unmarshal fails and it returns an error. In the v2
	// seam the core verify.Verifier no longer checks the signature or the standard
	// iss/aud/exp claims — those move into the TokenAuthenticator — so this stands
	// in for the real authenticator rejecting an expired / wrong-audience /
	// bad-signature token. The Verifier collapses that into its single generic
	// failure and the adapter fails closed.
	authFailureToken = "simulated-signature-or-standard-claim-failure"
)

// fakeAuthenticator is a stand-in verify.TokenAuthenticator that keeps the test
// pure-stdlib and offline. It treats the raw token as the already
// signature-verified JSON claims payload and decodes it into a
// *verify.VerifiedClaims — the only supported way to populate the unexported
// Kubernetes claims from outside the library. A json.Unmarshal failure plays the
// role of the real authenticator rejecting a token whose signature or standard
// claims (iss/aud/exp) did not verify.
type fakeAuthenticator struct{}

func (fakeAuthenticator) AuthenticateToken(_ context.Context, rawToken string) (*verify.VerifiedClaims, error) {
	var claims verify.VerifiedClaims
	if err := json.Unmarshal([]byte(rawToken), &claims); err != nil {
		return nil, fmt.Errorf("simulated authentication failure: %w", err)
	}
	return &claims, nil
}

// mkToken mints a token string (== JSON claims payload) matching the KEP-6060
// contract. Only the "kubernetes.io" object is consulted by fakeAuthenticator's
// decode (and thus by the core policy); iss/sub/aud are carried for realism and
// are inert here because the real signature/standard-claim checks live in the
// authenticator, which this fake simulates. mutate customizes the
// "kubernetes.io" claims before marshaling; nil leaves the valid baseline
// (a single validating-webhook binding authorized for group).
func mkToken(t *testing.T, group string, mutate func(k8s map[string]any)) string {
	t.Helper()
	k8s := map[string]any{
		"validatingWebhookConfiguration": map[string]string{
			"name": "my-webhook",
			"uid":  "webhook-uid",
		},
		"attestationClaims": map[string][]string{
			allowedAPIGroupClaimKey: {group},
		},
	}
	if mutate != nil {
		mutate(k8s)
	}
	claims := map[string]any{
		"iss":           testIssuer,
		"sub":           "system:serviceaccount:kube-system:webhook",
		"aud":           []string{testAudience},
		"kubernetes.io": k8s,
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	return string(payload)
}

func newVerifier(t *testing.T) *verify.Verifier {
	t.Helper()
	v, err := verify.NewVerifier(fakeAuthenticator{})
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
	validApps := mkToken(t, "apps", nil)
	wildcard := mkToken(t, "*", nil)
	mutatingApps := mkToken(t, "apps", func(k8s map[string]any) {
		// Exactly one bound object, but mutating instead of validating: the
		// exactly-one rule treats the two symmetrically, so this must be accepted.
		delete(k8s, "validatingWebhookConfiguration")
		k8s["mutatingWebhookConfiguration"] = map[string]string{"name": "mwc", "uid": "mwc-uid"}
	})
	bothBound := mkToken(t, "apps", func(k8s map[string]any) {
		// Two bound objects violates the exactly-one rule.
		k8s["mutatingWebhookConfiguration"] = map[string]string{"name": "mwc", "uid": "mwc-uid"}
	})
	noBound := mkToken(t, "apps", func(k8s map[string]any) {
		// Zero bound objects violates the exactly-one rule.
		delete(k8s, "validatingWebhookConfiguration")
	})

	tests := []struct {
		name        string
		setAuthHdr  bool
		authHeader  string
		token       string
		reqGroup    string
		wantAllowed bool
	}{
		{
			name:        "valid token, matching group",
			setAuthHdr:  true,
			token:       validApps,
			reqGroup:    "apps",
			wantAllowed: true,
		},
		{
			name:        "wildcard allowedAPIGroup matches any review group",
			setAuthHdr:  true,
			token:       wildcard,
			reqGroup:    "batch",
			wantAllowed: true,
		},
		{
			name:        "valid token bound to mutating webhook, matching group",
			setAuthHdr:  true,
			token:       mutatingApps,
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
			// A "Bearer" scheme with no token must be treated as absent, not as an
			// empty-string token handed to the verifier.
			name:        "empty bearer token",
			setAuthHdr:  true,
			authHeader:  "Bearer ",
			reqGroup:    "apps",
			wantAllowed: false,
		},
		{
			// Stands in for expired / wrong-audience / bad-signature: in v2 those
			// checks live in the authenticator, which here returns an error.
			name:        "authenticator error",
			setAuthHdr:  true,
			token:       authFailureToken,
			reqGroup:    "apps",
			wantAllowed: false,
		},
		{
			name:        "bound-object violation: both validating and mutating",
			setAuthHdr:  true,
			token:       bothBound,
			reqGroup:    "apps",
			wantAllowed: false,
		},
		{
			name:        "bound-object violation: neither validating nor mutating",
			setAuthHdr:  true,
			token:       noBound,
			reqGroup:    "apps",
			wantAllowed: false,
		},
		{
			name:        "allowedAPIGroup mismatch",
			setAuthHdr:  true,
			token:       validApps,
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
				if tc.token != "" && strings.Contains(resp.Result.Message, tc.token) {
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
	httpReq.Header.Set("Authorization", "Bearer "+mkToken(t, "apps", nil))

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
			req.Header.Set("Authorization", "Bearer "+mkToken(t, "apps", nil))
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
