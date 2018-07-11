/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package writer

import (
	goerrors "errors"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/admission/cert/generator"
	"sigs.k8s.io/controller-runtime/pkg/client"

	admissionregistration "k8s.io/api/admissionregistration/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var certs1, certs2 *generator.Artifacts

func init() {
	cn1 := "example.com"
	cn2 := "test-service.test-svc-namespace.svc"
	cp := generator.SelfSignedCertGenerator{}
	certs1, _ = cp.Generate(cn1)
	certs2, _ = cp.Generate(cn2)
}

var _ = Describe("NewProvider", func() {
	var cl client.Client
	var ops Options
	var expectedProvider CertWriter
	BeforeEach(func(done Done) {
		ops = Options{}
		close(done)
	})

	Describe("required client is missing", func() {
		It("should return an error", func() {
			_, err := NewCertWriter(ops)
			Expect(err).To(MatchError("Options.Client is required"))
		})
	})

	Describe("succeed", func() {
		BeforeEach(func(done Done) {
			cl = fake.NewFakeClient()
			ops.Client = cl
			expectedProvider = &MultiCertWriter{
				CertWriters: []CertWriter{
					&SecretCertWriter{
						Client:        cl,
						CertGenerator: &generator.SelfSignedCertGenerator{},
					},
					&FSCertWriter{
						CertGenerator: &generator.SelfSignedCertGenerator{},
					},
				},
			}
			close(done)
		})
		It("should successfully return a Provider", func() {
			provider, err := NewCertWriter(ops)
			Expect(err).NotTo(HaveOccurred())
			Expect(provider).To(Equal(expectedProvider))
		})
	})

})

type fakeCertReadWriter struct {
	numReadCalled  int
	readCertAndErr []certAndErr

	numWriteCalled  int
	writeCertAndErr []certAndErr

	numOverwriteCalled  int
	overwriteCertAndErr []certAndErr
}

type certAndErr struct {
	cert *generator.Artifacts
	err  error
}

var _ certReadWriter = &fakeCertReadWriter{}

func (f *fakeCertReadWriter) read(webhookName string) (*generator.Artifacts, error) {
	defer func() { f.numReadCalled++ }()

	if len(f.readCertAndErr) <= f.numReadCalled {
		return &generator.Artifacts{}, nil
	}
	certAndErr := f.readCertAndErr[f.numReadCalled]
	return certAndErr.cert, certAndErr.err
}

func (f *fakeCertReadWriter) write(webhookName string) (*generator.Artifacts, error) {
	defer func() { f.numWriteCalled++ }()

	if len(f.writeCertAndErr) <= f.numWriteCalled {
		return &generator.Artifacts{}, nil
	}
	certAndErr := f.writeCertAndErr[f.numWriteCalled]
	return certAndErr.cert, certAndErr.err
}

func (f *fakeCertReadWriter) overwrite(webhookName string) (*generator.Artifacts, error) {
	defer func() { f.numOverwriteCalled++ }()

	if len(f.overwriteCertAndErr) <= f.numOverwriteCalled {
		return &generator.Artifacts{}, nil
	}
	certAndErr := f.overwriteCertAndErr[f.numOverwriteCalled]
	return certAndErr.cert, certAndErr.err
}

var _ = Describe("handleCommon", func() {
	var webhook *admissionregistration.Webhook
	var cert *generator.Artifacts
	var invalidCert *generator.Artifacts

	BeforeEach(func(done Done) {
		url := "https://example.com/admission"
		webhook = &admissionregistration.Webhook{
			ClientConfig: admissionregistration.WebhookClientConfig{
				URL: &url,
			},
		}
		cert = &generator.Artifacts{
			CACert: []byte(certs1.CACert),
			Cert:   []byte(certs1.Cert),
			Key:    []byte(certs1.Key),
		}
		invalidCert = &generator.Artifacts{
			CACert: []byte(`CACertBytes`),
			Cert:   []byte(`CertBytes`),
			Key:    []byte(`KeyBytes`),
		}
		close(done)
	})

	Context("when webhook is nil", func() {
		It("should return no error", func() {
			certrw := &fakeCertReadWriter{}
			err := handleCommon(nil, certrw)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when certReadWriter is nil", func() {
		It("should return an error", func() {
			err := handleCommon(webhook, nil)
			Expect(err).To(MatchError(goerrors.New("certReaderWriter should not be nil")))
		})
	})

	Context("cert doesn't exist", func() {
		It("should return no error on successful write", func() {
			certrw := &fakeCertReadWriter{
				readCertAndErr: []certAndErr{
					{
						err: notFoundError{errors.NewNotFound(schema.GroupResource{}, "foo")},
					},
				},
				writeCertAndErr: []certAndErr{
					{
						cert: cert,
					},
				},
			}

			err := handleCommon(webhook, certrw)
			Expect(err).NotTo(HaveOccurred())
			Expect(certrw.numReadCalled).To(Equal(1))
			Expect(certrw.numWriteCalled).To(Equal(1))
			Expect(certrw.numOverwriteCalled).To(Equal(0))
		})

		It("should return the error on failed write", func() {
			certrw := &fakeCertReadWriter{
				readCertAndErr: []certAndErr{
					{
						err: notFoundError{errors.NewNotFound(schema.GroupResource{}, "foo")},
					},
				},
				writeCertAndErr: []certAndErr{
					{
						err: goerrors.New("failed to write"),
					},
				},
			}

			err := handleCommon(webhook, certrw)
			Expect(err).To(MatchError(goerrors.New("failed to write")))
			Expect(certrw.numReadCalled).To(Equal(1))
			Expect(certrw.numWriteCalled).To(Equal(1))
			Expect(certrw.numOverwriteCalled).To(Equal(0))
		})
	})

	Context("valid cert exist", func() {
		It("should return no error on successful read", func() {
			certrw := &fakeCertReadWriter{
				readCertAndErr: []certAndErr{
					{
						cert: cert,
					},
				},
			}

			err := handleCommon(webhook, certrw)
			Expect(err).NotTo(HaveOccurred())
			Expect(certrw.numReadCalled).To(Equal(1))
			Expect(certrw.numWriteCalled).To(Equal(0))
			Expect(certrw.numOverwriteCalled).To(Equal(0))
		})

		It("should return the error on failed read", func() {
			certrw := &fakeCertReadWriter{
				readCertAndErr: []certAndErr{
					{
						err: goerrors.New("failed to read"),
					},
				},
			}

			err := handleCommon(webhook, certrw)
			Expect(err).To(MatchError(goerrors.New("failed to read")))
			Expect(certrw.numReadCalled).To(Equal(1))
			Expect(certrw.numWriteCalled).To(Equal(0))
			Expect(certrw.numOverwriteCalled).To(Equal(0))
		})
	})

	Context("invalid cert exist", func() {
		It("should replace the empty cert with a new one", func() {
			certrw := &fakeCertReadWriter{
				readCertAndErr: []certAndErr{
					{
						cert: nil,
					},
				},
				overwriteCertAndErr: []certAndErr{
					{
						cert: cert,
					},
				},
			}

			err := handleCommon(webhook, certrw)
			Expect(err).NotTo(HaveOccurred())
			Expect(certrw.numReadCalled).To(Equal(1))
			Expect(certrw.numWriteCalled).To(Equal(0))
			Expect(certrw.numOverwriteCalled).To(Equal(1))
		})

		It("should return no error on successful overwrite", func() {
			certrw := &fakeCertReadWriter{
				readCertAndErr: []certAndErr{
					{
						cert: invalidCert,
					},
				},
				overwriteCertAndErr: []certAndErr{
					{
						cert: cert,
					},
				},
			}

			err := handleCommon(webhook, certrw)
			Expect(err).NotTo(HaveOccurred())
			Expect(certrw.numReadCalled).To(Equal(1))
			Expect(certrw.numWriteCalled).To(Equal(0))
			Expect(certrw.numOverwriteCalled).To(Equal(1))
		})

		It("should return the error on failed overwrite", func() {
			certrw := &fakeCertReadWriter{
				readCertAndErr: []certAndErr{
					{
						cert: invalidCert,
					},
				},
				overwriteCertAndErr: []certAndErr{
					{
						err: goerrors.New("failed to overwrite"),
					},
				},
			}

			err := handleCommon(webhook, certrw)
			Expect(err).To(MatchError(goerrors.New("failed to overwrite")))
			Expect(certrw.numReadCalled).To(Equal(1))
			Expect(certrw.numOverwriteCalled).To(Equal(1))
		})
	})

	Context("racing", func() {
		It("should return the valid cert created by the racing one", func() {
			certrw := &fakeCertReadWriter{
				readCertAndErr: []certAndErr{
					{
						err: notFoundError{errors.NewNotFound(schema.GroupResource{}, "foo")},
					},
					{
						cert: cert,
					},
				},
				writeCertAndErr: []certAndErr{
					{
						err: alreadyExistError{errors.NewAlreadyExists(schema.GroupResource{}, "foo")},
					},
				},
			}

			err := handleCommon(webhook, certrw)
			Expect(err).NotTo(HaveOccurred())
			Expect(certrw.numReadCalled).To(Equal(2))
			Expect(certrw.numWriteCalled).To(Equal(1))
		})

		It("should return the error if failed to read the cert created by the racing one", func() {
			certrw := &fakeCertReadWriter{
				readCertAndErr: []certAndErr{
					{
						err: notFoundError{errors.NewNotFound(schema.GroupResource{}, "foo")},
					},
					{
						err: goerrors.New("failed to read"),
					},
				},
				writeCertAndErr: []certAndErr{
					{
						err: alreadyExistError{errors.NewAlreadyExists(schema.GroupResource{}, "foo")},
					},
				},
			}

			err := handleCommon(webhook, certrw)
			Expect(err).To(MatchError(goerrors.New("failed to read")))
			Expect(certrw.numReadCalled).To(Equal(2))
			Expect(certrw.numWriteCalled).To(Equal(1))
		})
	})
})

var _ = Describe("dnsNameForWebhook", func() {
	var webhookClientConfig *admissionregistration.WebhookClientConfig
	Context("when both service and URL are set", func() {
		It("should return an error", func() {
			url := "https://foo.bar.com/blah"
			webhookClientConfig = &admissionregistration.WebhookClientConfig{
				Service: &admissionregistration.ServiceReference{
					Namespace: "test-ns",
					Name:      "test-service",
				},
				URL: &url,
			}
			_, err := dnsNameForWebhook(webhookClientConfig)
			Expect(err).To(MatchError(fmt.Errorf("service and URL can't be set at the same time in a webhook: %v", webhookClientConfig)))
		})
	})

	Context("when neither service nor URL is set", func() {
		It("should return an error", func() {
			webhookClientConfig = &admissionregistration.WebhookClientConfig{}
			_, err := dnsNameForWebhook(webhookClientConfig)
			Expect(err).To(MatchError(fmt.Errorf("one of service and URL need to be set in a webhook: %v", webhookClientConfig)))
		})
	})

	Context("when only service is set", func() {
		It("should return the common name and no error", func() {
			webhookClientConfig = &admissionregistration.WebhookClientConfig{
				Service: &admissionregistration.ServiceReference{
					Namespace: "test-ns",
					Name:      "test-service",
				},
			}
			cn, err := dnsNameForWebhook(webhookClientConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(cn).To(Equal("test-service.test-ns.svc"))
		})
	})

	Context("when only URL is set", func() {
		It("should return the common name and no error", func() {
			url := "https://foo.bar.com/blah"
			webhookClientConfig = &admissionregistration.WebhookClientConfig{
				URL: &url,
			}
			cn, err := dnsNameForWebhook(webhookClientConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(cn).To(Equal("foo.bar.com"))
		})
	})
})

var _ = Describe("validate cert", func() {
	Context("invalid pair", func() {
		It("should detect it", func() {
			certs := generator.Artifacts{
				CACert: certs1.CACert,
				Cert:   certs1.Cert,
				Key:    certs2.Key,
			}
			valid := validCert(&certs, "example.com")
			Expect(valid).To(BeFalse())
		})
	})

	Context("CA not matching", func() {
		It("should detect it", func() {
			certs := generator.Artifacts{
				CACert: certs2.CACert,
				Cert:   certs1.Cert,
				Key:    certs1.Key,
			}
			valid := validCert(&certs, "example.com")
			Expect(valid).To(BeFalse())
		})
	})

	Context("DNS name not matching", func() {
		It("should detect it", func() {
			certs := generator.Artifacts{
				CACert: certs1.CACert,
				Cert:   certs1.Cert,
				Key:    certs1.Key,
			}
			valid := validCert(&certs, "foo.com")
			Expect(valid).To(BeFalse())
		})
	})
})
