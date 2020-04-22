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
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
			TypeMeta: metav1.TypeMeta{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deployment",
				Namespace: "ns1",
			},
		}
		dep2 = &appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deployment-2",
				Namespace: "ns1",
				Labels: map[string]string{
					"test-label": "label-value",
				},
			},
		}
		cm = &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "ConfigMap",
			},
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
			err := cl.Get(context.Background(), namespacedName, obj)
			Expect(err).To(BeNil())
			Expect(obj).To(Equal(dep))
		})

		It("should be able to Get using unstructured", func() {
			By("Getting a deployment")
			namespacedName := types.NamespacedName{
				Name:      "test-deployment",
				Namespace: "ns1",
			}
			obj := &unstructured.Unstructured{}
			obj.SetAPIVersion("apps/v1")
			obj.SetKind("Deployment")
			err := cl.Get(context.Background(), namespacedName, obj)
			Expect(err).To(BeNil())
		})

		It("should be able to List", func() {
			By("Listing all deployments in a namespace")
			list := &appsv1.DeploymentList{}
			err := cl.List(context.Background(), list, client.InNamespace("ns1"))
			Expect(err).To(BeNil())
			Expect(list.Items).To(HaveLen(2))
			Expect(list.Items).To(ConsistOf(*dep, *dep2))
		})

		It("should be able to List using unstructured list", func() {
			By("Listing all deployments in a namespace")
			list := &unstructured.UnstructuredList{}
			list.SetAPIVersion("apps/v1")
			list.SetKind("DeploymentList")
			err := cl.List(context.Background(), list, client.InNamespace("ns1"))
			Expect(err).To(BeNil())
			Expect(list.Items).To(HaveLen(2))
		})

		It("should support filtering by labels and their values", func() {
			By("Listing deployments with a particular label and value")
			list := &appsv1.DeploymentList{}
			err := cl.List(context.Background(), list, client.InNamespace("ns1"),
				client.MatchingLabels(map[string]string{
					"test-label": "label-value",
				}))
			Expect(err).To(BeNil())
			Expect(list.Items).To(HaveLen(1))
			Expect(list.Items).To(ConsistOf(*dep2))
		})

		It("should support filtering by label existence", func() {
			By("Listing deployments with a particular label")
			list := &appsv1.DeploymentList{}
			err := cl.List(nil, list, client.InNamespace("ns1"),
				client.HasLabels{"test-label"})
			Expect(err).To(BeNil())
			Expect(list.Items).To(HaveLen(1))
			Expect(list.Items).To(ConsistOf(*dep2))
		})

		It("should be able to Create", func() {
			By("Creating a new configmap")
			newcm := &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-test-cm",
					Namespace: "ns2",
				},
			}
			err := cl.Create(context.Background(), newcm)
			Expect(err).To(BeNil())

			By("Getting the new configmap")
			namespacedName := types.NamespacedName{
				Name:      "new-test-cm",
				Namespace: "ns2",
			}
			obj := &corev1.ConfigMap{}
			err = cl.Get(context.Background(), namespacedName, obj)
			Expect(err).To(BeNil())
			Expect(obj).To(Equal(newcm))
			Expect(obj.ObjectMeta.ResourceVersion).To(Equal("1"))
		})

		It("should error on create with set resourceVersion", func() {
			By("Creating a new configmap")
			newcm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "new-test-cm",
					Namespace:       "ns2",
					ResourceVersion: "1",
				},
			}
			err := cl.Create(context.Background(), newcm)
			Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		It("should not change the submitted object if Create failed", func() {
			By("Trying to create an existing configmap")
			submitted := cm.DeepCopy()
			err := cl.Create(context.Background(), submitted)
			Expect(apierrors.IsAlreadyExists(err)).To(BeTrue())
			Expect(submitted).To(Equal(cm))
		})

		It("should error on Create with empty Name", func() {
			By("Creating a new configmap")
			newcm := &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns2",
				},
			}
			err := cl.Create(context.Background(), newcm)
			Expect(err.Error()).To(Equal("ConfigMap \"\" is invalid: metadata.name: Required value: name is required"))
		})

		It("should error on Update with empty Name", func() {
			By("Creating a new configmap")
			newcm := &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns2",
				},
			}
			err := cl.Update(context.Background(), newcm)
			Expect(err.Error()).To(Equal("ConfigMap \"\" is invalid: metadata.name: Required value: name is required"))
		})

		It("should be able to Create with GenerateName", func() {
			By("Creating a new configmap")
			newcm := &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "new-test-cm",
					Namespace:    "ns2",
					Labels: map[string]string{
						"test-label": "label-value",
					},
				},
			}
			err := cl.Create(nil, newcm)
			Expect(err).To(BeNil())

			By("Listing configmaps with a particular label")
			list := &corev1.ConfigMapList{}
			err = cl.List(nil, list, client.InNamespace("ns2"),
				client.MatchingLabels(map[string]string{
					"test-label": "label-value",
				}))
			Expect(err).To(BeNil())
			Expect(list.Items).To(HaveLen(1))
			Expect(list.Items[0].Name).NotTo(BeEmpty())
		})

		It("should be able to Update", func() {
			By("Updating a new configmap")
			newcm := &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-cm",
					Namespace:       "ns2",
					ResourceVersion: "",
				},
				Data: map[string]string{
					"test-key": "new-value",
				},
			}
			err := cl.Update(context.Background(), newcm)
			Expect(err).To(BeNil())

			By("Getting the new configmap")
			namespacedName := types.NamespacedName{
				Name:      "test-cm",
				Namespace: "ns2",
			}
			obj := &corev1.ConfigMap{}
			err = cl.Get(context.Background(), namespacedName, obj)
			Expect(err).To(BeNil())
			Expect(obj).To(Equal(newcm))
			Expect(obj.ObjectMeta.ResourceVersion).To(Equal("1"))
		})

		It("should reject updates with non-matching ResourceVersion", func() {
			By("Updating a new configmap")
			newcm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-cm",
					Namespace:       "ns2",
					ResourceVersion: "1",
				},
				Data: map[string]string{
					"test-key": "new-value",
				},
			}
			err := cl.Update(context.Background(), newcm)
			Expect(apierrors.IsConflict(err)).To(BeTrue())

			By("Getting the configmap")
			namespacedName := types.NamespacedName{
				Name:      "test-cm",
				Namespace: "ns2",
			}
			obj := &corev1.ConfigMap{}
			err = cl.Get(context.Background(), namespacedName, obj)
			Expect(err).To(BeNil())
			Expect(obj).To(Equal(cm))
			Expect(obj.ObjectMeta.ResourceVersion).To(Equal(""))
		})

		It("should be able to Delete", func() {
			By("Deleting a deployment")
			err := cl.Delete(context.Background(), dep)
			Expect(err).To(BeNil())

			By("Listing all deployments in the namespace")
			list := &appsv1.DeploymentList{}
			err = cl.List(context.Background(), list, client.InNamespace("ns1"))
			Expect(err).To(BeNil())
			Expect(list.Items).To(HaveLen(1))
			Expect(list.Items).To(ConsistOf(*dep2))
		})

		It("should be able to Delete a Collection", func() {
			By("Deleting a deploymentList")
			err := cl.DeleteAllOf(context.Background(), &appsv1.Deployment{}, client.InNamespace("ns1"))
			Expect(err).To(BeNil())

			By("Listing all deployments in the namespace")
			list := &appsv1.DeploymentList{}
			err = cl.List(context.Background(), list, client.InNamespace("ns1"))
			Expect(err).To(BeNil())
			Expect(list.Items).To(BeEmpty())
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
				err := cl.Create(context.Background(), newcm, client.DryRunAll)
				Expect(err).To(BeNil())

				By("Getting the new configmap")
				namespacedName := types.NamespacedName{
					Name:      "new-test-cm",
					Namespace: "ns2",
				}
				obj := &corev1.ConfigMap{}
				err = cl.Get(context.Background(), namespacedName, obj)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsNotFound(err)).To(BeTrue())
				Expect(obj).NotTo(Equal(newcm))
			})

			It("should not Update the object", func() {
				By("Updating a new configmap with DryRun")
				newcm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "test-cm",
						Namespace:       "ns2",
						ResourceVersion: "1",
					},
					Data: map[string]string{
						"test-key": "new-value",
					},
				}
				err := cl.Update(context.Background(), newcm, client.DryRunAll)
				Expect(err).To(BeNil())

				By("Getting the new configmap")
				namespacedName := types.NamespacedName{
					Name:      "test-cm",
					Namespace: "ns2",
				}
				obj := &corev1.ConfigMap{}
				err = cl.Get(context.Background(), namespacedName, obj)
				Expect(err).To(BeNil())
				Expect(obj).To(Equal(cm))
				Expect(obj.ObjectMeta.ResourceVersion).To(Equal(""))
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
			err = cl.Patch(context.Background(), dep, client.RawPatch(types.StrategicMergePatchType, mergePatch))
			Expect(err).NotTo(HaveOccurred())

			By("Getting the patched deployment")
			namespacedName := types.NamespacedName{
				Name:      "test-deployment",
				Namespace: "ns1",
			}
			obj := &appsv1.Deployment{}
			err = cl.Get(context.Background(), namespacedName, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(obj.Annotations["foo"]).To(Equal("bar"))
			Expect(obj.ObjectMeta.ResourceVersion).To(Equal("1"))
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
