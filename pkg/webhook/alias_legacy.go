/*
Copyright 2019 The Kubernetes Authors.

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

package webhook

import (
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// AdmissionRequestLegacy defines the input for an admission handler.
// It contains information to identify the object in
// question (group, version, kind, resource, subresource,
// name, namespace), as well as the operation in question
// (e.g. Get, Create, etc), and the object itself.
type AdmissionRequestLegacy = admission.RequestLegacy

// AdmissionResponseLegacy is the output of an admission handler.
// It contains a response indicating if a given
// operation is allowed, as well as a set of patches
// to mutate the object in the case of a mutating admission handler.
type AdmissionResponseLegacy = admission.ResponseLegacy

// AdmissionLegacy is webhook suitable for registration with the server
// an admission webhook that validates API operations and potentially
// mutates their contents.
type AdmissionLegacy = admission.WebhookLegacy

// AdmissionHandlerLegacy knows how to process admission requests, validating them,
// and potentially mutating the objects they contain.
type AdmissionHandlerLegacy = admission.HandlerLegacy

var (
	// AllowedLegacy indicates that the admission request should be allowed for the given reason.
	AllowedLegacy = admission.AllowedLegacy

	// DeniedLegacy indicates that the admission request should be denied for the given reason.
	DeniedLegacy = admission.DeniedLegacy

	// PatchedLegacy indicates that the admission request should be allowed for the given reason,
	// and that the contained object should be mutated using the given patches.
	PatchedLegacy = admission.PatchedLegacy

	// ErroredLegacy indicates that an error occurred in the admission request.
	ErroredLegacy = admission.ErroredLegacy
)
