package ctrl_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestCtrl(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ctrl Suite")
}
