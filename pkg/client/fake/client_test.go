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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Fake client", func() {
	var dep *appsv1.Deployment
	var dep2 *appsv1.Deployment
	var cm *corev1.ConfigMap
	var cl client.WithWatch

	BeforeEach(func() {
		dep = &appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            "test-deployment",
				Namespace:       "ns1",
				ResourceVersion: trackerAddResourceVersion,
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
				ResourceVersion: trackerAddResourceVersion,
			},
		}
		cm = &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "ConfigMap",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            "test-cm",
				Namespace:       "ns2",
				ResourceVersion: trackerAddResourceVersion,
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

		It("should be able to List using unstructured list when setting a non-list kind", func() {
			By("Listing all deployments in a namespace")
			list := &unstructured.UnstructuredList{}
			list.SetAPIVersion("apps/v1")
			list.SetKind("Deployment")
			err := cl.List(context.Background(), list, client.InNamespace("ns1"))
			Expect(err).To(BeNil())
			Expect(list.Items).To(HaveLen(2))
		})

		It("should be able to Create an unregistered type using unstructured", func() {
			item := &unstructured.Unstructured{}
			item.SetAPIVersion("custom/v1")
			item.SetKind("Image")
			item.SetName("my-item")
			err := cl.Create(context.Background(), item)
			Expect(err).To(BeNil())
		})

		It("should be able to Get an unregisted type using unstructured", func() {
			By("Creating an object of an unregistered type")
			item := &unstructured.Unstructured{}
			item.SetAPIVersion("custom/v2")
			item.SetKind("Image")
			item.SetName("my-item")
			err := cl.Create(context.Background(), item)
			Expect(err).To(BeNil())

			By("Getting and the object")
			item = &unstructured.Unstructured{}
			item.SetAPIVersion("custom/v2")
			item.SetKind("Image")
			item.SetName("my-item")
			err = cl.Get(context.Background(), client.ObjectKeyFromObject(item), item)
			Expect(err).To(BeNil())
		})

		It("should be able to List an unregistered type using unstructured", func() {
			list := &unstructured.UnstructuredList{}
			list.SetAPIVersion("custom/v3")
			list.SetKind("ImageList")
			err := cl.List(context.Background(), list)
			Expect(err).To(BeNil())
		})

		It("should be able to List an unregistered type using unstructured", func() {
			list := &unstructured.UnstructuredList{}
			list.SetAPIVersion("custom/v4")
			list.SetKind("Image")
			err := cl.List(context.Background(), list)
			Expect(err).To(BeNil())
		})

		It("should be able to Update an unregistered type using unstructured", func() {
			By("Creating an object of an unregistered type")
			item := &unstructured.Unstructured{}
			item.SetAPIVersion("custom/v5")
			item.SetKind("Image")
			item.SetName("my-item")
			err := cl.Create(context.Background(), item)
			Expect(err).To(BeNil())

			By("Updating the object")
			err = unstructured.SetNestedField(item.Object, int64(2), "spec", "replicas")
			Expect(err).To(BeNil())
			err = cl.Update(context.Background(), item)
			Expect(err).To(BeNil())

			By("Getting the object")
			item = &unstructured.Unstructured{}
			item.SetAPIVersion("custom/v5")
			item.SetKind("Image")
			item.SetName("my-item")
			err = cl.Get(context.Background(), client.ObjectKeyFromObject(item), item)
			Expect(err).To(BeNil())

			By("Inspecting the object")
			value, found, err := unstructured.NestedInt64(item.Object, "spec", "replicas")
			Expect(err).To(BeNil())
			Expect(found).To(BeTrue())
			Expect(value).To(Equal(int64(2)))
		})

		It("should be able to Patch an unregistered type using unstructured", func() {
			By("Creating an object of an unregistered type")
			item := &unstructured.Unstructured{}
			item.SetAPIVersion("custom/v6")
			item.SetKind("Image")
			item.SetName("my-item")
			err := cl.Create(context.Background(), item)
			Expect(err).To(BeNil())

			By("Updating the object")
			original := item.DeepCopy()
			err = unstructured.SetNestedField(item.Object, int64(2), "spec", "replicas")
			Expect(err).To(BeNil())
			err = cl.Patch(context.Background(), item, client.MergeFrom(original))
			Expect(err).To(BeNil())

			By("Getting the object")
			item = &unstructured.Unstructured{}
			item.SetAPIVersion("custom/v6")
			item.SetKind("Image")
			item.SetName("my-item")
			err = cl.Get(context.Background(), client.ObjectKeyFromObject(item), item)
			Expect(err).To(BeNil())

			By("Inspecting the object")
			value, found, err := unstructured.NestedInt64(item.Object, "spec", "replicas")
			Expect(err).To(BeNil())
			Expect(found).To(BeTrue())
			Expect(value).To(Equal(int64(2)))
		})

		It("should be able to Delete an unregistered type using unstructured", func() {
			By("Creating an object of an unregistered type")
			item := &unstructured.Unstructured{}
			item.SetAPIVersion("custom/v7")
			item.SetKind("Image")
			item.SetName("my-item")
			err := cl.Create(context.Background(), item)
			Expect(err).To(BeNil())

			By("Deleting the object")
			err = cl.Delete(context.Background(), item)
			Expect(err).To(BeNil())

			By("Getting the object")
			item = &unstructured.Unstructured{}
			item.SetAPIVersion("custom/v7")
			item.SetKind("Image")
			item.SetName("my-item")
			err = cl.Get(context.Background(), client.ObjectKeyFromObject(item), item)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
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
			err := cl.List(context.Background(), list, client.InNamespace("ns1"),
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
			submitted.ResourceVersion = ""
			submittedReference := submitted.DeepCopy()
			err := cl.Create(context.Background(), submitted)
			Expect(err).ToNot(BeNil())
			Expect(apierrors.IsAlreadyExists(err)).To(BeTrue())
			Expect(submitted).To(Equal(submittedReference))
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
			err := cl.Create(context.Background(), newcm)
			Expect(err).To(BeNil())

			By("Listing configmaps with a particular label")
			list := &corev1.ConfigMapList{}
			err = cl.List(context.Background(), list, client.InNamespace("ns2"),
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
			Expect(obj.ObjectMeta.ResourceVersion).To(Equal("1000"))
		})

		It("should allow updates with non-set ResourceVersion for a resource that allows unconditional updates", func() {
			By("Updating a new configmap")
			newcm := &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm",
					Namespace: "ns2",
				},
				Data: map[string]string{
					"test-key": "new-value",
				},
			}
			err := cl.Update(context.Background(), newcm)
			Expect(err).To(BeNil())

			By("Getting the configmap")
			namespacedName := types.NamespacedName{
				Name:      "test-cm",
				Namespace: "ns2",
			}
			obj := &corev1.ConfigMap{}
			err = cl.Get(context.Background(), namespacedName, obj)
			Expect(err).To(BeNil())
			Expect(obj).To(Equal(newcm))
			Expect(obj.ObjectMeta.ResourceVersion).To(Equal("1000"))
		})

		It("should reject updates with non-set ResourceVersion for a resource that doesn't allow unconditional updates", func() {
			By("Creating a new binding")
			binding := &corev1.Binding{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Binding",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-binding",
					Namespace: "ns2",
				},
				Target: corev1.ObjectReference{
					Kind:       "ConfigMap",
					APIVersion: "v1",
					Namespace:  cm.Namespace,
					Name:       cm.Name,
				},
			}
			Expect(cl.Create(context.Background(), binding)).To(Succeed())

			By("Updating the binding with a new resource lacking resource version")
			newBinding := &corev1.Binding{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Binding",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      binding.Name,
					Namespace: binding.Namespace,
				},
				Target: corev1.ObjectReference{
					Namespace: binding.Namespace,
					Name:      "blue",
				},
			}
			Expect(cl.Update(context.Background(), newBinding)).NotTo(Succeed())
		})

		It("should allow create on update for a resource that allows create on update", func() {
			By("Creating a new lease with update")
			lease := &coordinationv1.Lease{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "coordination.k8s.io/v1",
					Kind:       "Lease",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-lease",
					Namespace: "ns2",
				},
				Spec: coordinationv1.LeaseSpec{},
			}
			Expect(cl.Create(context.Background(), lease)).To(Succeed())

			By("Getting the lease")
			namespacedName := types.NamespacedName{
				Name:      lease.Name,
				Namespace: lease.Namespace,
			}
			obj := &coordinationv1.Lease{}
			Expect(cl.Get(context.Background(), namespacedName, obj)).To(Succeed())
			Expect(obj).To(Equal(lease))
			Expect(obj.ObjectMeta.ResourceVersion).To(Equal("1"))
		})

		It("should reject create on update for a resource that does not allow create on update", func() {
			By("Attemping to create a new configmap with update")
			newcm := &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "different-test-cm",
					Namespace: "ns2",
				},
				Data: map[string]string{
					"test-key": "new-value",
				},
			}
			Expect(cl.Update(context.Background(), newcm)).NotTo(Succeed())
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
			Expect(obj.ObjectMeta.ResourceVersion).To(Equal(trackerAddResourceVersion))
		})

		It("should reject Delete with a mismatched ResourceVersion", func() {
			bogusRV := "bogus"
			By("Deleting with a mismatched ResourceVersion Precondition")
			err := cl.Delete(context.Background(), dep, client.Preconditions{ResourceVersion: &bogusRV})
			Expect(apierrors.IsConflict(err)).To(BeTrue())

			list := &appsv1.DeploymentList{}
			err = cl.List(context.Background(), list, client.InNamespace("ns1"))
			Expect(err).To(BeNil())
			Expect(list.Items).To(HaveLen(2))
			Expect(list.Items).To(ConsistOf(*dep, *dep2))
		})

		It("should successfully Delete with a matching ResourceVersion", func() {
			goodRV := trackerAddResourceVersion
			By("Deleting with a matching ResourceVersion Precondition")
			err := cl.Delete(context.Background(), dep, client.Preconditions{ResourceVersion: &goodRV})
			Expect(err).To(BeNil())

			list := &appsv1.DeploymentList{}
			err = cl.List(context.Background(), list, client.InNamespace("ns1"))
			Expect(err).To(BeNil())
			Expect(list.Items).To(HaveLen(1))
			Expect(list.Items).To(ConsistOf(*dep2))
		})

		It("should be able to Delete with no ResourceVersion Precondition", func() {
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

		It("should be able to Delete with no opts even if object's ResourceVersion doesn't match server", func() {
			By("Deleting a deployment")
			depCopy := dep.DeepCopy()
			depCopy.ResourceVersion = "bogus"
			err := cl.Delete(context.Background(), depCopy)
			Expect(err).To(BeNil())

			By("Listing all deployments in the namespace")
			list := &appsv1.DeploymentList{}
			err = cl.List(context.Background(), list, client.InNamespace("ns1"))
			Expect(err).To(BeNil())
			Expect(list.Items).To(HaveLen(1))
			Expect(list.Items).To(ConsistOf(*dep2))
		})

		It("should handle finalizers on Update", func() {
			namespacedName := types.NamespacedName{
				Name:      "test-cm",
				Namespace: "delete-with-finalizers",
			}
			By("Updating a new object")
			newObj := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:       namespacedName.Name,
					Namespace:  namespacedName.Namespace,
					Finalizers: []string{"finalizers.sigs.k8s.io/test"},
				},
				Data: map[string]string{
					"test-key": "new-value",
				},
			}
			err := cl.Create(context.Background(), newObj)
			Expect(err).To(BeNil())

			By("Deleting the object")
			err = cl.Delete(context.Background(), newObj)
			Expect(err).To(BeNil())

			By("Getting the object")
			obj := &corev1.ConfigMap{}
			err = cl.Get(context.Background(), namespacedName, obj)
			Expect(err).To(BeNil())
			Expect(obj.DeletionTimestamp).NotTo(BeNil())

			By("Removing the finalizer")
			obj.Finalizers = []string{}
			err = cl.Update(context.Background(), obj)
			Expect(err).To(BeNil())

			By("Getting the object")
			obj = &corev1.ConfigMap{}
			err = cl.Get(context.Background(), namespacedName, obj)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
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

		It("should handle finalizers deleting a collection", func() {
			for i := 0; i < 5; i++ {
				namespacedName := types.NamespacedName{
					Name:      fmt.Sprintf("test-cm-%d", i),
					Namespace: "delete-collection-with-finalizers",
				}
				By("Creating a new object")
				newObj := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:       namespacedName.Name,
						Namespace:  namespacedName.Namespace,
						Finalizers: []string{"finalizers.sigs.k8s.io/test"},
					},
					Data: map[string]string{
						"test-key": "new-value",
					},
				}
				err := cl.Create(context.Background(), newObj)
				Expect(err).To(BeNil())
			}

			By("Deleting the object")
			err := cl.DeleteAllOf(context.Background(), &corev1.ConfigMap{}, client.InNamespace("delete-collection-with-finalizers"))
			Expect(err).To(BeNil())

			configmaps := corev1.ConfigMapList{}
			err = cl.List(context.Background(), &configmaps, client.InNamespace("delete-collection-with-finalizers"))
			Expect(err).To(BeNil())

			Expect(len(configmaps.Items)).To(Equal(5))
			for _, cm := range configmaps.Items {
				Expect(cm.DeletionTimestamp).NotTo(BeNil())
			}
		})

		It("should be able to watch", func() {
			By("Creating a watch")
			objWatch, err := cl.Watch(context.Background(), &corev1.ServiceList{})
			Expect(err).NotTo(HaveOccurred())

			defer objWatch.Stop()

			go func() {
				defer GinkgoRecover()
				// It is likely starting a new goroutine is slower than progressing
				// in the outer routine, sleep to make sure this is always true
				time.Sleep(100 * time.Millisecond)

				err := cl.Create(context.Background(), &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "for-watch"}})
				Expect(err).ToNot(HaveOccurred())
			}()

			event, ok := <-objWatch.ResultChan()
			Expect(ok).To(BeTrue())
			Expect(event.Type).To(Equal(watch.Added))

			service, ok := event.Object.(*corev1.Service)
			Expect(ok).To(BeTrue())
			Expect(service.Name).To(Equal("for-watch"))
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
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
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
				Expect(obj.ObjectMeta.ResourceVersion).To(Equal(trackerAddResourceVersion))
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
			Expect(obj.ObjectMeta.ResourceVersion).To(Equal("1000"))
		})

		It("should handle finalizers on Patch", func() {
			namespacedName := types.NamespacedName{
				Name:      "test-cm",
				Namespace: "delete-with-finalizers",
			}
			By("Updating a new object")
			now := metav1.Now()
			newObj := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:              namespacedName.Name,
					Namespace:         namespacedName.Namespace,
					Finalizers:        []string{"finalizers.sigs.k8s.io/test"},
					DeletionTimestamp: &now,
				},
				Data: map[string]string{
					"test-key": "new-value",
				},
			}
			err := cl.Create(context.Background(), newObj)
			Expect(err).To(BeNil())

			By("Removing the finalizer")
			obj := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:              namespacedName.Name,
					Namespace:         namespacedName.Namespace,
					Finalizers:        []string{},
					DeletionTimestamp: &now,
				},
			}
			obj.Finalizers = []string{}
			err = cl.Patch(context.Background(), obj, client.MergeFrom(newObj))
			Expect(err).To(BeNil())

			By("Getting the object")
			obj = &corev1.ConfigMap{}
			err = cl.Get(context.Background(), namespacedName, obj)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	}

	Context("with default scheme.Scheme", func() {
		BeforeEach(func() {
			cl = NewClientBuilder().
				WithObjects(dep, dep2, cm).
				Build()
		})
		AssertClientBehavior()
	})

	Context("with given scheme", func() {
		BeforeEach(func() {
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			Expect(appsv1.AddToScheme(scheme)).To(Succeed())
			Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())
			cl = NewClientBuilder().
				WithScheme(scheme).
				WithObjects(cm).
				WithLists(&appsv1.DeploymentList{Items: []appsv1.Deployment{*dep, *dep2}}).
				Build()
		})
		AssertClientBehavior()
	})

	It("should set the ResourceVersion to 999 when adding an object to the tracker", func() {
		cl := NewClientBuilder().WithObjects(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "cm"}}).Build()

		retrieved := &corev1.Secret{}
		Expect(cl.Get(context.Background(), types.NamespacedName{Name: "cm"}, retrieved)).To(Succeed())

		reference := &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Secret",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            "cm",
				ResourceVersion: "999",
			},
		}
		Expect(retrieved).To(Equal(reference))
	})
})
