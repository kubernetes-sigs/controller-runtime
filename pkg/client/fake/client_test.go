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

package fake

import (
	"encoding/json"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Fake client", func() {
	var dep *appsv1.Deployment
	var dep2 *appsv1.Deployment
	var cm *corev1.ConfigMap
	var cl client.Client

	BeforeEach(func() {
		dep = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deployment",
				Namespace: "ns1",
			},
		}
		dep2 = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deployment-2",
				Namespace: "ns1",
				Labels: map[string]string{
					"test-label": "label-value",
				},
			},
		}
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cm",
				Namespace: "ns2",
			},
			Data: map[string]string{
				"test-key": "test-value",
			},
		}
	})

	AssertClientBehavior := func() {
		It("should be able to Get", func() {
			By("Getting a deployment")
			namespacedName := types.NamespacedName{
				Name:      "test-deployment",
				Namespace: "ns1",
			}
			obj := &appsv1.Deployment{}
			err := cl.Get(nil, namespacedName, obj)
			Expect(err).To(BeNil())
			Expect(obj).To(Equal(dep))
		})

		It("should be able to List", func() {
			By("Listing all deployments in a namespace")
			list := &appsv1.DeploymentList{}
			err := cl.List(nil, list, client.InNamespace("ns1"))
			Expect(err).To(BeNil())
			Expect(list.Items).To(HaveLen(2))
			Expect(list.Items).To(ConsistOf(*dep, *dep2))
		})

		It("should support filtering by labels", func() {
			By("Listing deployments with a particular label")
			list := &appsv1.DeploymentList{}
			err := cl.List(nil, list, client.InNamespace("ns1"),
				client.MatchingLabels(map[string]string{
					"test-label": "label-value",
				}))
			Expect(err).To(BeNil())
			Expect(list.Items).To(HaveLen(1))
			Expect(list.Items).To(ConsistOf(*dep2))
		})

		It("should be able to Create", func() {
			By("Creating a new configmap")
			newcm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-test-cm",
					Namespace: "ns2",
				},
			}
			err := cl.Create(nil, newcm)
			Expect(err).To(BeNil())

			By("Getting the new configmap")
			namespacedName := types.NamespacedName{
				Name:      "new-test-cm",
				Namespace: "ns2",
			}
			obj := &corev1.ConfigMap{}
			err = cl.Get(nil, namespacedName, obj)
			Expect(err).To(BeNil())
			Expect(obj).To(Equal(newcm))
		})

		It("should be able to Update", func() {
			By("Updating a new configmap")
			newcm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm",
					Namespace: "ns2",
				},
				Data: map[string]string{
					"test-key": "new-value",
				},
			}
			err := cl.Update(nil, newcm)
			Expect(err).To(BeNil())

			By("Getting the new configmap")
			namespacedName := types.NamespacedName{
				Name:      "test-cm",
				Namespace: "ns2",
			}
			obj := &corev1.ConfigMap{}
			err = cl.Get(nil, namespacedName, obj)
			Expect(err).To(BeNil())
			Expect(obj).To(Equal(newcm))
		})

		It("should be able to Delete", func() {
			By("Deleting a deployment")
			err := cl.Delete(nil, dep)
			Expect(err).To(BeNil())

			By("Listing all deployments in the namespace")
			list := &appsv1.DeploymentList{}
			err = cl.List(nil, list, client.InNamespace("ns1"))
			Expect(err).To(BeNil())
			Expect(list.Items).To(HaveLen(1))
			Expect(list.Items).To(ConsistOf(*dep2))
		})

		Context("with the DryRun option", func() {
			It("should not create a new object", func() {
				By("Creating a new configmap with DryRun")
				newcm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "new-test-cm",
						Namespace: "ns2",
					},
				}
				err := cl.Create(nil, newcm, client.CreateDryRunAll)
				Expect(err).To(BeNil())

				By("Getting the new configmap")
				namespacedName := types.NamespacedName{
					Name:      "new-test-cm",
					Namespace: "ns2",
				}
				obj := &corev1.ConfigMap{}
				err = cl.Get(nil, namespacedName, obj)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsNotFound(err)).To(BeTrue())
				Expect(obj).NotTo(Equal(newcm))
			})

			It("should not Update the object", func() {
				By("Updating a new configmap with DryRun")
				newcm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cm",
						Namespace: "ns2",
					},
					Data: map[string]string{
						"test-key": "new-value",
					},
				}
				err := cl.Update(nil, newcm, client.UpdateDryRunAll)
				Expect(err).To(BeNil())

				By("Getting the new configmap")
				namespacedName := types.NamespacedName{
					Name:      "test-cm",
					Namespace: "ns2",
				}
				obj := &corev1.ConfigMap{}
				err = cl.Get(nil, namespacedName, obj)
				Expect(err).To(BeNil())
				Expect(obj).To(Equal(cm))
			})
		})

		It("should be able to Patch", func() {
			By("Patching a deployment")
			mergePatch, err := json.Marshal(map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						"foo": "bar",
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			err = cl.Patch(nil, dep, client.ConstantPatch(types.StrategicMergePatchType, mergePatch))
			Expect(err).NotTo(HaveOccurred())

			By("Getting the patched deployment")
			namespacedName := types.NamespacedName{
				Name:      "test-deployment",
				Namespace: "ns1",
			}
			obj := &appsv1.Deployment{}
			err = cl.Get(nil, namespacedName, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Annotations["foo"]).To(Equal("bar"))
		})
	}

	Context("with default scheme.Scheme", func() {
		BeforeEach(func(done Done) {
			cl = NewFakeClient(dep, dep2, cm)
			close(done)
		})
		AssertClientBehavior()
	})

	Context("with given scheme", func() {
		BeforeEach(func(done Done) {
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			Expect(appsv1.AddToScheme(scheme)).To(Succeed())
			cl = NewFakeClientWithScheme(scheme, dep, dep2, cm)
			close(done)
		})
		AssertClientBehavior()
	})
})
