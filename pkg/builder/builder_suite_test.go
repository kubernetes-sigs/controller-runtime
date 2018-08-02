package builder

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func TestSource(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "application Suite", []Reporter{envtest.NewlineReporter{}})
}

var testenv *envtest.Environment
var cfg *rest.Config

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(logf.ZapLoggerTo(GinkgoWriter, true))

	testenv = &envtest.Environment{}

	var err error
	cfg, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())

	close(done)
}, 60)

var _ = AfterSuite(func() {
	testenv.Stop()
})
