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
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	admissionregistration "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/admission/cert/generator"
	fakegenerator "sigs.k8s.io/controller-runtime/pkg/admission/cert/generator/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("SecretCertWriter", func() {
	var mwc *admissionregistration.MutatingWebhookConfiguration
	var vwc *admissionregistration.ValidatingWebhookConfiguration
	var url string
	var cl client.Client

	BeforeEach(func(done Done) {
		cl = fake.NewFakeClient()
		annotations := map[string]string{
			"secret.certprovisioner.kubernetes.io/webhook-1": "namespace-bar/secret-foo",
		}
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
		close(done)
	})

	Describe("EnsureCerts", func() {
		var certWriter CertWriter
		var secretCertWriter *SecretCertWriter
		var secret *corev1.Secret
		var expectedSecret runtime.RawExtension

		Context("Failed to EnsureCerts", func() {
			BeforeEach(func(done Done) {
				certWriter = &SecretCertWriter{
					Client: cl,
				}
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
					Expect(err).To(MatchError(fmt.Errorf(
						"unsupported type: %T, only support v1beta1.MutatingWebhookConfiguration and v1beta1.ValidatingWebhookConfiguration",
						&corev1.Pod{})))
				})
			})

			Describe("can't find webhook name specified in the annotations", func() {
				BeforeEach(func(done Done) {
					mwc.Annotations["secret.certprovisioner.kubernetes.io/webhook-does-not-exist"] = "some-ns/some-secret"
					vwc.Annotations["secret.certprovisioner.kubernetes.io/webhook-does-not-exist"] = "some-ns/some-secret"
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
			BeforeEach(func(done Done) {
				secretCertWriter = &SecretCertWriter{
					Client: cl,
					CertGenerator: &fakegenerator.CertGenerator{
						DNSNameToCertArtifacts: map[string]*generator.Artifacts{
							"test-service.test-svc-namespace.svc": {
								CACert: []byte(`CACertBytes`),
								Cert:   []byte(`CertBytes`),
								Key:    []byte(`KeyBytes`),
							},
						},
					},
				}
				certWriter = secretCertWriter

				isController := true
				blockOwnerDeletion := true
				secret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace-bar",
						Name:      "secret-foo",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "admissionregistration.k8s.io/v1beta1",
								Kind:               "MutatingWebhookConfiguration",
								Name:               "test-mwc",
								UID:                "123456",
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

			Context("CertGenerator is not set", func() {
				BeforeEach(func(done Done) {
					secretCertWriter = &SecretCertWriter{Client: cl}
					certWriter = secretCertWriter
					close(done)
				})

				It("should default it and return no error", func() {
					err := certWriter.EnsureCerts(mwc)
					Expect(err).NotTo(HaveOccurred())
					list := &corev1.List{}
					err = cl.List(nil, &client.ListOptions{
						Namespace: "namespace-bar",
						Raw: &metav1.ListOptions{
							TypeMeta: metav1.TypeMeta{
								APIVersion: "v1",
								Kind:       "Secret",
							},
						},
					}, list)
					Expect(err).NotTo(HaveOccurred())
					Expect(list.Items).To(HaveLen(1))
				})
			})

			Context("no existing secret", func() {
				BeforeEach(func(done Done) {
					j, _ := json.Marshal(secret)
					expectedSecret = runtime.RawExtension{Raw: j}
					close(done)
				})

				It("should create new secrets with certs", func() {
					err := certWriter.EnsureCerts(mwc)
					Expect(err).NotTo(HaveOccurred())
					list := &corev1.List{}
					err = cl.List(nil, &client.ListOptions{
						Namespace: "namespace-bar",
						Raw: &metav1.ListOptions{
							TypeMeta: metav1.TypeMeta{
								APIVersion: "v1",
								Kind:       "Secret",
							},
						},
					}, list)
					Expect(err).NotTo(HaveOccurred())
					Expect(list.Items).To(ConsistOf(expectedSecret))
					Expect(list.Items).To(HaveLen(1))
				})
			})

			Context("old secret exists", func() {
				var oldSecret *corev1.Secret

				Context("cert is invalid", func() {
					BeforeEach(func(done Done) {
						j, _ := json.Marshal(secret)
						expectedSecret = runtime.RawExtension{Raw: j}
						close(done)
					})

					Describe("cert in secret is incomplete", func() {
						BeforeEach(func(done Done) {
							oldSecret = secret.DeepCopy()
							oldSecret.Data = nil
							cl = fake.NewFakeClient(oldSecret)
							secretCertWriter.Client = cl
							close(done)
						})

						It("should replace with new certs", func() {
							err := certWriter.EnsureCerts(mwc)
							Expect(err).NotTo(HaveOccurred())
							list := &corev1.List{}
							err = cl.List(nil, &client.ListOptions{
								Namespace: "namespace-bar",
								Raw: &metav1.ListOptions{
									TypeMeta: metav1.TypeMeta{
										APIVersion: "v1",
										Kind:       "Secret",
									},
								},
							}, list)
							Expect(err).NotTo(HaveOccurred())
							Expect(list.Items).To(ConsistOf(expectedSecret))
							Expect(list.Items).To(HaveLen(1))
						})
					})

					Describe("cert content is invalid", func() {
						BeforeEach(func(done Done) {
							oldSecret = secret.DeepCopy()
							oldSecret.Data = map[string][]byte{
								CACertName:     []byte(`oldCACertBytes`),
								ServerKeyName:  []byte(`oldKeyBytes`),
								ServerCertName: []byte(`oldCertBytes`),
							}
							cl = fake.NewFakeClient(oldSecret)
							secretCertWriter.Client = cl
							close(done)
						})

						It("should replace with new certs", func() {
							err := certWriter.EnsureCerts(mwc)
							Expect(err).NotTo(HaveOccurred())
							list := &corev1.List{}
							err = cl.List(nil, &client.ListOptions{
								Namespace: "namespace-bar",
								Raw: &metav1.ListOptions{
									TypeMeta: metav1.TypeMeta{
										APIVersion: "v1",
										Kind:       "Secret",
									},
								},
							}, list)
							Expect(err).NotTo(HaveOccurred())
							Expect(list.Items).To(ConsistOf(expectedSecret))
							Expect(list.Items).To(HaveLen(1))
						})
					})
				})

				Context("cert is valid", func() {
					BeforeEach(func(done Done) {
						oldSecret.Data = map[string][]byte{
							CACertName:     []byte(certs2.CACert),
							ServerKeyName:  []byte(certs2.Key),
							ServerCertName: []byte(certs2.Cert),
						}
						j, _ := json.Marshal(oldSecret)
						expectedSecret = runtime.RawExtension{Raw: j}
						cl = fake.NewFakeClient(oldSecret)
						secretCertWriter.Client = cl
						close(done)
					})

					Context("when not expiring", func() {
						BeforeEach(func(done Done) {
							oldSecret = secret.DeepCopy()
							oldSecret.Data = map[string][]byte{
								CACertName:     []byte(certs2.CACert),
								ServerKeyName:  []byte(certs2.Key),
								ServerCertName: []byte(certs2.Cert),
							}
							j, _ := json.Marshal(oldSecret)
							expectedSecret = runtime.RawExtension{Raw: j}

							cl = fake.NewFakeClient(oldSecret)
							secretCertWriter.Client = cl
							close(done)
						})
						It("should keep the secret", func() {
							err := certWriter.EnsureCerts(mwc)
							Expect(err).NotTo(HaveOccurred())
							list := &corev1.List{}
							err = cl.List(nil, &client.ListOptions{
								Namespace: "namespace-bar",
								Raw: &metav1.ListOptions{
									TypeMeta: metav1.TypeMeta{
										APIVersion: "v1",
										Kind:       "Secret",
									},
								},
							}, list)
							Expect(err).NotTo(HaveOccurred())
							Expect(list.Items).To(HaveLen(1))
							Expect(list.Items[0]).To(Equal(expectedSecret))
						})
					})

					Context("when expiring", func() {
						// TODO: implement this.
						BeforeEach(func(done Done) {
							oldSecret = secret.DeepCopy()
							oldSecret.Data = map[string][]byte{
								CACertName: []byte(`oldCACertBytes`),
								//ServerKeyName:  []byte(expiringKeyPEM),
								//ServerCertName: []byte(expiringCertPEM),
							}
							//j, _ := json.Marshal(someNewValidSecret)
							//expectedSecret = runtime.RawExtension{Raw: j}

							cl = fake.NewFakeClient(oldSecret)
							secretCertWriter.Client = cl
							close(done)
						})

						It("should replace the expiring cert", func() {

						})
					})
				})
			})
		})
	})
})
