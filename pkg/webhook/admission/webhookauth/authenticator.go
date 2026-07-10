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
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/webhook-auth/verify"
	"k8s.io/webhook-auth/verify/admissionhttp"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// authenticator adapts a k8s.io/webhook-auth *verify.Verifier onto the
// controller-runtime admission.Authenticator seam.
type authenticator struct{ v *verify.Verifier }

var _ admission.Authenticator = authenticator{}

// NewAuthenticator returns an admission.Authenticator that enforces KEP-6060
// API server authentication using the supplied k8s.io/webhook-auth verifier.
//
// The caller owns verifier construction, which keeps controller-runtime free of
// any JOSE/OIDC dependency: build v with
//
//	verify.NewVerifier(authenticator)
//
// supplying your own verify.TokenAuthenticator (the single piece a real
// deployment provides): it verifies the token's signature and standard iss/aud/exp
// claims and returns a *verify.VerifiedClaims for the policy layer to finish.
//
// Opt-in / default-off: assign the returned value to Webhook.Authenticator to
// turn verification on; leaving it nil preserves existing behavior exactly.
//
// Zero-config alternative (deliberately not wired here): the library also offers
// k8s.io/webhook-auth/verify/oidc.InCluster, which discovers the cluster issuer
// from the pod's projected service-account token and builds an
// OIDC-discovery/JWKS authenticator with no explicit configuration (option-free;
// for an explicit issuer/audience use oidc.NewRemoteVerifier). We do NOT call it
// from this package because it would pull the go-oidc dependency into
// controller-runtime's module graph. If a zero-config helper is desired, we could
// add a thin optional constructor (for example NewInClusterAuthenticator) in a
// separate, optional package so consumers opt into the extra dependency. Pending
// review with SIG-Auth.
func NewAuthenticator(v *verify.Verifier) admission.Authenticator {
	return authenticator{v: v}
}

// Authenticate verifies the API server's bearer token against the KEP-6060
// contract and fails closed on any error. It reuses the AdmissionRequest that
// controller-runtime already decoded, so the review is never decoded twice.
//
// The library's failure model is deliberately opaque: every rejection is a single
// generic error. We therefore always deny with a 401 and log only the
// non-sensitive verify.Reason string; callers must not branch on the reason.
func (a authenticator) Authenticate(ctx context.Context, r *http.Request, req admission.Request) admission.Response {
	token, ok := admissionhttp.BearerToken(r)
	if !ok {
		return admission.Unauthenticated("missing or malformed bearer token")
	}
	if a.v == nil {
		return admission.Unauthenticated("verifier is not configured")
	}
	// Reuse controller-runtime's single decode; never re-read r.Body.
	ar := &admissionv1.AdmissionReview{Request: &req.AdmissionRequest}
	if err := admissionhttp.VerifyAdmissionReview(ctx, a.v, ar, token); err != nil {
		logf.FromContext(ctx).Info("webhook-auth: denied unauthenticated request", "reason", verify.Reason(err))
		return admission.Unauthenticated("unauthenticated")
	}
	return admission.Allowed("")
}
