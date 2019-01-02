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

package server

import (
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/builder"
)

var (
	log = logf.Log.WithName("webhook_server")
)

// ServerBuilder builds webhook servers for Validating and Defaulting Resources
type ServerBuilder struct {
	// Name is the name of the webhook server.  Defaults to "unknown".
	Name string

	// Options are the ServerOptions used to start and register the server.  Defaults to
	// generating a CA + Certificates stored in a Secret.
	Options *webhook.ServerOptions

	objects []runtime.Object
	mgr     manager.Manager
	fn      func(*webhook.ServerOptions)
}

type WebhookDefinition struct {
	Name           string
	WebhookBuilder *builder.WebhookBuilder
	Handlers       []admission.Handler
}

// Add adds a Resource, registering webhooks for Validation and Defaulting
func (s *ServerBuilder) Add(r runtime.Object) *ServerBuilder {

	//v, ok := r.(resource.Validator)
	//if ok {
	//
	//}
	//
	//d, ok := r.(resource.Defaulter)
	//if ok {
	//
	//}

	return s
}

// ApplyOptions registers a function to be called on the default server options before they are used
func (s *ServerBuilder) ApplyOptions(fn func(*webhook.ServerOptions)) *ServerBuilder {
	s.fn = fn
	return s
}

// Complete builds the webhook server and registers it with the Manager
func (s *ServerBuilder) Complete() error {
	s.initName()
	s.initOptions()

	svr, err := webhook.NewServer(s.Name+"-admission-server", s.mgr, *s.Options)
	if err != nil {
		return err
	}

	webhooks, err := s.initWebhooks(s.initHandlers())
	if err != nil {
		return err
	}

	return svr.Register(webhooks...)
}

func (s *ServerBuilder) initName() {
	if s.Name == "" {
		s.Name = "unknown"
	}
}

func (s *ServerBuilder) initOptions() {
	// Populate the default set of server options if unset
	if s.Options == nil {
		ns := os.Getenv("POD_NAMESPACE")
		if len(ns) == 0 {
			ns = "default"
		}
		secretName := os.Getenv("SECRET_NAME")
		if len(secretName) == 0 {
			secretName = "webhook-server-secret"
		}
		s.Options = &webhook.ServerOptions{
			Port:    9876,
			CertDir: "/tmp/cert",
			BootstrapOptions: &webhook.BootstrapOptions{
				Secret: &types.NamespacedName{
					Namespace: ns,
					Name:      secretName,
				},

				Service: &webhook.Service{
					Namespace: ns,
					Name:      "webhook-server-service",
					// Selectors should select the pods that runs this webhook server.
					Selectors: map[string]string{
						"control-plane": "controller-manager",
					},
				},
			},
		}
	}

	// Allow users to override default options
	if s.fn != nil {
		s.fn(s.Options)
	}
}

func (s *ServerBuilder) initHandlers() []WebhookDefinition {
	whr := []WebhookDefinition{}

	return whr
}

func (s *ServerBuilder) initWebhooks(whr []WebhookDefinition) ([]webhook.Webhook, error) {

	var webhooks []webhook.Webhook
	for _, builder := range whr {
		wh, err := builder.WebhookBuilder.
			Handlers(builder.Handlers...).
			WithManager(s.mgr).
			Build()
		if err != nil {
			return webhooks, err
		}
		webhooks = append(webhooks, wh)
	}

	return webhooks, nil
}
