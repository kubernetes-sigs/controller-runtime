package internal_test

import (
	"net/url"

	. "github.com/kubernetes-sig-testing/frameworks/integration/internal"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Etcd", func() {
	It("can create Etcd arguments", func() {
		input := DefaultedProcessInput{
			URL: url.URL{
				Scheme: "http",
				Host:   "some.etcd.service:5432",
			},
			Dir: "/some/data/dir",
		}

		args := MakeEtcdArgs(input)
		Expect(args).To(ContainElement("--advertise-client-urls=http://some.etcd.service:5432"))
		Expect(args).To(ContainElement("--listen-client-urls=http://some.etcd.service:5432"))
		Expect(args).To(ContainElement("--data-dir=/some/data/dir"))
	})
})

var _ = Describe("GetEtcdStartMessage()", func() {
	Context("when using a non tls URL", func() {
		It("generates valid start message", func() {
			url := url.URL{
				Scheme: "http",
				Host:   "some.insecure.host:1234",
			}
			message := GetEtcdStartMessage(url)
			Expect(message).To(Equal("serving insecure client requests on some.insecure.host"))
		})
	})
	Context("when using a tls URL", func() {
		It("generates valid start message", func() {
			url := url.URL{
				Scheme: "https",
				Host:   "some.secure.host:8443",
			}
			message := GetEtcdStartMessage(url)
			Expect(message).To(Equal("serving client requests on some.secure.host"))
		})
	})
})
