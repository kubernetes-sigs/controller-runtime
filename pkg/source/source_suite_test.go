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

package source_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/reporters"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var suiteName = "Source Suite"

func TestSource(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, suiteName)
}

var testenv *envtest.Environment
var config *rest.Config
var clientset *kubernetes.Clientset
var icache cache.Cache
var ctx context.Context
var cancel context.CancelFunc

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	testenv = &envtest.Environment{}

	var err error
	config, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())

	clientset, err = kubernetes.NewForConfig(config)
	Expect(err).NotTo(HaveOccurred())

	icache, err = cache.New(config, cache.Options{})
	Expect(err).NotTo(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		Expect(icache.Start(ctx)).NotTo(HaveOccurred())
	}()
})

var _ = AfterSuite(func() {
	cancel()
	Expect(testenv.Stop()).To(Succeed())
})

var _ = ReportAfterSuite("Report to Prow", func(report Report) {
	reporters.ReportViaDeprecatedReporter(printer.NewlineReporter{}, report)          //nolint // For migration of custom reporter check https://onsi.github.io/ginkgo/MIGRATING_TO_V2#migration-strategy-2
	reporters.ReportViaDeprecatedReporter(printer.NewProwReporter(suiteName), report) //nolint // For migration of custom reporter check https://onsi.github.io/ginkgo/MIGRATING_TO_V2#migration-strategy-2
})
