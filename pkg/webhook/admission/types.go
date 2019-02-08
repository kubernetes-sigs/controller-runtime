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
