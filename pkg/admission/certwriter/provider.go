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

	"errors"

	"sigs.k8s.io/controller-runtime/pkg/admission/certgenerator"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Options are options for configuring a CertWriterProvider.
type Options struct {
	Client        client.Client
	CertGenerator certgenerator.CertGenerator
}

type CertWriterProvider interface {
	// Provide provides a new CertWriter with the given runtime.Object
	Provide(runtime.Object) (CertWriter, error)
}

// New builds a new CertWriter for the provided webhook configuration.
func NewProvider(ops Options) (CertWriterProvider, error) {
	if ops.CertGenerator == nil {
		ops.CertGenerator = &certgenerator.SelfSignedCertGenerator{}
	}
	if ops.Client == nil {
		// TODO: default the client if possible
		return nil, errors.New("Options.Client is required")
	}
	s := &SecretCertWriterProvider{
		Client:        ops.Client,
		CertGenerator: ops.CertGenerator,
	}
	f := &FSCertWriterProvider{
		CertGenerator: ops.CertGenerator,
	}
	return &MultiCertWriterProvider{
		CertWriterProviders: []CertWriterProvider{
			s,
			f,
		},
	}, nil
}
