package versions_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestVersions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Versions Suite")
}
