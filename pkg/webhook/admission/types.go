/*
Copyright 2018 The Kubernetes Authors.

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
	"github.com/appscode/jsonpatch"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Request defines the input for an admission handler.
// It contains information to identify the object in
// question (group, version, kind, resource, subresource,
// name, namespace), as well as the operation in question
// (e.g. Get, Create, etc), and the object itself.
type Request struct {
	admissionv1beta1.AdmissionRequest
}

// Response is the output of an admission handler.
// It contains a response indicating if a given
// operation is allowed, as well as a set of patches
// to mutate the object in the case of a mutating admission handler.
type Response struct {
	// Patches are the JSON patches for mutating webhooks.
	// Using this instead of setting Response.Patch to minimize
	// overhead of serialization and deserialization.
	Patches []jsonpatch.JsonPatchOperation
	// AdmissionResponse is the raw admission response.
	// The Patch field in it will be overwritten by the listed patches.
	admissionv1beta1.AdmissionResponse
}

// Decoder is used to decode AdmissionRequest.
type Decoder interface {
	// Decode decodes the raw byte object from the AdmissionRequest to the passed-in runtime.Object.
	Decode(Request, runtime.Object) error
}

// WebhookType defines the type of an admission webhook
// (validating or mutating).
type WebhookType int

const (
	_ WebhookType = iota
	// MutatingWebhook represents a webhook that can mutate the object sent to it.
	MutatingWebhook
	// ValidatingWebhook represents a webhook that can only gate whether or not objects
	// sent to it are accepted.
	ValidatingWebhook
)
