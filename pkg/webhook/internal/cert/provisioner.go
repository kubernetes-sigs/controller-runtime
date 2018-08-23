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

package cert

import (
	"fmt"
	"reflect"
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/webhook/internal/cert/generator"
	"sigs.k8s.io/controller-runtime/pkg/webhook/internal/cert/writer"
)

// Provisioner provisions certificates for webhook configurations and writes them to an output
// destination - such as a Secret or local file. Provisioner can update the CA field of
// certain resources with the CA of the certs.
type Provisioner struct {
	Client client.Client
	// CertGenerator generates certificate for a given common name.
	CertGenerator generator.CertGenerator
	// CertWriter knows how to persist the certificate.
	CertWriter writer.CertWriter

	once sync.Once
}

// Sync takes a runtime.Object which is expected to be either a MutatingWebhookConfiguration or
// a ValidatingWebhookConfiguration.
// It provisions certificate for each webhook in the webhookConfiguration, ensures the cert and CA are valid,
// and not expiring. It updates the CABundle in the webhook configuration if necessary.
func (cp *Provisioner) Sync(webhookConfiguration runtime.Object) error {
	var err error
	// Do the initialization for CertInput only once.
	cp.once.Do(func() {
		if cp.CertGenerator == nil {
			cp.CertGenerator = &generator.SelfSignedCertGenerator{}
		}
		if cp.Client == nil {
			cp.Client, err = client.New(config.GetConfigOrDie(), client.Options{})
			if err != nil {
				return
			}
		}
		if cp.CertWriter == nil {
			cp.CertWriter, err = writer.NewCertWriter(
				writer.Options{
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

	// Deepcopy the webhook configuration object before invoking EnsureCerts,
	// since EnsureCerts will modify the provided object.
	cloned := webhookConfiguration.DeepCopyObject()
	err = cp.CertWriter.EnsureCerts(webhookConfiguration)
	if err != nil {
		return err
	}

	// If some fields have been changed, we will update the object.
	// Mostly this is because of the CABundle field has been updated.
	if reflect.DeepEqual(webhookConfiguration, cloned) {
		return nil
	}
	return nil
	// TODO: figure what we want to do with this.
	// Disable the auto-updating in this function.
	//return cp.Client.Update(context.Background(), cloned)
}
