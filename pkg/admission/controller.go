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
	"reflect"
	"sync"

	"k8s.io/apimachinery/pkg/runtime"

	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/admission/certgenerator"
	"sigs.k8s.io/controller-runtime/pkg/admission/certwriter"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// CertProvisioner provisions certificates for webhook configurations and writes them to an output
// destination - such as a Secret or local file. CertProvisioner can update the CA field of
// certain resources with the CA of the certs.
type CertProvisioner struct {
	Client client.Client
	// CertGenerator generates certificate for a given common name.
	CertGenerator      certgenerator.CertGenerator
	CertWriterProvider certwriter.CertWriterProvider

	once *sync.Once
}

// Sync takes a runtime.Object which is expected to be either a MutatingWebhookConfiguration or
// a ValidatingWebhookConfiguration.
// It provisions the certs for each webhook in the webhookConfiguration, ensures the cert and CA are valid and
// update the CABundle in the webhook configuration if necessary.
func (cp *CertProvisioner) Sync(webhookConfiguration runtime.Object) error {
	var err error
	// Do the initialization for CertInput only once.
	cp.once.Do(func() {
		if cp.CertGenerator == nil {
			cp.CertGenerator = &certgenerator.SelfSignedCertGenerator{}
		}
		if cp.Client == nil {
			cp.Client, err = client.New(config.GetConfigOrDie(), client.Options{})
			if err != nil {
				return
			}
		}
		if cp.CertWriterProvider == nil {
			cp.CertWriterProvider, err = certwriter.NewProvider(
				certwriter.Options{
					Client:        cp.Client,
					CertGenerator: cp.CertGenerator,
				})
			if err != nil {
				return
			}
		}
	})
	if err != nil {
		return fmt.Errorf("failed to default the CertProvision: %v", err)
	}

	cloned := webhookConfiguration.DeepCopyObject()

	writer, err := cp.CertWriterProvider.Provide(cloned)
	if err != nil {
		return err
	}

	if err := writer.EnsureCert(); err != nil {
		return err
	}

	if reflect.DeepEqual(webhookConfiguration, cloned) {
		return nil
	}
	return cp.Client.Update(nil, cloned)
}
