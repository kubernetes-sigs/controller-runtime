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
	"errors"
	"net/http"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const authorizationSchemeBearer = "bearer"

type authenticator struct {
	verifier Verifier
}

var _ admission.Authenticator = authenticator{}

func NewAuthenticator(verifier Verifier) admission.Authenticator {
	return authenticator{verifier: verifier}
}

func (a authenticator) Authenticate(ctx context.Context, httpReq *http.Request, req admission.Request) admission.Response {
	token, ok := bearerToken(httpReq)
	if !ok {
		return admission.Unauthenticated("missing or malformed bearer token")
	}
	if a.verifier == nil {
		return admission.Unauthenticated("verifier is not configured")
	}
	if _, err := a.verifier.Verify(ctx, token, req); err != nil {
		return responseForVerifierError(err)
	}
	return admission.Allowed("")
}

func bearerToken(req *http.Request) (string, bool) {
	if req == nil {
		return "", false
	}
	values := req.Header.Values("Authorization")
	if len(values) != 1 {
		return "", false
	}
	fields := strings.Fields(values[0])
	if len(fields) != 2 || !strings.EqualFold(fields[0], authorizationSchemeBearer) || fields[1] == "" {
		return "", false
	}
	return fields[1], true
}

func responseForVerifierError(err error) admission.Response {
	message := "authentication failed"
	var verifierErr *Error
	if errors.As(err, &verifierErr) && verifierErr.Message != "" {
		message = verifierErr.Message
	}
	if IsUnauthorized(err) {
		return admission.Unauthorized(message)
	}
	return admission.Unauthenticated(message)
}
