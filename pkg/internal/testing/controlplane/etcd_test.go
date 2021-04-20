package controlplane_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "sigs.k8s.io/controller-runtime/pkg/internal/testing/controlplane"
)

var _ = Describe("etcd", func() {
	// basic coherence test
	It("should start and stop successfully", func() {
		etcd := &Etcd{}
		Expect(etcd.Start()).To(Succeed())
		defer func() {
			Expect(etcd.Stop()).To(Succeed())
		}()
		Expect(etcd.URL).NotTo(BeNil())
	})
})
