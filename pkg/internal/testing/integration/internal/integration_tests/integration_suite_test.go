package integrationtests

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
)

func TestIntegration(t *testing.T) {
	t.Parallel()
	RegisterFailHandler(Fail)
	suiteName := "Integration Framework Integration Tests"
	RunSpecsWithDefaultAndCustomReporters(t, suiteName, []Reporter{printer.NewlineReporter{}, printer.NewProwReporter(suiteName)})
}
