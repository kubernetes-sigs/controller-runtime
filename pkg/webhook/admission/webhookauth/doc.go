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

// Package webhookauth verifies KEP-6060 admission webhook authentication JWTs.
//
// Verification is offline: JWS signatures are checked against cached
// OIDC-discovered JWKS, then issuer, audience, exp, nbf, iat, webhook binding,
// and allowed API group claims are enforced. TokenReview is never used by
// default. Unknown kid or cache expiry triggers JWKS refresh; refresh failure
// fails closed.
//
// The default signing algorithm allow-list is RS256. Callers may configure
// other asymmetric algorithms, but "none" and symmetric HS* algorithms are
// rejected to follow RFC 8725 JWT BCP guidance against unsecured JWTs and
// algorithm confusion. RFC 9700 OAuth2 Security BCP likewise motivates strict
// audience-bound bearer-token validation.
//
// KEP-6060's wire format is provisional. All private claim names used here are
// isolated in verifier.go and must be reconciled after upstream API review.
package webhookauth
