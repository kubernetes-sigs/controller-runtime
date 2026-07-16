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
	"context"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// exampleHandler is a no-op admission handler used only to complete the wiring
// in these compile-time documentation examples.
var exampleHandler = admission.HandlerFunc(func(context.Context, admission.Request) admission.Response {
	return admission.Allowed("")
})

// ExampleWebhook_WithInClusterAuthenticator shows the zero-config, in-cluster
// wiring: the issuer, audience, and trusted CA are discovered from the pod's
// projected service-account token. The fluent method turns KEP-6060 API-server
// authentication on and fails fast at startup on any discovery error.
//
// This example is compiled but not executed (it has no "// Output:" line): the
// method performs OIDC discovery and background JWKS refresh against the live
// in-cluster issuer, which is unavailable in a unit-test environment.
func ExampleWebhook_WithInClusterAuthenticator() {
	// Pass a process-lifetime context (e.g. the manager's) — it governs OIDC
	// discovery and the long-lived background JWKS refresh.
	ctx := context.Background()

	wh, err := (&admission.Webhook{
		Handler: exampleHandler,
	}).WithInClusterAuthenticator(ctx)
	if err != nil {
		// In a real setup, fail startup; here we simply return.
		return
	}
	_ = wh
}

// ExampleWebhook_WithRemoteAuthenticator shows the out-of-cluster wiring with an
// explicit issuer and audience. Pass an *http.Client whose transport trusts the
// issuer's serving CA (nil uses the default transport).
//
// This example is compiled but not executed (it has no "// Output:" line): the
// method performs OIDC discovery against the issuer, which is unavailable in a
// unit-test environment.
func ExampleWebhook_WithRemoteAuthenticator() {
	// Pass a process-lifetime context — it governs OIDC discovery and the
	// long-lived background JWKS refresh.
	ctx := context.Background()

	const (
		issuer   = "https://issuer.example.com"
		audience = "webhook.example.com"
	)

	// A default http.Client is used here for illustration; in a real
	// deployment pass a client with a transport that trusts the issuer's
	// serving CA.
	httpClient := &http.Client{}

	wh, err := (&admission.Webhook{
		Handler: exampleHandler,
	}).WithRemoteAuthenticator(ctx, issuer, audience, httpClient)
	if err != nil {
		// In a real setup, fail startup; here we simply return.
		return
	}
	_ = wh
}

// ExampleWebhook_WithAuthenticator shows bring-your-own wiring: wrap a
// pre-built k8s.io/webhookauth *verify.Verifier with admission.NewAuthenticator
// and set it via the fluent method. Prefer WithInClusterAuthenticator or
// WithRemoteAuthenticator unless you need full control over verifier
// construction.
func ExampleWebhook_WithAuthenticator() {
	// In a real setup, build a *verify.Verifier (e.g. via k8s.io/webhookauth's
	// oidc helpers) and wrap it with admission.NewAuthenticator. A nil
	// authenticator preserves the default admission webhook behavior.
	var custom admission.Authenticator

	wh := (&admission.Webhook{
		Handler: exampleHandler,
	}).WithAuthenticator(custom)
	_ = wh
}
