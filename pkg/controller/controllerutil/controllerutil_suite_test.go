package controllerutil_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestControllerutil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controllerutil Suite")
}
