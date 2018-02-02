package integration_test

import (
	"bytes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/kubernetes-sig-testing/frameworks/integration"
)

var _ = Describe("Etcd", func() {
	It("sets the properties after defaulting", func() {
		etcd := &Etcd{}

		Expect(etcd.URL).To(BeZero())
		Expect(etcd.DataDir).To(BeZero())
		Expect(etcd.Path).To(BeZero())
		Expect(etcd.StartTimeout).To(BeZero())
		Expect(etcd.StopTimeout).To(BeZero())

		Expect(etcd.Start()).To(Succeed())
		defer func() {
			Expect(etcd.Stop()).To(Succeed())
		}()

		Expect(etcd.URL).NotTo(BeZero())
		Expect(etcd.DataDir).NotTo(BeZero())
		Expect(etcd.Path).NotTo(BeZero())
		Expect(etcd.StartTimeout).NotTo(BeZero())
		Expect(etcd.StopTimeout).NotTo(BeZero())
	})

	It("can inspect IO", func() {
		stderr := &bytes.Buffer{}
		etcd := &Etcd{
			Err: stderr,
		}

		Expect(etcd.Start()).To(Succeed())
		defer func() {
			Expect(etcd.Stop()).To(Succeed())
		}()

		Expect(stderr.String()).NotTo(BeEmpty())
	})
})
