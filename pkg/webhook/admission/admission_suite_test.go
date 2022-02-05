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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/reporters"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var suiteName = "Admission Webhook Suite"

func TestAdmissionWebhook(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, suiteName)
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
})

var _ = ReportAfterSuite("Report to Prow", func(report Report) {
	reporters.ReportViaDeprecatedReporter(printer.NewlineReporter{}, report)          //nolint // For migration of custom reporter check https://onsi.github.io/ginkgo/MIGRATING_TO_V2#migration-strategy-2
	reporters.ReportViaDeprecatedReporter(printer.NewProwReporter(suiteName), report) //nolint // For migration of custom reporter check https://onsi.github.io/ginkgo/MIGRATING_TO_V2#migration-strategy-2
})
