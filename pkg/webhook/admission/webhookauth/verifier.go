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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Provisional KEP-6060 wire constants. Upstream API review has not settled the
// private claim keys, binding field names, feature gate name, or audience
// derivation format; reconcile this single block with final Kubernetes API.
const (
	kubernetesPrivateClaimKey               = "kubernetes.io"
	validatingWebhookConfigurationClaimKey  = "validatingWebhookConfiguration"
	mutatingWebhookConfigurationClaimKey    = "mutatingWebhookConfiguration"
	attestationClaimsFieldKey               = "attestationClaims"
	attestationsFieldKey                    = "attestations"
	AllowedAPIGroupClaim                    = "webhook-authentication.k8s.io/allowedAPIGroup"
	allowedAPIGroupWildcard                 = "*"
	webhookConfigurationBindingNameFieldKey = "name"
	webhookConfigurationBindingUIDFieldKey  = "uid"
	wellKnownOpenIDConfigurationPathSegment = ".well-known/openid-configuration"
)

const (
	defaultJWKSCacheTTL = 5 * time.Minute
	defaultClockSkew    = time.Minute
)

type WebhookType string

const (
	ValidatingWebhook WebhookType = "validating"
	MutatingWebhook   WebhookType = "mutating"
)

type WebhookBinding struct {
	Type WebhookType
	Name string
	UID  string
}

type Clock interface{ Now() time.Time }
type ClockFunc func() time.Time

func (f ClockFunc) Now() time.Time { return f() }

type KeySet interface {
	Keys(context.Context, string) ([]jose.JSONWebKey, error)
}
type RefreshingKeySet interface {
	KeySet
	Refresh(context.Context) error
}

type Options struct {
	IssuerURL         string
	Audience          string
	Binding           WebhookBinding
	KeySet            KeySet
	HTTPClient        *http.Client
	Clock             Clock
	AllowedAlgorithms []jose.SignatureAlgorithm
	ClockSkew         time.Duration
	JWKSCacheTTL      time.Duration
}

type Verifier interface {
	Verify(context.Context, string, admission.Request) (*Result, error)
}

type Result struct {
	Subject         string
	Audience        []string
	AllowedAPIGroup string
	Binding         WebhookBinding
	Expiration      time.Time
}

type verifier struct {
	issuerURL        string
	audience         string
	binding          WebhookBinding
	keySet           KeySet
	refreshingKeySet RefreshingKeySet
	clock            Clock
	allowedAlgs      []jose.SignatureAlgorithm
	clockSkew        time.Duration
}

func NewVerifier(opts Options) (Verifier, error) {
	issuer, audience := strings.TrimSpace(opts.IssuerURL), strings.TrimSpace(opts.Audience)
	if issuer == "" {
		return nil, fmt.Errorf("issuer URL is required")
	}
	if audience == "" || audience == allowedAPIGroupWildcard {
		return nil, fmt.Errorf("audience must be non-empty and non-wildcard")
	}
	if opts.Binding.Type != ValidatingWebhook && opts.Binding.Type != MutatingWebhook {
		return nil, fmt.Errorf("binding type must be validating or mutating")
	}
	if strings.TrimSpace(opts.Binding.Name) == "" {
		return nil, fmt.Errorf("binding name is required")
	}
	keySet := opts.KeySet
	if keySet == nil {
		remote, err := NewRemoteKeySet(RemoteKeySetOptions{IssuerURL: issuer, Client: opts.HTTPClient, CacheTTL: opts.JWKSCacheTTL})
		if err != nil {
			return nil, err
		}
		keySet = remote
	}
	refreshing, _ := keySet.(RefreshingKeySet)
	clock := opts.Clock
	if clock == nil {
		clock = ClockFunc(time.Now)
	}
	clockSkew := opts.ClockSkew
	if clockSkew == 0 {
		clockSkew = defaultClockSkew
	}
	if clockSkew < 0 || clockSkew > 5*time.Minute {
		return nil, fmt.Errorf("clock skew must be between 0 and 5 minutes")
	}
	algs := opts.AllowedAlgorithms
	if len(algs) == 0 {
		algs = []jose.SignatureAlgorithm{jose.RS256}
	}
	for _, alg := range algs {
		if alg == "" || alg == jose.SignatureAlgorithm("none") || strings.HasPrefix(string(alg), "HS") {
			return nil, fmt.Errorf("unsupported signing algorithm %q", alg)
		}
	}
	return &verifier{issuerURL: issuer, audience: audience, binding: opts.Binding, keySet: keySet, refreshingKeySet: refreshing, clock: clock, allowedAlgs: append([]jose.SignatureAlgorithm(nil), algs...), clockSkew: clockSkew}, nil
}

func (v *verifier) Verify(ctx context.Context, raw string, req admission.Request) (*Result, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, unauthenticated("missing token")
	}
	if strings.Count(raw, ".") != 2 {
		return nil, unauthenticated("malformed JWT")
	}
	parsed, err := jwt.ParseSigned(raw, v.allowedAlgs)
	if err != nil {
		return nil, unauthenticated("malformed or disallowed JWT")
	}
	if len(parsed.Headers) != 1 {
		return nil, unauthenticated("JWT must contain exactly one signature")
	}
	claims, private, err := v.verifySignatureAndDecodeClaims(ctx, parsed)
	if err != nil {
		return nil, err
	}
	if err := claims.ValidateWithLeeway(jwt.Expected{Issuer: v.issuerURL, AnyAudience: jwt.Audience{v.audience}, Time: v.clock.Now()}, v.clockSkew); err != nil {
		if errors.Is(err, jwt.ErrInvalidAudience) {
			return nil, unauthorized("JWT audience is not authorized")
		}
		return nil, unauthenticated("JWT standard claims are invalid")
	}
	if claims.Expiry == nil {
		return nil, unauthenticated("JWT expiration claim is required")
	}
	if claims.NotBefore == nil {
		return nil, unauthenticated("JWT not-before claim is required")
	}
	if claims.IssuedAt == nil {
		return nil, unauthenticated("JWT issued-at claim is required")
	}
	binding, err := extractAndVerifyBinding(private, v.binding)
	if err != nil {
		return nil, err
	}
	group, err := extractAndVerifyAllowedAPIGroup(private, req.Resource.Group)
	if err != nil {
		return nil, err
	}
	return &Result{Subject: claims.Subject, Audience: []string(claims.Audience), AllowedAPIGroup: group, Binding: binding, Expiration: claims.Expiry.Time()}, nil
}

func (v *verifier) verifySignatureAndDecodeClaims(ctx context.Context, parsed *jwt.JSONWebToken) (jwt.Claims, map[string]json.RawMessage, error) {
	kid := parsed.Headers[0].KeyID
	keys, err := v.keySet.Keys(ctx, kid)
	if err != nil {
		return jwt.Claims{}, nil, unauthenticated("unable to obtain verification keys")
	}
	claims, private, ok := tryVerifyWithKeys(parsed, keys, parsed.Headers[0].Algorithm)
	if ok {
		return claims, private, nil
	}
	if v.refreshingKeySet != nil {
		if err := v.refreshingKeySet.Refresh(ctx); err != nil {
			return jwt.Claims{}, nil, unauthenticated("unable to refresh verification keys")
		}
		keys, err = v.refreshingKeySet.Keys(ctx, kid)
		if err != nil {
			return jwt.Claims{}, nil, unauthenticated("unable to obtain refreshed verification keys")
		}
		claims, private, ok = tryVerifyWithKeys(parsed, keys, parsed.Headers[0].Algorithm)
		if ok {
			return claims, private, nil
		}
	}
	return jwt.Claims{}, nil, unauthenticated("JWT signature verification failed")
}

func tryVerifyWithKeys(parsed *jwt.JSONWebToken, keys []jose.JSONWebKey, alg string) (jwt.Claims, map[string]json.RawMessage, bool) {
	for _, key := range keys {
		if key.Use != "" && key.Use != "sig" {
			continue
		}
		if key.Algorithm != "" && key.Algorithm != alg {
			continue
		}
		if !key.Valid() {
			continue
		}
		var c jwt.Claims
		p := map[string]json.RawMessage{}
		if err := parsed.Claims(key.Key, &c, &p); err == nil {
			return c, p, true
		}
	}
	return jwt.Claims{}, nil, false
}

func extractAndVerifyBinding(claims map[string]json.RawMessage, expected WebhookBinding) (WebhookBinding, error) {
	k, err := nestedObject(claims, kubernetesPrivateClaimKey)
	if err != nil {
		return WebhookBinding{}, err
	}
	v, hv, err := bindingClaim(k, validatingWebhookConfigurationClaimKey)
	if err != nil {
		return WebhookBinding{}, err
	}
	m, hm, err := bindingClaim(k, mutatingWebhookConfigurationClaimKey)
	if err != nil {
		return WebhookBinding{}, err
	}
	if hv == hm {
		return WebhookBinding{}, unauthorized("JWT must contain exactly one webhook configuration binding")
	}
	actual := WebhookBinding{}
	if hv {
		actual = WebhookBinding{Type: ValidatingWebhook, Name: v.Name, UID: v.UID}
	} else {
		actual = WebhookBinding{Type: MutatingWebhook, Name: m.Name, UID: m.UID}
	}
	if actual.Type != expected.Type || actual.Name != expected.Name {
		return WebhookBinding{}, unauthorized("JWT webhook binding does not match expected webhook")
	}
	if expected.UID != "" && actual.UID != expected.UID {
		return WebhookBinding{}, unauthorized("JWT webhook binding UID does not match expected webhook")
	}
	return actual, nil
}

type webhookBindingClaim struct{ Name, UID string }

func bindingClaim(claims map[string]json.RawMessage, key string) (webhookBindingClaim, bool, error) {
	raw, ok := claims[key]
	if !ok {
		return webhookBindingClaim{}, false, nil
	}
	var claim map[string]json.RawMessage
	if err := json.Unmarshal(raw, &claim); err != nil {
		return webhookBindingClaim{}, false, unauthorized("JWT webhook binding claim is malformed")
	}
	name, err := stringField(claim, webhookConfigurationBindingNameFieldKey)
	if err != nil || name == "" {
		return webhookBindingClaim{}, false, unauthorized("JWT webhook binding name is required")
	}
	uid, err := optionalStringField(claim, webhookConfigurationBindingUIDFieldKey)
	if err != nil {
		return webhookBindingClaim{}, false, unauthorized("JWT webhook binding UID is malformed")
	}
	return webhookBindingClaim{Name: name, UID: uid}, true, nil
}

func extractAndVerifyAllowedAPIGroup(claims map[string]json.RawMessage, resourceGroup string) (string, error) {
	k, err := nestedObject(claims, kubernetesPrivateClaimKey)
	if err != nil {
		return "", err
	}
	attestations, err := attestationClaims(k)
	if err != nil {
		return "", err
	}
	vals, ok := attestations[AllowedAPIGroupClaim]
	if !ok || len(vals) != 1 {
		return "", unauthorized("JWT allowed API group claim must contain exactly one value")
	}
	if vals[0] != allowedAPIGroupWildcard && vals[0] != resourceGroup {
		return "", unauthorized("JWT allowed API group claim does not match request resource")
	}
	return vals[0], nil
}

func nestedObject(claims map[string]json.RawMessage, key string) (map[string]json.RawMessage, error) {
	raw, ok := claims[key]
	if !ok {
		return nil, unauthorized("JWT Kubernetes private claims are required")
	}
	out := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, unauthorized("JWT Kubernetes private claims are malformed")
	}
	return out, nil
}

func attestationClaims(k map[string]json.RawMessage) (map[string][]string, error) {
	raw, ok := k[attestationClaimsFieldKey]
	if !ok {
		raw, ok = k[attestationsFieldKey]
	}
	if !ok {
		return nil, unauthorized("JWT attestation claims are required")
	}
	out := map[string][]string{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, unauthorized("JWT attestation claims are malformed")
	}
	return out, nil
}

func stringField(claims map[string]json.RawMessage, key string) (string, error) {
	raw, ok := claims[key]
	if !ok {
		return "", errors.New("missing string field")
	}
	var out string
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	return out, nil
}

func optionalStringField(claims map[string]json.RawMessage, key string) (string, error) {
	raw, ok := claims[key]
	if !ok {
		return "", nil
	}
	var out string
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	return out, nil
}

type RemoteKeySetOptions struct {
	IssuerURL string
	Client    *http.Client
	CacheTTL  time.Duration
}

type RemoteKeySet struct {
	issuerURL string
	client    *http.Client
	cacheTTL  time.Duration
	mu        sync.RWMutex
	keys      jose.JSONWebKeySet
	expiresAt time.Time
}

func NewRemoteKeySet(opts RemoteKeySetOptions) (*RemoteKeySet, error) {
	issuer := strings.TrimRight(strings.TrimSpace(opts.IssuerURL), "/")
	if issuer == "" {
		return nil, fmt.Errorf("issuer URL is required")
	}
	if _, err := url.ParseRequestURI(issuer); err != nil {
		return nil, fmt.Errorf("issuer URL is invalid: %w", err)
	}
	client := opts.Client
	if client == nil {
		client = http.DefaultClient
	}
	ttl := opts.CacheTTL
	if ttl == 0 {
		ttl = defaultJWKSCacheTTL
	}
	if ttl < 0 {
		return nil, fmt.Errorf("JWKS cache TTL must not be negative")
	}
	return &RemoteKeySet{issuerURL: issuer, client: client, cacheTTL: ttl}, nil
}

func (s *RemoteKeySet) Keys(ctx context.Context, kid string) ([]jose.JSONWebKey, error) {
	if err := s.ensureFresh(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if kid != "" {
		return append([]jose.JSONWebKey(nil), s.keys.Key(kid)...), nil
	}
	return append([]jose.JSONWebKey(nil), s.keys.Keys...), nil
}

func (s *RemoteKeySet) Refresh(ctx context.Context) error {
	d, err := s.fetchDiscovery(ctx)
	if err != nil {
		return err
	}
	keys, err := s.fetchJWKS(ctx, d.JWKSURI)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys = keys
	s.expiresAt = time.Now().Add(s.cacheTTL)
	return nil
}

func (s *RemoteKeySet) ensureFresh(ctx context.Context) error {
	s.mu.RLock()
	fresh := len(s.keys.Keys) > 0 && time.Now().Before(s.expiresAt)
	s.mu.RUnlock()
	if fresh {
		return nil
	}
	return s.Refresh(ctx)
}

type discoveryDocument struct {
	Issuer  string `json:"issuer"`
	JWKSURI string `json:"jwks_uri"`
}

func (s *RemoteKeySet) fetchDiscovery(ctx context.Context) (discoveryDocument, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.issuerURL+"/"+wellKnownOpenIDConfigurationPathSegment, nil)
	if err != nil {
		return discoveryDocument{}, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return discoveryDocument{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return discoveryDocument{}, fmt.Errorf("OIDC discovery returned status %d", resp.StatusCode)
	}
	var doc discoveryDocument
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return discoveryDocument{}, err
	}
	if doc.Issuer != s.issuerURL {
		return discoveryDocument{}, fmt.Errorf("OIDC discovery issuer mismatch")
	}
	if doc.JWKSURI == "" {
		return discoveryDocument{}, fmt.Errorf("OIDC discovery jwks_uri is required")
	}
	return doc, nil
}

func (s *RemoteKeySet) fetchJWKS(ctx context.Context, uri string) (jose.JSONWebKeySet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return jose.JSONWebKeySet{}, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return jose.JSONWebKeySet{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return jose.JSONWebKeySet{}, fmt.Errorf("JWKS returned status %d", resp.StatusCode)
	}
	var keys jose.JSONWebKeySet
	if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil {
		return jose.JSONWebKeySet{}, err
	}
	if len(keys.Keys) == 0 {
		return jose.JSONWebKeySet{}, fmt.Errorf("JWKS contains no keys")
	}
	return keys, nil
}
