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
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	testIssuer   = "https://issuer.example.test"
	testAudience = "webhook.example.test/validating/widgets"
	testSubject  = "system:serviceaccount:kube-system:kube-apiserver"
	testWebhook  = "widgets.example.test"
	testUID      = "12345"
	testKid      = "kid-1"
)

type staticKeySet struct{ keys jose.JSONWebKeySet }

func (s staticKeySet) Keys(_ context.Context, keyID string) ([]jose.JSONWebKey, error) {
	if keyID == "" {
		return s.keys.Keys, nil
	}
	return s.keys.Key(keyID), nil
}

func TestVerifierValidToken(t *testing.T) {
	f := newFixture(t)
	result, err := f.verify(t, f.token(t, tokenOptions{}))
	if err != nil {
		t.Fatalf("Verify() unexpected error: %v", err)
	}
	if result.Subject != testSubject || result.AllowedAPIGroup != "apps" {
		t.Fatalf("result = %#v", result)
	}
}

func TestVerifierUnauthenticatedFailures(t *testing.T) {
	f := newFixture(t)
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct{ name, token string }{
		{name: "bad signature", token: f.tokenWithKey(t, otherKey, testKid, jose.RS256, tokenOptions{})},
		{name: "none alg", token: f.noneToken(t)},
		{name: "disallowed alg", token: f.tokenWithKey(t, []byte(strings.Repeat("s", 32)), testKid, jose.HS256, tokenOptions{})},
		{name: "malformed", token: "not-a-jwt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.verify(t, tt.token)
			if !IsUnauthenticated(err) {
				t.Fatalf("Verify() error = %v, want unauthenticated", err)
			}
			if strings.Contains(err.Error(), tt.token) {
				t.Fatalf("error leaked token material: %v", err)
			}
		})
	}
}

func TestVerifierStandardClaimFailures(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name                string
		opts                tokenOptions
		wantUnauthenticated bool
	}{
		{name: "wrong issuer", opts: tokenOptions{issuer: "https://other.example.test"}, wantUnauthenticated: true},
		{name: "expired", opts: tokenOptions{expiry: f.now.Add(-2 * time.Minute)}, wantUnauthenticated: true},
		{name: "not yet valid", opts: tokenOptions{notBefore: f.now.Add(2 * time.Minute)}, wantUnauthenticated: true},
		{name: "wrong audience", opts: tokenOptions{audience: []string{"other-audience"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.verify(t, f.token(t, tt.opts))
			if tt.wantUnauthenticated {
				if !IsUnauthenticated(err) {
					t.Fatalf("Verify() error = %v, want unauthenticated", err)
				}
				return
			}
			if !IsUnauthorized(err) {
				t.Fatalf("Verify() error = %v, want unauthorized", err)
			}
		})
	}
}

func TestVerifierWebhookBindingClaims(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name string
		opts tokenOptions
	}{
		{name: "missing binding", opts: tokenOptions{binding: map[string]any{}}},
		{name: "multiple binding claims", opts: tokenOptions{binding: map[string]any{validatingWebhookConfigurationClaimKey: binding(testWebhook, testUID), mutatingWebhookConfigurationClaimKey: binding(testWebhook, testUID)}}},
		{name: "wrong binding type", opts: tokenOptions{binding: map[string]any{mutatingWebhookConfigurationClaimKey: binding(testWebhook, testUID)}}},
		{name: "wrong binding name", opts: tokenOptions{binding: map[string]any{validatingWebhookConfigurationClaimKey: binding("other", testUID)}}},
		{name: "wrong binding uid", opts: tokenOptions{binding: map[string]any{validatingWebhookConfigurationClaimKey: binding(testWebhook, "other")}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := f.verify(t, f.token(t, tt.opts)); !IsUnauthorized(err) {
				t.Fatalf("Verify() error = %v, want unauthorized", err)
			}
		})
	}
}

func TestVerifierAllowedAPIGroupClaims(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name string
		opts tokenOptions
		want string
	}{
		{name: "missing allowed API group", opts: tokenOptions{allowedAPIGroup: []string{}}},
		{name: "multi valued allowed API group", opts: tokenOptions{allowedAPIGroup: []string{"apps", "batch"}}},
		{name: "wrong allowed API group", opts: tokenOptions{allowedAPIGroup: []string{"batch"}}},
		{name: "wildcard allowed API group", opts: tokenOptions{allowedAPIGroup: []string{allowedAPIGroupWildcard}}, want: allowedAPIGroupWildcard},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := f.verify(t, f.token(t, tt.opts))
			if tt.want != "" {
				if err != nil || result.AllowedAPIGroup != tt.want {
					t.Fatalf("Verify() = %#v, %v; want allowed API group %q", result, err, tt.want)
				}
				return
			}
			if !IsUnauthorized(err) {
				t.Fatalf("Verify() error = %v, want unauthorized", err)
			}
		})
	}
}

type fixture struct {
	now      time.Time
	key      *rsa.PrivateKey
	verifier Verifier
}

type tokenOptions struct {
	issuer          string
	audience        []string
	expiry          time.Time
	notBefore       time.Time
	issuedAt        time.Time
	binding         map[string]any
	allowedAPIGroup []string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 7, 17, 0, 0, 0, time.UTC)
	verifier, err := NewVerifier(Options{
		IssuerURL: testIssuer,
		Audience:  testAudience,
		Binding:   WebhookBinding{Type: ValidatingWebhook, Name: testWebhook, UID: testUID},
		KeySet:    staticKeySet{keys: jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: testKid, Use: "sig", Algorithm: string(jose.RS256)}}}},
		Clock:     ClockFunc(func() time.Time { return now }),
	})
	if err != nil {
		t.Fatal(err)
	}
	return fixture{now: now, key: key, verifier: verifier}
}

func (f fixture) verify(t *testing.T, token string) (*Result, error) {
	t.Helper()
	return f.verifier.Verify(context.Background(), token, admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Resource: metav1.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}})
}

func (f fixture) token(t *testing.T, opts tokenOptions) string {
	t.Helper()
	return f.tokenWithKey(t, f.key, testKid, jose.RS256, opts)
}

func (f fixture) tokenWithKey(t *testing.T, key any, kid string, alg jose.SignatureAlgorithm, opts tokenOptions) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: alg, Key: key}, (&jose.SignerOptions{}).WithHeader(jose.HeaderKey("kid"), kid))
	if err != nil {
		t.Fatal(err)
	}
	issuer := opts.issuer
	if issuer == "" {
		issuer = testIssuer
	}
	audience := opts.audience
	if audience == nil {
		audience = []string{testAudience}
	}
	expiry := opts.expiry
	if expiry.IsZero() {
		expiry = f.now.Add(time.Hour)
	}
	notBefore := opts.notBefore
	if notBefore.IsZero() {
		notBefore = f.now.Add(-time.Minute)
	}
	issuedAt := opts.issuedAt
	if issuedAt.IsZero() {
		issuedAt = f.now.Add(-time.Minute)
	}
	claims := jwt.Claims{Issuer: issuer, Subject: testSubject, Audience: jwt.Audience(audience), Expiry: jwt.NewNumericDate(expiry), NotBefore: jwt.NewNumericDate(notBefore), IssuedAt: jwt.NewNumericDate(issuedAt)}
	token, err := jwt.Signed(signer).Claims(claims).Claims(map[string]any{kubernetesPrivateClaimKey: f.privateKubernetesClaims(opts)}).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func (f fixture) privateKubernetesClaims(opts tokenOptions) map[string]any {
	bindingClaims := opts.binding
	if bindingClaims == nil {
		bindingClaims = map[string]any{validatingWebhookConfigurationClaimKey: binding(testWebhook, testUID)}
	}
	out := map[string]any{}
	for key, value := range bindingClaims {
		out[key] = value
	}
	allowedAPIGroup := opts.allowedAPIGroup
	if allowedAPIGroup == nil {
		allowedAPIGroup = []string{"apps"}
	}
	out[attestationClaimsFieldKey] = map[string][]string{AllowedAPIGroupClaim: allowedAPIGroup}
	return out
}

func binding(name, uid string) map[string]string {
	return map[string]string{webhookConfigurationBindingNameFieldKey: name, webhookConfigurationBindingUIDFieldKey: uid}
}

func (f fixture) noneToken(t *testing.T) string {
	t.Helper()
	claims := jwt.Claims{Issuer: testIssuer, Subject: testSubject, Audience: jwt.Audience{testAudience}, Expiry: jwt.NewNumericDate(f.now.Add(time.Hour)), NotBefore: jwt.NewNumericDate(f.now.Add(-time.Minute)), IssuedAt: jwt.NewNumericDate(f.now.Add(-time.Minute))}
	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	var claimsMap map[string]any
	if err := json.Unmarshal(claimsBytes, &claimsMap); err != nil {
		t.Fatal(err)
	}
	claimsMap[kubernetesPrivateClaimKey] = f.privateKubernetesClaims(tokenOptions{})
	headerBytes, err := json.Marshal(map[string]string{"alg": "none"})
	if err != nil {
		t.Fatal(err)
	}
	payloadBytes, err := json.Marshal(claimsMap)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding
	return enc.EncodeToString(headerBytes) + "." + enc.EncodeToString(payloadBytes) + "."
}
