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
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/testing_frameworks/integration/addr"
)

func TestBuilder(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "application Suite", []Reporter{envtest.NewlineReporter{}})
}

var testenv *envtest.Environment
var cfg *rest.Config

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.LoggerTo(GinkgoWriter, true))

	testenv = &envtest.Environment{}
	addCRDToEnvironment(testenv,
		testDefaulterGVK,
		testValidatorGVK,
		testDefaultValidatorGVK)

	var err error
	cfg, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())

	// Prevent the metrics listener being created
	metrics.DefaultBindAddress = "0"

	webhook.DefaultPort, _, err = addr.Suggest()
	Expect(err).NotTo(HaveOccurred())

	close(done)
}, 60)

var _ = AfterSuite(func() {
	Expect(testenv.Stop()).To(Succeed())

	// Put the DefaultBindAddress back
	metrics.DefaultBindAddress = ":8080"

	// Change the webhook.DefaultPort back to the original default.
	webhook.DefaultPort = 443
})

func addCRDToEnvironment(env *envtest.Environment, gvks ...schema.GroupVersionKind) {
	for _, gvk := range gvks {
		plural, singlar := meta.UnsafeGuessKindToResource(gvk)
		crd := &apiextensionsv1beta1.CustomResourceDefinition{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "apiextensions.k8s.io",
				Kind:       "CustomResourceDefinition",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: plural.Resource + "." + gvk.Group,
			},
			Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
				Group:   gvk.Group,
				Version: gvk.Version,
				Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
					Plural:   plural.Resource,
					Singular: singlar.Resource,
					Kind:     gvk.Kind,
				},
				Versions: []apiextensionsv1beta1.CustomResourceDefinitionVersion{
					{
						Name:    gvk.Version,
						Served:  true,
						Storage: true,
					},
				},
			},
		}
		env.CRDs = append(env.CRDs, crd)
	}
}
