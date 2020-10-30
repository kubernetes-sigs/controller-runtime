package testingclient_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestTestingclient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Testingclient Suite")
}
