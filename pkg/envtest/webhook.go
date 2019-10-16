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

package envtest

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
	"fmt"

	admissionreg "k8s.io/api/admissionregistration/v1beta1"
	kclient "k8s.io/client-go/kubernetes"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WebhookInstallOptions are the options for installing mutating or validating webhooks
type WebhookInstallOptions struct {
	// Paths is a list of paths to the directories containing the mutating or validating webhooks
	Paths []string

	// MutatingWebhooks is a list of MutatingWebhookConfigurations to install
	MutatingWebhooks []*admissionreg.MutatingWebhookConfiguration

	// ValidatingWebhooks is a list of ValidatingWebhookConfigurations to install
	ValidatingWebhooks []*admissionreg.ValidatingWebhookConfiguration

	// ErrorIfPathMissing will cause an error if a Path does not exist
	ErrorIfPathMissing bool

	// MaxTime is the max time to wait
	MaxTime time.Duration

	// PollInterval is the interval to check
	PollInterval time.Duration

	// CAData is the CA data to place into the CA field of the client config for each hook.
	CAData []byte

	// BaseHost is the host/port to substitute for each webhook's client config. 
	BaseHost string
}

// ApplyCustomServing applies custom CA data and host settings to each webhook.
func (o *WebhookInstallOptions) ApplyCustomServing() {
	for _, hookSet := range o.MutatingWebhooks {
		for i := range hookSet.Webhooks {
			cfg := &hookSet.Webhooks[i].ClientConfig
			cfg.CABundle = o.CAData
			var path string
			if cfg.Service != nil && cfg.Service.Path != nil {
				path = *cfg.Service.Path
			}
			cfg.Service = nil
			url := fmt.Sprintf("https://%s/%s", o.BaseHost, path)
			cfg.URL = &url
		}
	}
	for _, hookSet := range o.ValidatingWebhooks {
		for i := range hookSet.Webhooks {
			cfg := &hookSet.Webhooks[i].ClientConfig
			cfg.CABundle = o.CAData
			var path string
			if cfg.Service != nil && cfg.Service.Path != nil {
				path = *cfg.Service.Path
			}
			cfg.Service = nil
			url := fmt.Sprintf("https://%s/%s", o.BaseHost, path)
			cfg.URL = &url
		}
	}
}

// MightHaveHooks returns true if there's a chance that some hooks will be created.
func (o WebhookInstallOptions) MightHaveHooks() bool {
	return len(o.Paths) > 0 || len(o.MutatingWebhooks) > 0 || len(o.ValidatingWebhooks) > 0
}

// InstallWebhooks installs a collection of webhooks into a cluster by reading the crd yaml files from a directory
func InstallWebhooks(config *rest.Config, options WebhookInstallOptions) ([]*admissionreg.MutatingWebhookConfiguration, []*admissionreg.ValidatingWebhookConfiguration, error) {
	defaultWebhookOptions(&options)

	// Read the Webhook yamls into options.Webhooks
	if err := readWebhookFiles(&options); err != nil {
		return nil, nil, err
	}

	options.ApplyCustomServing()

	// Create the Webhooks in the apiserver
	if err := CreateWebhooks(config, options.MutatingWebhooks, options.ValidatingWebhooks); err != nil {
		return options.MutatingWebhooks, options.ValidatingWebhooks, err
	}

	// TODO(directxman12): do we need to wait for webhooks?

	return options.MutatingWebhooks, options.ValidatingWebhooks, nil
}

// readWebhookFiles reads the directories of Webhooks in options.Paths and adds the Webhook structs to options.Webhooks
func readWebhookFiles(options *WebhookInstallOptions) error {
	if len(options.Paths) > 0 {
		for _, path := range options.Paths {
			if _, err := os.Stat(path); !options.ErrorIfPathMissing && os.IsNotExist(err) {
				continue
			}
			mutHooks, valHooks, err := readWebhooks(path)
			if err != nil {
				return err
			}
			options.MutatingWebhooks = append(options.MutatingWebhooks, mutHooks...)
			options.ValidatingWebhooks = append(options.ValidatingWebhooks, valHooks...)
		}
	}
	return nil
}

// defaultWebhookOptions sets the default values for Webhooks
func defaultWebhookOptions(o *WebhookInstallOptions) {
	if o.MaxTime == 0 {
		o.MaxTime = defaultMaxWait
	}
	if o.PollInterval == 0 {
		o.PollInterval = defaultPollInterval
	}
}
// CreateWebhooks creates the Webhooks
func CreateWebhooks(config *rest.Config, mutHooks []*admissionreg.MutatingWebhookConfiguration, valHooks []*admissionreg.ValidatingWebhookConfiguration) error {
	cs, err := kclient.NewForConfig(config)
	if err != nil {
		return err
	}

	// Create each webhook
	for _, hook := range mutHooks {
		log.V(1).Info("installing mutating webhook", "webhook", hook.Name)
		if _, err := cs.AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Create(hook); err != nil {
			return err
		}
	}
	for _, hook := range valHooks {
		log.V(1).Info("installing validating webhook", "webhook", hook.Name)
		if _, err := cs.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Create(hook); err != nil {
			return err
		}
	}
	return nil
}

// readWebhooks reads the Webhooks from files and Unmarshals them into structs
func readWebhooks(path string) ([]*admissionreg.MutatingWebhookConfiguration, []*admissionreg.ValidatingWebhookConfiguration, error) {
	// Get the webhook files
	var files []os.FileInfo
	var err error
	log.V(1).Info("reading Webhooks from path", "path", path)
	if files, err = ioutil.ReadDir(path); err != nil {
		return nil, nil, err
	}

	// White list the file extensions that may contain Webhooks
	crdExts := sets.NewString(".json", ".yaml", ".yml")

	var mutHooks []*admissionreg.MutatingWebhookConfiguration
	var valHooks []*admissionreg.ValidatingWebhookConfiguration
	for _, file := range files {
		// Only parse whitelisted file types
		if !crdExts.Has(filepath.Ext(file.Name())) {
			continue
		}

		// Unmarshal Webhooks from file into structs
		docs, err := readDocuments(filepath.Join(path, file.Name()))
		if err != nil {
			return nil, nil, err
		}

		for _, doc := range docs {
			var generic metav1.PartialObjectMetadata
			if err = yaml.Unmarshal(doc, &generic); err != nil {
				return nil, nil, err
			}

			switch {
			case generic.Kind == "MutatingWebhookConfiguration":
				var hook admissionreg.MutatingWebhookConfiguration
				if err := yaml.Unmarshal(doc, &hook); err != nil {
					return nil, nil, err
				}
				mutHooks = append(mutHooks, &hook)
			case generic.Kind == "ValidatingWebhookConfiguration":
				var hook admissionreg.ValidatingWebhookConfiguration
				if err := yaml.Unmarshal(doc, &hook); err != nil {
					return nil, nil, err
				}
				valHooks = append(valHooks, &hook)
			default:
				continue
			}
		}

		log.V(1).Info("read webhooks from file", "file", file.Name())
	}
	return mutHooks, valHooks, nil
}
