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
	"context"
	"net/http"

	"k8s.io/webhookauth/verify"
	"k8s.io/webhookauth/verify/admissionhttp"
	"k8s.io/webhookauth/verify/oidc"
	"k8s.io/webhookauth/verify/oidc/incluster"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// healthChecker is implemented by Authenticators whose readiness can be probed
// (e.g. the verifier-backed authenticator, which is not ready until its audience is bound).
type healthChecker interface{ HealthCheck() error }

// verifierAuthenticator adapts a k8s.io/webhookauth *verify.Verifier onto the
// admission.Authenticator seam.
type verifierAuthenticator struct{ v *verify.Verifier }

var (
	_ Authenticator = verifierAuthenticator{}
	_ healthChecker = verifierAuthenticator{}
)

// NewAuthenticator wraps a k8s.io/webhookauth *verify.Verifier as an
// Authenticator that enforces KEP-6060 API server authentication.
//
// This is the advanced / bring-your-own verifier entry point. Most users should
// prefer the fluent Webhook.WithInClusterAuthenticator (zero-config, in-cluster)
// or Webhook.WithRemoteAuthenticator (explicit issuer/audience) methods, which
// build the verifier for you.
//
// The verifier verifies the API server's bearer token entirely offline against
// the library's contract (signature, issuer, audience, exp/nbf/iat, webhook
// binding, and allowed API group). Any verification failure fails closed.
func NewAuthenticator(v *verify.Verifier) Authenticator {
	return verifierAuthenticator{v: v}
}

// Authenticate verifies the API server's bearer token against the KEP-6060
// contract and fails closed on any error. It reuses the AdmissionRequest that
// controller-runtime already decoded, so the review is never decoded twice.
//
// The library's failure model is deliberately opaque: every rejection is a single
// generic error (verify.ErrVerificationFailed). We therefore always deny with a
// 401 and log only a static message; callers must not branch on the reason and no
// claim material is ever surfaced.
func (a verifierAuthenticator) Authenticate(ctx context.Context, r *http.Request, req Request) Response {
	token, ok := admissionhttp.BearerToken(r)
	if !ok {
		return Unauthenticated("missing or malformed bearer token")
	}
	if a.v == nil {
		return Unauthenticated("verifier is not configured")
	}
	// Reuse controller-runtime's single decode; never re-read r.Body.
	if err := admissionhttp.VerifyAdmissionRequest(ctx, a.v, &req.AdmissionRequest, token); err != nil {
		logf.FromContext(ctx).Info("webhook-auth: denied unauthenticated request")
		return Unauthenticated("unauthenticated")
	}
	return Allowed("")
}

// HealthCheck reports whether the backing verifier is ready to verify API server
// tokens, delegating to the library verifier's own HealthCheck. A nil verifier
// gates nothing and always reports ready.
func (a verifierAuthenticator) HealthCheck() error {
	if a.v == nil {
		return nil // no verifier configured → nothing to gate readiness on
	}
	return a.v.HealthCheck()
}

// HealthCheck reports whether the webhook's KEP-6060 authenticator (if any) is
// ready to verify API server tokens. It has the signature of healthz.Checker, so
// it can be registered directly:
//
//	mgr.AddReadyzCheck("webhook-auth", wh.HealthCheck)
//
// This is a readiness check: register it with AddReadyzCheck, not AddHealthzCheck.
// An authenticator that cannot yet bind its audience should stay not-ready (so the
// pod is not sent traffic) rather than fail liveness, which would crash-loop the pod.
//
// When no verifier-backed authenticator is configured it always reports ready, so
// it is safe to register unconditionally. When an in-cluster/remote authenticator
// is configured, it reports not-ready until the verifier has bound its audience —
// so a pod that can never derive its audience fails readiness and is restarted,
// instead of silently denying all traffic.
func (wh *Webhook) HealthCheck(_ *http.Request) error {
	if hc, ok := wh.authenticator.(healthChecker); ok {
		return hc.HealthCheck()
	}
	return nil
}

// WithAuthenticator sets a custom Authenticator (bring-your-own / testing) and
// returns the Webhook for fluent chaining. A nil Authenticator preserves the
// default admission webhook behavior.
func (wh *Webhook) WithAuthenticator(a Authenticator) *Webhook {
	wh.authenticator = a
	return wh
}

// WithInClusterAuthenticator configures zero-config in-cluster KEP-6060
// authentication: the issuer, audience, and trusted CA are read from the pod's
// own projected service-account token (see k8s.io/webhookauth/verify/oidc/incluster.InCluster).
//
// It performs OIDC discovery now (a network round-trip) and returns an error so
// setup fails fast at startup. Pass a process-lifetime ctx (e.g. the manager's) —
// it governs OIDC discovery and the long-lived background JWKS refresh.
//
// On success it returns the Webhook for fluent chaining.
func (wh *Webhook) WithInClusterAuthenticator(ctx context.Context) (*Webhook, error) {
	v, err := incluster.InCluster(ctx)
	if err != nil {
		return nil, err
	}
	wh.authenticator = NewAuthenticator(v)
	return wh, nil
}

// WithRemoteAuthenticator configures explicit issuer/audience KEP-6060
// authentication via OIDC discovery (see k8s.io/webhookauth/verify/oidc.NewRemoteVerifier).
//
// httpClient (may be nil) is the transport that trusts the issuer's serving CA.
// It performs OIDC discovery now (a network round-trip) and returns an error so
// setup fails fast at startup. Pass a process-lifetime ctx — it governs OIDC
// discovery and the long-lived background JWKS refresh.
//
// On success it returns the Webhook for fluent chaining.
func (wh *Webhook) WithRemoteAuthenticator(ctx context.Context, issuer, audience string, httpClient *http.Client) (*Webhook, error) {
	var opts []oidc.Option
	if httpClient != nil {
		opts = append(opts, oidc.WithHTTPClient(httpClient))
	}
	v, err := oidc.NewRemoteVerifier(ctx, issuer, audience, opts...)
	if err != nil {
		return nil, err
	}
	wh.authenticator = NewAuthenticator(v)
	return wh, nil
}
