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

package certwriter

import (
	//. "github.com/onsi/ginkgo"
	//. "github.com/onsi/gomega"

	"encoding/json"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	admissionregistration "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/admission/certgenerator"
	fakegenerator "sigs.k8s.io/controller-runtime/pkg/admission/certgenerator/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var certPEM = `-----BEGIN CERTIFICATE-----
MIICRzCCAfGgAwIBAgIJALMb7ecMIk3MMA0GCSqGSIb3DQEBCwUAMH4xCzAJBgNV
BAYTAkdCMQ8wDQYDVQQIDAZMb25kb24xDzANBgNVBAcMBkxvbmRvbjEYMBYGA1UE
CgwPR2xvYmFsIFNlY3VyaXR5MRYwFAYDVQQLDA1JVCBEZXBhcnRtZW50MRswGQYD
VQQDDBJ0ZXN0LWNlcnRpZmljYXRlLTAwIBcNMTcwNDI2MjMyNjUyWhgPMjExNzA0
MDIyMzI2NTJaMH4xCzAJBgNVBAYTAkdCMQ8wDQYDVQQIDAZMb25kb24xDzANBgNV
BAcMBkxvbmRvbjEYMBYGA1UECgwPR2xvYmFsIFNlY3VyaXR5MRYwFAYDVQQLDA1J
VCBEZXBhcnRtZW50MRswGQYDVQQDDBJ0ZXN0LWNlcnRpZmljYXRlLTAwXDANBgkq
hkiG9w0BAQEFAANLADBIAkEAtBMa7NWpv3BVlKTCPGO/LEsguKqWHBtKzweMY2CV
tAL1rQm913huhxF9w+ai76KQ3MHK5IVnLJjYYA5MzP2H5QIDAQABo1AwTjAdBgNV
HQ4EFgQU22iy8aWkNSxv0nBxFxerfsvnZVMwHwYDVR0jBBgwFoAU22iy8aWkNSxv
0nBxFxerfsvnZVMwDAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQsFAANBAEOefGbV
NcHxklaW06w6OBYJPwpIhCVozC1qdxGX1dg8VkEKzjOzjgqVD30m59OFmSlBmHsl
nkVA6wyOSDYBf3o=
-----END CERTIFICATE-----`

var keyPEM = `-----BEGIN RSA PRIVATE KEY-----
MIIBUwIBADANBgkqhkiG9w0BAQEFAASCAT0wggE5AgEAAkEAtBMa7NWpv3BVlKTC
PGO/LEsguKqWHBtKzweMY2CVtAL1rQm913huhxF9w+ai76KQ3MHK5IVnLJjYYA5M
zP2H5QIDAQABAkAS9BfXab3OKpK3bIgNNyp+DQJKrZnTJ4Q+OjsqkpXvNltPJosf
G8GsiKu/vAt4HGqI3eU77NvRI+mL4MnHRmXBAiEA3qM4FAtKSRBbcJzPxxLEUSwg
XSCcosCktbkXvpYrS30CIQDPDxgqlwDEJQ0uKuHkZI38/SPWWqfUmkecwlbpXABK
iQIgZX08DA8VfvcA5/Xj1Zjdey9FVY6POLXen6RPiabE97UCICp6eUW7ht+2jjar
e35EltCRCjoejRHTuN9TC0uCoVipAiAXaJIx/Q47vGwiw6Y8KXsNU6y54gTbOSxX
54LzHNk/+Q==
-----END RSA PRIVATE KEY-----`

var _ = Describe("SecretCertProvider", func() {
	var mwc *admissionregistration.MutatingWebhookConfiguration
	var vwc *admissionregistration.ValidatingWebhookConfiguration
	var url string
	var webhookMap map[string]*webhookAndSecret

	BeforeEach(func(done Done) {
		annotations := map[string]string{
			"secret.certprovisioner.kubernetes.io/webhook-1": "namespace-bar/secret-foo",
			"secret.certprovisioner.kubernetes.io/webhook-2": "default/secret-baz",
		}
		url = "https://example.com/admission"
		mwc = &admissionregistration.MutatingWebhookConfiguration{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "admissionregistration/v1beta1",
				Kind:       "MutatingWebhookConfiguration",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        "test-mwc",
				Namespace:   "test-ns",
				UID:         types.UID("123456"),
				Annotations: annotations,
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
		vwc = &admissionregistration.ValidatingWebhookConfiguration{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "admissionregistration/v1beta1",
				Kind:       "MutatingWebhookConfiguration",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        "test-vwc",
				Namespace:   "test-ns",
				UID:         types.UID("123457"),
				Annotations: annotations,
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
		webhookMap = map[string]*webhookAndSecret{
			"webhook-1": {
				webhook: &admissionregistration.Webhook{
					Name: "webhook-1",
					ClientConfig: admissionregistration.WebhookClientConfig{
						Service: &admissionregistration.ServiceReference{
							Namespace: "test-svc-namespace",
							Name:      "test-service",
						},
						CABundle: []byte(``),
					},
				},
				secret: types.NamespacedName{
					Namespace: "namespace-bar",
					Name:      "secret-foo",
				},
			},
			"webhook-2": {
				webhook: &admissionregistration.Webhook{
					Name: "webhook-2",
					ClientConfig: admissionregistration.WebhookClientConfig{
						URL:      &url,
						CABundle: []byte(``),
					},
				},
				secret: types.NamespacedName{
					Namespace: "default",
					Name:      "secret-baz",
				},
			},
		}
		close(done)
	})

	Describe("Provide", func() {
		var provider SecretCertWriterProvider
		var expectedMutatingWriter CertWriter
		var expectedValidatingWriter CertWriter

		BeforeEach(func(done Done) {
			client := fake.NewFakeClient()
			provider = SecretCertWriterProvider{
				Client: client,
			}

			expectedMutatingWriter = &secretCertWriter{
				client:        client,
				certGenerator: &certgenerator.SelfSignedCertGenerator{},
				webhookConfig: mwc,
				webhookMap:    webhookMap,
			}
			expectedValidatingWriter = &secretCertWriter{
				client:        client,
				certGenerator: &certgenerator.SelfSignedCertGenerator{},
				webhookConfig: vwc,
				webhookMap:    webhookMap,
			}
			close(done)
		})

		Context("Failed to Provide", func() {
			Describe("nil object", func() {
				It("should return error", func() {
					_, err := provider.Provide(nil)
					Expect(err).To(MatchError("unexpected nil webhook configuration object"))
				})
			})

			Describe("non-webhook configuration object", func() {
				It("should return error", func() {
					_, err := provider.Provide(&corev1.Pod{})
					Expect(err).To(MatchError(fmt.Errorf("unsupported type: %T, only support v1beta1.MutatingWebhookConfiguration and v1beta1.ValidatingWebhookConfiguration", &corev1.Pod{})))
				})
			})

			Describe("can't find webhook name specified in the annotations", func() {
				BeforeEach(func(done Done) {
					mwc.Annotations["secret.certprovisioner.kubernetes.io/webhook-does-not-exist"] = "some-ns/some-secret"
					close(done)
				})
				It("should return error", func() {
					_, err := provider.Provide(mwc)
					Expect(err).To(MatchError(fmt.Errorf("expecting a webhook named %q", "webhook-does-not-exist")))
				})
			})
		})

		Context("Succeeded to Provide w/o defaulting", func() {
			BeforeEach(func(done Done) {
				provider.CertGenerator = &certgenerator.SelfSignedCertGenerator{}
				close(done)
			})

			It("should be able to provide a CertWriter for MutatingWebhookConfiguration", func() {
				writer, err := provider.Provide(mwc)
				Expect(err).NotTo(HaveOccurred())
				Expect(writer).NotTo(BeNil())
				Expect(writer).To(Equal(expectedMutatingWriter))
			})

			It("should be able to provide a CertWriter for ValidatingWebhookConfiguration", func() {
				writer, err := provider.Provide(vwc)
				Expect(err).NotTo(HaveOccurred())
				Expect(writer).NotTo(BeNil())
				Expect(writer).To(Equal(expectedValidatingWriter))
			})
		})

		Context("Succeeded to Provide w/ defaulting", func() {
			It("should default the CertGenerator", func() {
				writer, err := provider.Provide(mwc)
				Expect(err).NotTo(HaveOccurred())
				Expect(writer).NotTo(BeNil())
				Expect(writer).To(Equal(expectedMutatingWriter))
			})
		})
	})

	Describe("EnsureCert", func() {
		var writer *secretCertWriter
		var secret *corev1.Secret
		var expectedSecret runtime.RawExtension

		BeforeEach(func(done Done) {
			// TODO: this is overriding the existing one
			webhookMap = map[string]*webhookAndSecret{
				"webhook-1": {
					webhook: &admissionregistration.Webhook{
						Name: "webhook-1",
						ClientConfig: admissionregistration.WebhookClientConfig{
							Service: &admissionregistration.ServiceReference{
								Namespace: "test-svc-namespace",
								Name:      "test-service",
							},
							CABundle: []byte(``),
						},
					},
					secret: types.NamespacedName{
						Namespace: "namespace-bar",
						Name:      "secret-foo",
					},
				},
			}

			writer = &secretCertWriter{
				certGenerator: &fakegenerator.FakeCertGenerator{
					DnsNameToCertArtifacts: map[string]*certgenerator.CertArtifacts{
						"test-service.test-svc-namespace.svc": {
							CACert: []byte(`CACertBytes`),
							Cert:   []byte(`CertBytes`),
							Key:    []byte(`KeyBytes`),
						},
					},
				},
				webhookConfig: mwc,
				webhookMap:    webhookMap,
			}

			isController := true
			blockOwnerDeletion := false
			secret = &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace-bar",
					Name:      "secret-foo",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "admissionregistration/v1beta1",
							Kind:               "MutatingWebhookConfiguration",
							Name:               "test-mwc",
							UID:                types.UID("123456"),
							BlockOwnerDeletion: &blockOwnerDeletion,
							Controller:         &isController,
						},
					},
				},
				Data: map[string][]byte{
					CACertName:     []byte(`CACertBytes`),
					ServerKeyName:  []byte(`KeyBytes`),
					ServerCertName: []byte(`CertBytes`),
				},
			}

			close(done)
		})

		Context("no existing secret", func() {
			BeforeEach(func(done Done) {
				j, _ := json.Marshal(secret)
				expectedSecret = runtime.RawExtension{Raw: j}
				writer.client = fake.NewFakeClient()
				close(done)
			})

			Describe("ClientConfig is invalid", func() {
				BeforeEach(func(done Done) {
					url := "https://foo.bar.com/someurl"
					writer.webhookMap["webhook-1"].webhook.ClientConfig.URL = &url
					close(done)
				})
				It("should return an error", func() {
					err := writer.EnsureCert()
					Expect(err).To(MatchError(fmt.Errorf("service and URL can't be set at the same time in a webhook: %v",
						&writer.webhookMap["webhook-1"].webhook.ClientConfig)))
				})
			})

			It("should create new secrets with certs", func() {
				err := writer.EnsureCert()
				Expect(err).NotTo(HaveOccurred())
				list := &corev1.List{}
				err = writer.client.List(nil, &client.ListOptions{
					Namespace: "namespace-bar",
					Raw: &metav1.ListOptions{
						TypeMeta: metav1.TypeMeta{
							APIVersion: "v1",
							Kind:       "Secret",
						},
					},
				}, list)
				Expect(err).NotTo(HaveOccurred())
				// This complains about type mismatch somehow: list.Items in runtime.RawExtension, but expectedSecret is interface[]
				//Expect(list.Items).To(ConsistOf(expectedSecret))
				Expect(list.Items).To(HaveLen(1))
				Expect(list.Items[0]).To(Equal(expectedSecret))
			})
		})

		Context("old secret exists", func() {
			var oldSecret *corev1.Secret
			BeforeEach(func(done Done) {
				oldSecret = secret.DeepCopy()
				oldSecret.Data = map[string][]byte{
					CACertName:     []byte(`oldCACertBytes`),
					ServerKeyName:  []byte(`oldKeyBytes`),
					ServerCertName: []byte(`oldCertBytes`),
				}
				close(done)
			})

			Describe("ClientConfig is invalid", func() {
				BeforeEach(func(done Done) {
					url := "https://foo.bar.com/someurl"
					writer.webhookMap["webhook-1"].webhook.ClientConfig.URL = &url
					writer.client = fake.NewFakeClient(oldSecret)
					close(done)
				})
				It("should return an error", func() {
					err := writer.EnsureCert()
					Expect(err).To(MatchError(fmt.Errorf("service and URL can't be set at the same time in a webhook: %v",
						&writer.webhookMap["webhook-1"].webhook.ClientConfig)))
				})
			})

			Context("cert is valid", func() {
				BeforeEach(func(done Done) {
					oldSecret.Data = map[string][]byte{
						CACertName:     []byte(`oldCACertBytes`),
						ServerKeyName:  []byte(keyPEM),
						ServerCertName: []byte(certPEM),
					}
					j, _ := json.Marshal(oldSecret)
					expectedSecret = runtime.RawExtension{Raw: j}
					writer.client = fake.NewFakeClient(oldSecret)
					close(done)
				})

				It("should keep the secret", func() {
					err := writer.EnsureCert()
					Expect(err).NotTo(HaveOccurred())
					list := &corev1.List{}
					err = writer.client.List(nil, &client.ListOptions{
						Namespace: "namespace-bar",
						Raw: &metav1.ListOptions{
							TypeMeta: metav1.TypeMeta{
								APIVersion: "v1",
								Kind:       "Secret",
							},
						},
					}, list)
					Expect(err).NotTo(HaveOccurred())
					// This complains about type mismatch somehow: list.Items in runtime.RawExtension, but expectedSecret is interface[]
					//Expect(list.Items).To(ConsistOf(expectedSecret))
					Expect(list.Items).To(HaveLen(1))
					Expect(list.Items[0]).To(Equal(expectedSecret))
				})
			})

			Context("cert is invalid", func() {
				BeforeEach(func(done Done) {
					writer.client = fake.NewFakeClient(oldSecret)
					j, _ := json.Marshal(secret)
					expectedSecret = runtime.RawExtension{Raw: j}
					close(done)
				})

				Describe("cert in secret is incomplete", func() {
					BeforeEach(func() {
						oldSecret.Data = nil
						writer.client = fake.NewFakeClient(oldSecret)
					})
					It("should replace with new certs", func() {
						err := writer.EnsureCert()
						Expect(err).NotTo(HaveOccurred())
						list := &corev1.List{}
						err = writer.client.List(nil, &client.ListOptions{
							Namespace: "namespace-bar",
							Raw: &metav1.ListOptions{
								TypeMeta: metav1.TypeMeta{
									APIVersion: "v1",
									Kind:       "Secret",
								},
							},
						}, list)
						Expect(err).NotTo(HaveOccurred())
						// This complains about type mismatch somehow: list.Items in runtime.RawExtension, but expectedSecret is interface[]
						//Expect(list.Items).To(ConsistOf(expectedSecret))
						Expect(list.Items).To(HaveLen(1))
						Expect(list.Items[0]).To(Equal(expectedSecret))
					})
				})

				Describe("cert content is invalid", func() {
					It("should replace with new certs", func() {
						err := writer.EnsureCert()
						Expect(err).NotTo(HaveOccurred())
						list := &corev1.List{}
						err = writer.client.List(nil, &client.ListOptions{
							Namespace: "namespace-bar",
							Raw: &metav1.ListOptions{
								TypeMeta: metav1.TypeMeta{
									APIVersion: "v1",
									Kind:       "Secret",
								},
							},
						}, list)
						Expect(err).NotTo(HaveOccurred())
						// This complains about type mismatch somehow: list.Items in runtime.RawExtension, but expectedSecret is interface[]
						//Expect(list.Items).To(ConsistOf(expectedSecret))
						Expect(list.Items).To(HaveLen(1))
						Expect(list.Items[0]).To(Equal(expectedSecret))
					})
				})

			})

			Context("cert is valid, but expiring", func() {
				// TODO: implement this.
				BeforeEach(func(done Done) {
					writer.client = fake.NewFakeClient()
					close(done)
				})

				It("should replace the expiring cert", func() {

				})
			})
		})
	})
})
