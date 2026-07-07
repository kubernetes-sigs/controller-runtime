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

// Package webhookauth is an opt-in controller-runtime adapter for KEP-6060
// admission webhook authentication.
//
// It consumes the k8s.io/webhook-auth library rather than re-implementing token
// verification: NewAuthenticator wraps a *verify.Verifier as an
// admission.Authenticator that controller-runtime invokes after it has decoded
// the AdmissionReview and before the user handler runs. The API server's bearer
// token is verified entirely offline against the library's contract (signature,
// issuer, audience, exp/nbf/iat, webhook binding, and allowed API group) with no
// TokenReview round-trip. Any verification failure fails closed.
//
// Opt-in / default-off: set the returned value on Webhook.Authenticator to turn
// verification on. Leaving Authenticator nil preserves existing behavior exactly,
// so a webhook that does not adopt verification is unaffected.
//
// This package intentionally adds no crypto/OIDC dependency of its own. The
// caller constructs the verify.Verifier (supplying a verify.KeySet) and thus
// decides whether to pull in go-oidc. See NewAuthenticator for the zero-config
// in-cluster verifier the library offers and why it is left to the consumer.
//
// KEP-6060's wire format is provisional; the claim contract lives in
// k8s.io/webhook-auth and must be reconciled after upstream API review.
package webhookauth
