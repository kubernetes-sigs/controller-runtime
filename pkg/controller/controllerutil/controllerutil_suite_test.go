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

package controllerutil_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/reporters"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
)

var suiteName = "Controllerutil Suite"

func TestControllerutil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, suiteName)
}

var testenv *envtest.Environment
var cfg *rest.Config
var c client.Client

var _ = BeforeSuite(func() {
	var err error

	testenv = &envtest.Environment{}

	cfg, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())

	c, err = client.New(cfg, client.Options{})
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	Expect(testenv.Stop()).To(Succeed())
})

var _ = ReportAfterSuite("Report to Prow", func(report Report) {
	reporters.ReportViaDeprecatedReporter(printer.NewlineReporter{}, report)          //nolint // For migration of custom reporter check https://onsi.github.io/ginkgo/MIGRATING_TO_V2#migration-strategy-2
	reporters.ReportViaDeprecatedReporter(printer.NewProwReporter(suiteName), report) //nolint // For migration of custom reporter check https://onsi.github.io/ginkgo/MIGRATING_TO_V2#migration-strategy-2
})
