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

package builder

import (
	"errors"
	"fmt"

	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/types"
)

// WebhookBuilder builds a webhook based on the provided options.
type WebhookBuilder struct {
	// Name specifies the Name of the webhook. It must be unique in the http
	// server that serves all the webhooks.
	name string

	// Path is the URL Path to register this webhook. e.g. "/feature-foo-mutating-pods".
	path string

	// Type specifies the type of the webhook
	t *types.WebhookType
	// only one of operations and Rules can be set.
	operations []admissionregistrationv1beta1.OperationType

	// resources that this webhook cares.
	// Only one of apiType and Rules can be set.
	apiType runtime.Object
	rules   []admissionregistrationv1beta1.RuleWithOperations

	// This field maps to the FailurePolicy in the admissionregistrationv1beta1.Webhook
	failurePolicy *admissionregistrationv1beta1.FailurePolicyType

	// This field maps to the NamespaceSelector in the admissionregistrationv1beta1.Webhook
	namespaceSelector *metav1.LabelSelector

	// manager is the manager for the webhook.
	manager manager.Manager
}

// NewWebhookBuilder creates an empty WebhookBuilder.
func NewWebhookBuilder() *WebhookBuilder {
	return &WebhookBuilder{}
}

// Name sets the Name of the webhook.
// This is optional
func (b *WebhookBuilder) Name(name string) *WebhookBuilder {
	b.name = name
	return b
}

// Type sets the type of the admission webhook
// This is required
func (b *WebhookBuilder) Type(t types.WebhookType) *WebhookBuilder {
	b.t = &t
	return b
}

// Path sets the Path for the webhook.
// This is optional
func (b *WebhookBuilder) Path(path string) *WebhookBuilder {
	b.path = path
	return b
}

// Operations sets the operations that this webhook cares.
// It will be overridden by Rules if Rules are not empty.
// This is optional
func (b *WebhookBuilder) Operations(ops ...admissionregistrationv1beta1.OperationType) *WebhookBuilder {
	b.operations = ops
	return b
}

// ForType sets the type of resources that the webhook will operate.
// This cannot be use with Rules.
func (b *WebhookBuilder) ForType(obj runtime.Object) *WebhookBuilder {
	b.apiType = obj
	return b
}

// Rules sets the RuleWithOperations for the webhook.
// It overrides ForType and Operations.
// This is optional and for advanced user
func (b *WebhookBuilder) Rules(rules ...admissionregistrationv1beta1.RuleWithOperations) *WebhookBuilder {
	b.rules = rules
	return b
}

// FailurePolicy sets the FailurePolicy of the webhook.
// If not set, it will be defaulted by the server.
// This is optional
func (b *WebhookBuilder) FailurePolicy(policy admissionregistrationv1beta1.FailurePolicyType) *WebhookBuilder {
	b.failurePolicy = &policy
	return b
}

// NamespaceSelector sets the NamespaceSelector for the webhook.
// This is optional
func (b *WebhookBuilder) NamespaceSelector(namespaceSelector *metav1.LabelSelector) *WebhookBuilder {
	b.namespaceSelector = namespaceSelector
	return b
}

// WithManager set the manager for the webhook for provisioning client etc.
func (b *WebhookBuilder) WithManager(mgr manager.Manager) *WebhookBuilder {
	b.manager = mgr
	return b
}

func (b *WebhookBuilder) validate() error {
	if b.t == nil {
		return errors.New("webhook type cannot be nil")
	}
	if b.rules == nil && b.apiType == nil {
		return fmt.Errorf("ForType should be set")
	}
	if b.rules != nil && b.apiType != nil {
		return fmt.Errorf("at most one of ForType and Rules can be set")
	}
	return nil
}

// Build creates the Webhook based on the options provided.
func (b *WebhookBuilder) Build(handlers ...admission.Handler) (*admission.Webhook, error) {
	err := b.validate()
	if err != nil {
		return nil, err
	}

	w := &admission.Webhook{
		Name:              b.name,
		Type:              *b.t,
		FailurePolicy:     b.failurePolicy,
		NamespaceSelector: b.namespaceSelector,
		Handlers:          handlers,
	}

	if len(b.path) == 0 {
		if *b.t == types.WebhookTypeMutating {
			b.path = "/mutatingwebhook"
		} else if *b.t == types.WebhookTypeValidating {
			b.path = "/validatingwebhook"
		}
	}
	w.Path = b.path

	if b.rules != nil {
		w.Rules = b.rules
		return w, nil
	}

	if b.manager == nil {
		return nil, errors.New("manager should be set using WithManager")
	}
	gvk, err := apiutil.GVKForObject(b.apiType, b.manager.GetScheme())
	if err != nil {
		return nil, err
	}
	mapper := b.manager.GetRESTMapper()
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, err
	}

	if b.operations == nil {
		b.operations = []admissionregistrationv1beta1.OperationType{
			admissionregistrationv1beta1.Create,
			admissionregistrationv1beta1.Update,
		}
	}
	w.Rules = []admissionregistrationv1beta1.RuleWithOperations{
		{
			Operations: b.operations,
			Rule: admissionregistrationv1beta1.Rule{
				APIGroups:   []string{gvk.Group},
				APIVersions: []string{gvk.Version},
				Resources:   []string{mapping.Resource},
			},
		},
	}
	return w, nil
}
