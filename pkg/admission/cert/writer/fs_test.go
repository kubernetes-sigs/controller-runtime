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
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sync/atomic"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	admissionregistration "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/admission/cert/generator"
	fakegenerator "sigs.k8s.io/controller-runtime/pkg/admission/cert/generator/fake"
)

var _ = Describe("FSCertWriter", func() {
	var mwc *admissionregistration.MutatingWebhookConfiguration
	var vwc *admissionregistration.ValidatingWebhookConfiguration
	var url string
	var count uint64 = 0

	BeforeEach(func(done Done) {
		atomic.AddUint64(&count, 1)
		url = "https://example.com/admission"
		mwc = &admissionregistration.MutatingWebhookConfiguration{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "admissionregistration.k8s.io/v1beta1",
				Kind:       "MutatingWebhookConfiguration",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        "test-mwc",
				Namespace:   "test-ns",
				UID:         types.UID("123456"),
				Annotations: map[string]string{},
			},
			Webhooks: []admissionregistration.Webhook{
				{
					Name: "webhook-1",
					ClientConfig: admissionregistration.WebhookClientConfig{
						Service: &admissionregistration.ServiceReference{
							Namespace: "test-svc-namespace",
							Name:      "test-service",
						},
						CABundle: []byte(``),
					},
				},
			},
		}
		vwc = &admissionregistration.ValidatingWebhookConfiguration{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "admissionregistration.k8s.io/v1beta1",
				Kind:       "ValidatingWebhookConfiguration",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        "test-vwc",
				Namespace:   "test-ns",
				UID:         types.UID("123457"),
				Annotations: map[string]string{},
			},
			Webhooks: []admissionregistration.Webhook{
				{
					Name: "webhook-1",
					ClientConfig: admissionregistration.WebhookClientConfig{
						Service: &admissionregistration.ServiceReference{
							Namespace: "test-svc-namespace",
							Name:      "test-service",
						},
						CABundle: []byte(``),
					},
				},
				{
					Name: "webhook-2",
					ClientConfig: admissionregistration.WebhookClientConfig{
						URL:      &url,
						CABundle: []byte(``),
					},
				},
			},
		}
		close(done)
	})

	Describe("EnsureCerts", func() {
		var certWriter CertWriter
		var fsCertWriter *FSCertWriter

		Context("Failed to EnsureCerts", func() {
			BeforeEach(func(done Done) {
				certWriter = &FSCertWriter{}
				close(done)
			})

			Describe("nil object", func() {
				It("should return error", func() {
					err := certWriter.EnsureCerts(nil)
					Expect(err).To(MatchError("unexpected nil webhook configuration object"))
				})
			})

			Describe("non-webhook configuration object", func() {
				It("should return error", func() {
					err := certWriter.EnsureCerts(&corev1.Pod{})
					Expect(err).To(MatchError(fmt.Errorf("unsupported type: %T, only support v1beta1.MutatingWebhookConfiguration and v1beta1.ValidatingWebhookConfiguration",
						&corev1.Pod{})))
				})
			})

			Describe("can't find webhook name specified in the annotations", func() {
				BeforeEach(func(done Done) {
					mwc.Annotations["fs.certprovisioner.kubernetes.io/webhook-does-not-exist"] = "somepath/"
					vwc.Annotations["fs.certprovisioner.kubernetes.io/webhook-does-not-exist"] = "somepath/"
					close(done)
				})
				It("should return error", func() {
					err := certWriter.EnsureCerts(mwc)
					Expect(err).To(MatchError(fmt.Errorf("expecting a webhook named %q", "webhook-does-not-exist")))
					err = certWriter.EnsureCerts(vwc)
					Expect(err).To(MatchError(fmt.Errorf("expecting a webhook named %q", "webhook-does-not-exist")))
				})
			})
		})

		Context("Succeeded to EnsureCerts", func() {
			var testingDir string
			BeforeEach(func(done Done) {
				var err error
				testingDir, err = ioutil.TempDir("", "testdir")
				Expect(err).NotTo(HaveOccurred())
				mwc.Annotations["fs.certprovisioner.kubernetes.io/webhook-1"] = testingDir
				fsCertWriter = &FSCertWriter{
					CertGenerator: &fakegenerator.CertGenerator{
						DNSNameToCertArtifacts: map[string]*generator.Artifacts{
							"test-service.test-svc-namespace.svc": {
								CACert: []byte(certs2.CACert),
								Cert:   []byte(certs2.Cert),
								Key:    []byte(certs2.Key),
							},
						},
					},
				}
				certWriter = fsCertWriter
				close(done)
			})

			AfterEach(func() {
				os.RemoveAll(testingDir)
			})

			Context("CertGenerator is not set", func() {
				BeforeEach(func(done Done) {
					fsCertWriter = &FSCertWriter{}
					certWriter = fsCertWriter
					close(done)
				})

				It("should default it and return no error", func() {
					err := certWriter.EnsureCerts(mwc)
					Expect(err).NotTo(HaveOccurred())

				})
			})

			Context("no existing secret", func() {
				It("should create new secrets with certs", func() {
					err := certWriter.EnsureCerts(mwc)
					Expect(err).NotTo(HaveOccurred())
					caBytes, err := ioutil.ReadFile(path.Join(testingDir, CACertName))
					Expect(err).NotTo(HaveOccurred())
					Expect(caBytes).To(Equal([]byte(certs2.CACert)))
					certBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerCertName))
					Expect(err).NotTo(HaveOccurred())
					Expect(certBytes).To(Equal([]byte(certs2.Cert)))
					keyBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerKeyName))
					Expect(err).NotTo(HaveOccurred())
					Expect(keyBytes).To(Equal([]byte(certs2.Key)))
				})
			})

			Context("old secret exists", func() {
				Context("cert is invalid", func() {
					Describe("cert in secret is incomplete", func() {
						Context("cert file is not a symbolic link", func() {
							BeforeEach(func(done Done) {
								err := ioutil.WriteFile(path.Join(testingDir, CACertName), []byte(`oldCACertBytes`), 0600)
								Expect(err).NotTo(HaveOccurred())
								close(done)
							})

							It("should replace with new certs", func() {
								err := certWriter.EnsureCerts(mwc)
								Expect(err).NotTo(HaveOccurred())
								caBytes, err := ioutil.ReadFile(path.Join(testingDir, CACertName))
								Expect(err).NotTo(HaveOccurred())
								Expect(caBytes).To(Equal([]byte(certs2.CACert)))
								certBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerCertName))
								Expect(err).NotTo(HaveOccurred())
								Expect(certBytes).To(Equal([]byte(certs2.Cert)))
								keyBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerKeyName))
								Expect(err).NotTo(HaveOccurred())
								Expect(keyBytes).To(Equal([]byte(certs2.Key)))
							})
						})

						Context("cert file is a symbolic link", func() {
							BeforeEach(func(done Done) {
								dataDir := path.Join(testingDir, "..data")
								realDataDir := path.Join(testingDir, "..2018_06_01_15_04_05.12345678")
								caFileName := path.Join(testingDir, "..2018_06_01_15_04_05.12345678", CACertName)
								err := os.Mkdir(realDataDir, 0700)
								Expect(err).NotTo(HaveOccurred())
								err = ioutil.WriteFile(caFileName, []byte(`oldCACertBytes`), 0600)
								Expect(err).NotTo(HaveOccurred())
								err = os.Symlink(realDataDir, dataDir)
								Expect(err).NotTo(HaveOccurred())
								close(done)
							})

							It("should replace with new certs", func() {
								err := certWriter.EnsureCerts(mwc)
								Expect(err).NotTo(HaveOccurred())
								caBytes, err := ioutil.ReadFile(path.Join(testingDir, CACertName))
								Expect(err).NotTo(HaveOccurred())
								Expect(caBytes).To(Equal([]byte(certs2.CACert)))
								certBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerCertName))
								Expect(err).NotTo(HaveOccurred())
								Expect(certBytes).To(Equal([]byte(certs2.Cert)))
								keyBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerKeyName))
								Expect(err).NotTo(HaveOccurred())
								Expect(keyBytes).To(Equal([]byte(certs2.Key)))
							})
						})
					})

					Describe("cert content is invalid", func() {
						Context("cert files are not symbolic links", func() {
							BeforeEach(func(done Done) {
								ioutil.WriteFile(path.Join(testingDir, CACertName), []byte(`oldCACertBytes`), 0600)
								ioutil.WriteFile(path.Join(testingDir, ServerCertName), []byte(`oldCertBytes`), 0600)
								ioutil.WriteFile(path.Join(testingDir, ServerKeyName), []byte(`oldKeyBytes`), 0600)
								close(done)
							})

							It("should replace with new certs", func() {
								err := certWriter.EnsureCerts(mwc)
								Expect(err).NotTo(HaveOccurred())
								caBytes, err := ioutil.ReadFile(path.Join(testingDir, CACertName))
								Expect(err).NotTo(HaveOccurred())
								Expect(caBytes).To(Equal([]byte(certs2.CACert)))
								certBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerCertName))
								Expect(err).NotTo(HaveOccurred())
								Expect(certBytes).To(Equal([]byte(certs2.Cert)))
								keyBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerKeyName))
								Expect(err).NotTo(HaveOccurred())
								Expect(keyBytes).To(Equal([]byte(certs2.Key)))
							})
						})

						Context("cert files are symbolic links", func() {
							BeforeEach(func(done Done) {
								dataDir := path.Join(testingDir, "..data")
								realDataDir := path.Join(testingDir, "..2018_06_01_15_04_05.12345678")
								caFileName := path.Join(testingDir, "..2018_06_01_15_04_05.12345678", CACertName)
								certFileName := path.Join(testingDir, "..2018_06_01_15_04_05.12345678", ServerCertName)
								keyFileName := path.Join(testingDir, "..2018_06_01_15_04_05.12345678", ServerKeyName)
								err := os.Mkdir(realDataDir, 0700)
								Expect(err).NotTo(HaveOccurred())
								err = ioutil.WriteFile(caFileName, []byte(`oldCACertBytes`), 0600)
								Expect(err).NotTo(HaveOccurred())
								err = ioutil.WriteFile(certFileName, []byte(`oldCertBytes`), 0600)
								Expect(err).NotTo(HaveOccurred())
								err = ioutil.WriteFile(keyFileName, []byte(`oldKeyBytes`), 0600)
								Expect(err).NotTo(HaveOccurred())
								err = os.Symlink(realDataDir, dataDir)
								Expect(err).NotTo(HaveOccurred())
								close(done)
							})

							It("should replace with new certs", func() {
								err := certWriter.EnsureCerts(mwc)
								Expect(err).NotTo(HaveOccurred())
								caBytes, err := ioutil.ReadFile(path.Join(testingDir, CACertName))
								Expect(err).NotTo(HaveOccurred())
								Expect(caBytes).To(Equal([]byte(certs2.CACert)))
								certBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerCertName))
								Expect(err).NotTo(HaveOccurred())
								Expect(certBytes).To(Equal([]byte(certs2.Cert)))
								keyBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerKeyName))
								Expect(err).NotTo(HaveOccurred())
								Expect(keyBytes).To(Equal([]byte(certs2.Key)))
							})
						})
					})
				})
			})

			Context("cert is valid", func() {
				Context("when not expiring", func() {
					BeforeEach(func(done Done) {
						err := ioutil.WriteFile(path.Join(testingDir, CACertName), []byte(certs2.CACert), 0600)
						Expect(err).NotTo(HaveOccurred())
						err = ioutil.WriteFile(path.Join(testingDir, ServerCertName), []byte(certs2.Cert), 0600)
						Expect(err).NotTo(HaveOccurred())
						err = ioutil.WriteFile(path.Join(testingDir, ServerKeyName), []byte(certs2.Key), 0600)
						Expect(err).NotTo(HaveOccurred())
						close(done)
					})
					It("should keep the secret", func() {
						err := certWriter.EnsureCerts(mwc)
						Expect(err).NotTo(HaveOccurred())
						caBytes, err := ioutil.ReadFile(path.Join(testingDir, CACertName))
						Expect(err).NotTo(HaveOccurred())
						Expect(caBytes).To(Equal([]byte(certs2.CACert)))
						certBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerCertName))
						Expect(err).NotTo(HaveOccurred())
						Expect(certBytes).To(Equal([]byte(certs2.Cert)))
						keyBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerKeyName))
						Expect(err).NotTo(HaveOccurred())
						Expect(keyBytes).To(Equal([]byte(certs2.Key)))
					})
				})

				Context("when expiring", func() {
					// TODO: implement this.
					BeforeEach(func(done Done) {
						close(done)
					})

					It("should replace the expiring cert", func() {

					})
				})
			})
		})
	})
})
