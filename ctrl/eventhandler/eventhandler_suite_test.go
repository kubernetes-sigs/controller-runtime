package eventhandler_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestEventhandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Eventhandler Suite")
}
