package eventhandler_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	logf "github.com/kubernetes-sigs/kubebuilder/pkg/log"
)

func TestEventhandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Eventhandler Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(logf.ZapLogger(true))
})
