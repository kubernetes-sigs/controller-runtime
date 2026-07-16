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

package admission_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/webhookauth/verify"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	testIssuer   = "https://issuer.example.com"
	testAudience = "webhook.example.com"

	// authFailureToken is a raw token that is NOT a decodable claims payload, so
	// fakeAuthenticator's json.Unmarshal fails and it returns an error. In the v3
	// seam the core verify.Verifier no longer checks the signature or the standard
	// iss/aud/exp claims — those live in the TokenAuthenticator — so this stands
	// in for the real authenticator rejecting an expired / wrong-audience /
	// bad-signature token. The Verifier collapses that into its single generic
	// failure and the adapter fails closed.
	authFailureToken = "simulated-signature-or-standard-claim-failure"
)

// fakeAuthenticator is a stand-in verify.TokenAuthenticator that keeps the test
// pure-stdlib and offline. In the v3 seam AuthenticateToken returns the token's
// allowedAPIGroup values (or an error simulating a signature / iss / aud / exp
// failure); the webhook-binding and exactly-one-of-(validating|mutating) checks
// now live inside the real authenticator, so they are not exercised through this
// fake. It decodes the raw token as a JSON payload carrying the allowed groups; a
// json.Unmarshal failure plays the role of the real authenticator rejecting a
// token whose signature or standard claims did not verify.
type fakeAuthenticator struct{}

func (fakeAuthenticator) AuthenticateToken(_ context.Context, rawToken string) ([]string, error) {
	var payload struct {
		AllowedAPIGroups []string `json:"allowedAPIGroups"`
	}
	if err := json.Unmarshal([]byte(rawToken), &payload); err != nil {
		return nil, fmt.Errorf("simulated authentication failure: %w", err)
	}
	return payload.AllowedAPIGroups, nil
}

// mkToken mints a token string for the offline fake: a JSON payload carrying the
// allowedAPIGroup values fakeAuthenticator returns. In the v3 seam the
// signature / iss / aud / exp checks and the webhook-binding checks live inside
// the real authenticator, so this offline fake models only the allowedAPIGroup
// list the Verifier matches against the review's group.
func mkToken(t *testing.T, group string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"allowedAPIGroups": []string{group},
	})
	if err != nil {
		t.Fatalf("marshal token payload: %v", err)
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

func TestNewAuthenticator(t *testing.T) {
	validApps := mkToken(t, "apps")
	wildcard := mkToken(t, "*")

	// TODO(kep-6060): rebuild bothBound / noBound / mutatingApps binding-violation
	// coverage under the v3 seam (commit 2, post-review). In v3 the
	// exactly-one-of-(validating|mutating) webhook-binding checks moved INTO the
	// real authenticator; the offline fakeAuthenticator only returns the
	// allowedAPIGroup list, so those cases cannot be exercised through this
	// Verifier fake without replicating the binding logic. They are dropped here
	// to restore compilation and re-added with real v3 semantics in commit 2.

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
			// Stands in for expired / wrong-audience / bad-signature: in v3 those
			// checks live in the authenticator, which here returns an error.
			name:        "authenticator error",
			setAuthHdr:  true,
			token:       authFailureToken,
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

	auth := admission.NewAuthenticator(newVerifier(t))

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

// TestNewAuthenticatorNilVerifierFailsClosed ensures a misconfigured
// authenticator (nil verifier) denies rather than panicking or allowing.
func TestNewAuthenticatorNilVerifierFailsClosed(t *testing.T) {
	auth := admission.NewAuthenticator(nil)
	httpReq := httptest.NewRequest(http.MethodPost, "/", nil)
	httpReq.Header.Set("Authorization", "Bearer "+mkToken(t, "apps"))

	resp := auth.Authenticate(context.Background(), httpReq, reqForGroup("apps"))
	if resp.Allowed {
		t.Fatal("nil verifier must deny")
	}
}

// TestWithAuthenticatorEndToEnd drives a verifier-backed authenticator through
// the real admission.Webhook.ServeHTTP pipeline (decode -> Authenticate ->
// Handle), proving the hook short-circuits an unauthenticated request before the
// handler and reuses controller-runtime's single AdmissionReview decode.
func TestWithAuthenticatorEndToEnd(t *testing.T) {
	v := newVerifier(t)

	var reached bool
	spy := admission.HandlerFunc(func(_ context.Context, _ admission.Request) admission.Response {
		reached = true
		return admission.Allowed("")
	})

	wh := (&admission.Webhook{
		Handler: spy,
	}).WithAuthenticator(admission.NewAuthenticator(v))
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
			req.Header.Set("Authorization", "Bearer "+mkToken(t, "apps"))
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

// TestWithInClusterAuthenticatorErrorPropagation exercises the fail-fast method.
// In a unit-test environment there is no projected service-account token at
// /var/run/secrets/kubernetes.io/serviceaccount/token, so oidc.InCluster fails
// and the method must surface that error (and a nil Webhook).
//
// The happy path performs live OIDC discovery and background JWKS refresh, so it
// is intentionally not tested here — it is covered by the library's own tests and
// by the compile-time examples. We do not stand up a real OIDC server.
func TestWithInClusterAuthenticatorErrorPropagation(t *testing.T) {
	wh, err := (&admission.Webhook{}).WithInClusterAuthenticator(context.Background())
	if err == nil {
		t.Fatalf("expected an error with no projected service-account token, got nil (wh=%v)", wh)
	}
	if wh != nil {
		t.Fatalf("expected a nil Webhook on error, got %v", wh)
	}
}

// TestWithRemoteAuthenticatorErrorPropagation exercises the explicit
// issuer/audience path. A bogus, unreachable issuer combined with an
// already-expired context makes OIDC discovery fail fast, so the method must
// surface that error (and a nil Webhook) without any network round-trip
// completing.
func TestWithRemoteAuthenticatorErrorPropagation(t *testing.T) {
	// Already-cancelled context so discovery fails immediately rather than
	// waiting on a real network dial.
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()

	wh, err := (&admission.Webhook{}).WithRemoteAuthenticator(ctx, "https://issuer.invalid", testAudience, nil)
	if err == nil {
		t.Fatalf("expected an error for an unreachable issuer, got nil (wh=%v)", wh)
	}
	if wh != nil {
		t.Fatalf("expected a nil Webhook on error, got %v", wh)
	}
}

// TestWithRemoteAuthenticatorValidatesArgs confirms the empty-issuer/empty-audience
// guards in the underlying oidc.NewRemoteVerifier propagate through the method.
// This path is fully offline (the guards reject before any discovery).
func TestWithRemoteAuthenticatorValidatesArgs(t *testing.T) {
	for _, tc := range []struct {
		name     string
		issuer   string
		audience string
	}{
		{name: "empty issuer", issuer: "", audience: testAudience},
		{name: "empty audience", issuer: testIssuer, audience: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wh, err := (&admission.Webhook{}).WithRemoteAuthenticator(context.Background(), tc.issuer, tc.audience, nil)
			if err == nil {
				t.Fatalf("expected an error for %s, got nil (wh=%v)", tc.name, wh)
			}
			if wh != nil {
				t.Fatalf("expected a nil Webhook on error, got %v", wh)
			}
		})
	}
}
