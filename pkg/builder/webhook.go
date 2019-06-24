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

package builder

import (
	"net/http"
	"net/url"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"
)

// WebhookBuilder builds a Controller.
type WebhookBuilder struct {
	apiType runtime.Object
	mgr     manager.Manager
	config  *rest.Config
}

func WebhookManagedBy(m manager.Manager) *WebhookBuilder {
	return &WebhookBuilder{mgr: m}
}

// For defines the type of Object being *reconciled*, and configures the ControllerManagedBy to respond to create / delete /
// update events by *reconciling the object*.
// This is the equivalent of calling
// Watches(&source.Kind{Type: apiType}, &handler.EnqueueRequestForObject{})
// If the passed in object has implemented the admission.Defaulter interface, a MutatingWebhook will be wired for this type.
// If the passed in object has implemented the admission.Validator interface, a ValidatingWebhook will be wired for this type.
// TODO: figure out if we support only one object or multiple objects
func (blder *WebhookBuilder) For(apiType runtime.Object) *WebhookBuilder {
	blder.apiType = apiType
	return blder
}

// Complete builds the Application ControllerManagedBy.
func (blder *WebhookBuilder) Complete() error {
	// Set the Config
	if err := blder.doConfig(); err != nil {
		return err
	}

	// Set the Manager
	if err := blder.doManager(); err != nil {
		return err
	}

	// Set the Webook if needed
	if err := blder.doWebhook(); err != nil {
		return err
	}

	return nil
}

func (blder *WebhookBuilder) doConfig() error {
	if blder.config != nil {
		return nil
	}
	if blder.mgr != nil {
		blder.config = blder.mgr.GetConfig()
		return nil
	}
	var err error
	blder.config, err = getConfig()
	return err
}

func (blder *WebhookBuilder) doManager() error {
	if blder.mgr != nil {
		return nil
	}
	var err error
	blder.mgr, err = newManager(blder.config, manager.Options{})
	return err
}

func (blder *WebhookBuilder) doWebhook() error {
	// Create a webhook for each type
	gvk, err := apiutil.GVKForObject(blder.apiType, blder.mgr.GetScheme())
	if err != nil {
		return err
	}

	// TODO: When the conversion webhook lands, we need to handle all registered versions of a given group-kind.
	// A potential workflow for defaulting webhook
	// 1) a bespoke (non-hub) version comes in
	// 2) convert it to the hub version
	// 3) do defaulting
	// 4) convert it back to the same bespoke version
	// 5) calculate the JSON patch
	//
	// A potential workflow for validating webhook
	// 1) a bespoke (non-hub) version comes in
	// 2) convert it to the hub version
	// 3) do validation
	if defaulter, isDefaulter := blder.apiType.(admission.Defaulter); isDefaulter {
		mwh := admission.DefaultingWebhookFor(defaulter)
		if mwh != nil {
			path := generateMutatePath(gvk)

			// Checking if the path is already registered.
			// If so, just skip it.
			if !blder.isAlreadyHandled(path) {
				log.Info("Registering a mutating webhook",
					"GVK", gvk,
					"path", path)
				blder.mgr.GetWebhookServer().Register(path, mwh)
			}
		}
	}

	if validator, isValidator := blder.apiType.(admission.Validator); isValidator {
		vwh := admission.ValidatingWebhookFor(validator)
		if vwh != nil {
			path := generateValidatePath(gvk)

			// Checking if the path is already registered.
			// If so, just skip it.
			if !blder.isAlreadyHandled(path) {
				log.Info("Registering a validating webhook",
					"GVK", gvk,
					"path", path)
				blder.mgr.GetWebhookServer().Register(path, vwh)
			}
		}
	}

	err = conversion.CheckConvertibility(blder.mgr.GetScheme(), blder.apiType)
	if err != nil {
		log.Error(err, "conversion check failed", "GVK", gvk)
	}
	return nil
}

func (blder *WebhookBuilder) isAlreadyHandled(path string) bool {
	h, p := blder.mgr.GetWebhookServer().WebhookMux.Handler(&http.Request{URL: &url.URL{Path: path}})
	if p == path && h != nil {
		return true
	}
	return false
}

func generateMutatePath(gvk schema.GroupVersionKind) string {
	return "/mutate-" + strings.Replace(gvk.Group, ".", "-", -1) + "-" +
		gvk.Version + "-" + strings.ToLower(gvk.Kind)
}

func generateValidatePath(gvk schema.GroupVersionKind) string {
	return "/validate-" + strings.Replace(gvk.Group, ".", "-", -1) + "-" +
		gvk.Version + "-" + strings.ToLower(gvk.Kind)
}
