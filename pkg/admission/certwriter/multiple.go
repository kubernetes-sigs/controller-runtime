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

package certwriter

import (
	"k8s.io/apimachinery/pkg/runtime"
)

// MultiCertWriterProvider composes a slice of CertWriterProviders.
type MultiCertWriterProvider struct {
	CertWriterProviders []CertWriterProvider
}

var _ CertWriterProvider = &MultiCertWriterProvider{}

// Provide creates a new CertWriter and initialized with the passed-in webhookConfig
func (s *MultiCertWriterProvider) Provide(webhookConfig runtime.Object) (CertWriter, error) {
	writers := make([]CertWriter, len(s.CertWriterProviders))
	var err error
	for i, certWriterProvider := range s.CertWriterProviders {
		writers[i], err = certWriterProvider.Provide(webhookConfig)
		if err != nil {
			return nil, err
		}
	}
	return &multiCertWriter{certWriters: writers}, nil
}

// multiCertWriter composes a slice of CertWriters.
type multiCertWriter struct {
	certWriters []CertWriter
}

var _ CertWriter = &multiCertWriter{}

// EnsureCert processes the webhooks managed by this CertWriter.
// It provisions the certificate and update the CA in the webhook.
func (s *multiCertWriter) EnsureCert() error {
	for _, certWriter := range s.certWriters {
		err := certWriter.EnsureCert()
		if err != nil {
			return err
		}
	}
	return nil
}
