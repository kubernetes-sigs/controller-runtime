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

package fake

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/admission/certgenerator"
)

// FakeCertGenerator is a CertGenerator for testing.
type FakeCertGenerator struct {
	DnsNameToCertArtifacts map[string]*certgenerator.CertArtifacts
}

var _ certgenerator.CertGenerator = &FakeCertGenerator{}

func (cp *FakeCertGenerator) Generate(commonName string) (*certgenerator.CertArtifacts, error) {
	certs, found := cp.DnsNameToCertArtifacts[commonName]
	if !found {
		return nil, fmt.Errorf("failed to find common name %q in the FakeCertGenerator", commonName)
	}
	return certs, nil
}
