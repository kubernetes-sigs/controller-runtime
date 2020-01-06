package integrationtests

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "sigs.k8s.io/controller-runtime/pkg/internal/testing/integration"
)

var _ = Describe("APIServer", func() {
	Context("when no EtcdURL is provided", func() {
		It("does not panic", func() {
			apiServer := &APIServer{}

			starter := func() {
				Expect(apiServer.Start()).To(
					MatchError(ContainSubstring("expected EtcdURL to be configured")),
				)
			}

			Expect(starter).NotTo(Panic())
		})
	})
})
