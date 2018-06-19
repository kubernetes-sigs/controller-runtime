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
	//. "github.com/onsi/ginkgo"
	//. "github.com/onsi/gomega"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/admission/certgenerator"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("NewProvider", func() {
	var cl client.Client
	var ops Options
	var expectedProvider CertWriterProvider
	BeforeEach(func() {
		ops = Options{}
	})

	Describe("required client is missing", func() {
		It("should return an error", func() {
			_, err := NewProvider(ops)
			Expect(err).To(MatchError("Options.Client is required"))
		})
	})

	Describe("succeed", func() {
		BeforeEach(func() {
			cl = fake.NewFakeClient()
			ops.Client = cl
			expectedProvider = &MultiCertWriterProvider{
				CertWriterProviders: []CertWriterProvider{
					&SecretCertWriterProvider{
						Client:        cl,
						CertGenerator: &certgenerator.SelfSignedCertGenerator{},
					},
					&FSCertWriterProvider{
						CertGenerator: &certgenerator.SelfSignedCertGenerator{},
					},
				},
			}
		})
		It("should successfully return a Provider", func() {
			provider, err := NewProvider(ops)
			Expect(err).NotTo(HaveOccurred())
			Expect(provider).To(Equal(expectedProvider))
		})
	})

})
