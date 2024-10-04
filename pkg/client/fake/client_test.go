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
	"strconv"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

const (
	machineIDFromStatusUpdate = "machine-id-from-status-update"
	cidrFromStatusUpdate      = "cidr-from-status-update"
)

var _ = Describe("Fake client", func() {
	var dep *appsv1.Deployment
	var dep2 *appsv1.Deployment
	var cm *corev1.ConfigMap
	var cl client.WithWatch

	BeforeEach(func() {
		replicas := int32(1)
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
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Strategy: appsv1.DeploymentStrategy{
					Type: appsv1.RecreateDeploymentStrategyType,
				},
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
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
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

	AssertClientWithoutIndexBehavior := func() {
		It("should be able to Get", func() {
			By("Getting a deployment")
			namespacedName := types.NamespacedName{
				Name:      "test-deployment",
				Namespace: "ns1",
			}
			obj := &appsv1.Deployment{}
			err := cl.Get(context.Background(), namespacedName, obj)
			Expect(err).ToNot(HaveOccurred())
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
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be able to List", func() {
			By("Listing all deployments in a namespace")
			list := &appsv1.DeploymentList{}
			err := cl.List(context.Background(), list, client.InNamespace("ns1"))
			Expect(err).ToNot(HaveOccurred())
			Expect(list.Items).To(HaveLen(2))
			Expect(list.Items).To(ConsistOf(*dep, *dep2))
		})

		It("should be able to List using unstructured list", func() {
			By("Listing all deployments in a namespace")
			list := &unstructured.UnstructuredList{}
			list.SetAPIVersion("apps/v1")
			list.SetKind("DeploymentList")
			err := cl.List(context.Background(), list, client.InNamespace("ns1"))
			Expect(err).ToNot(HaveOccurred())
			Expect(list.Items).To(HaveLen(2))
		})

		It("should be able to List using unstructured list when setting a non-list kind", func() {
			By("Listing all deployments in a namespace")
			list := &unstructured.UnstructuredList{}
			list.SetAPIVersion("apps/v1")
			list.SetKind("Deployment")
			err := cl.List(context.Background(), list, client.InNamespace("ns1"))
			Expect(err).ToNot(HaveOccurred())
			Expect(list.Items).To(HaveLen(2))
		})

		It("should be able to retrieve registered objects that got manipulated as unstructured", func() {
			list := func() {
				By("Listing all endpoints in a namespace")
				list := &unstructured.UnstructuredList{}
				list.SetAPIVersion("v1")
				list.SetKind("EndpointsList")
				err := cl.List(context.Background(), list, client.InNamespace("ns1"))
				Expect(err).ToNot(HaveOccurred())
				Expect(list.Items).To(HaveLen(1))
			}

			unstructuredEndpoint := func() *unstructured.Unstructured {
				item := &unstructured.Unstructured{}
				item.SetAPIVersion("v1")
				item.SetKind("Endpoints")
				item.SetName("test-endpoint")
				item.SetNamespace("ns1")
				return item
			}

			By("Adding the object during client initialization")
			cl = NewClientBuilder().WithRuntimeObjects(unstructuredEndpoint()).Build()
			list()
			Expect(cl.Delete(context.Background(), unstructuredEndpoint())).To(Succeed())

			By("Creating an object")
			item := unstructuredEndpoint()
			err := cl.Create(context.Background(), item)
			Expect(err).ToNot(HaveOccurred())
			list()

			By("Updating the object")
			item.SetAnnotations(map[string]string{"foo": "bar"})
			err = cl.Update(context.Background(), item)
			Expect(err).ToNot(HaveOccurred())
			list()

			By("Patching the object")
			old := item.DeepCopy()
			item.SetAnnotations(map[string]string{"bar": "baz"})
			err = cl.Patch(context.Background(), item, client.MergeFrom(old))
			Expect(err).ToNot(HaveOccurred())
			list()
		})

		It("should be able to Create an unregistered type using unstructured", func() {
			item := &unstructured.Unstructured{}
			item.SetAPIVersion("custom/v1")
			item.SetKind("Image")
			item.SetName("my-item")
			err := cl.Create(context.Background(), item)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be able to Get an unregisted type using unstructured", func() {
			By("Creating an object of an unregistered type")
			item := &unstructured.Unstructured{}
			item.SetAPIVersion("custom/v2")
			item.SetKind("Image")
			item.SetName("my-item")
			err := cl.Create(context.Background(), item)
			Expect(err).ToNot(HaveOccurred())

			By("Getting and the object")
			item = &unstructured.Unstructured{}
			item.SetAPIVersion("custom/v2")
			item.SetKind("Image")
			item.SetName("my-item")
			err = cl.Get(context.Background(), client.ObjectKeyFromObject(item), item)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be able to List an unregistered type using unstructured", func() {
			list := &unstructured.UnstructuredList{}
			list.SetAPIVersion("custom/v3")
			list.SetKind("ImageList")
			err := cl.List(context.Background(), list)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be able to List an unregistered type using unstructured", func() {
			list := &unstructured.UnstructuredList{}
			list.SetAPIVersion("custom/v4")
			list.SetKind("Image")
			err := cl.List(context.Background(), list)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be able to Update an unregistered type using unstructured", func() {
			By("Creating an object of an unregistered type")
			item := &unstructured.Unstructured{}
			item.SetAPIVersion("custom/v5")
			item.SetKind("Image")
			item.SetName("my-item")
			err := cl.Create(context.Background(), item)
			Expect(err).ToNot(HaveOccurred())

			By("Updating the object")
			err = unstructured.SetNestedField(item.Object, int64(2), "spec", "replicas")
			Expect(err).ToNot(HaveOccurred())
			err = cl.Update(context.Background(), item)
			Expect(err).ToNot(HaveOccurred())

			By("Getting the object")
			item = &unstructured.Unstructured{}
			item.SetAPIVersion("custom/v5")
			item.SetKind("Image")
			item.SetName("my-item")
			err = cl.Get(context.Background(), client.ObjectKeyFromObject(item), item)
			Expect(err).ToNot(HaveOccurred())

			By("Inspecting the object")
			value, found, err := unstructured.NestedInt64(item.Object, "spec", "replicas")
			Expect(err).ToNot(HaveOccurred())
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
			Expect(err).ToNot(HaveOccurred())

			By("Updating the object")
			original := item.DeepCopy()
			err = unstructured.SetNestedField(item.Object, int64(2), "spec", "replicas")
			Expect(err).ToNot(HaveOccurred())
			err = cl.Patch(context.Background(), item, client.MergeFrom(original))
			Expect(err).ToNot(HaveOccurred())

			By("Getting the object")
			item = &unstructured.Unstructured{}
			item.SetAPIVersion("custom/v6")
			item.SetKind("Image")
			item.SetName("my-item")
			err = cl.Get(context.Background(), client.ObjectKeyFromObject(item), item)
			Expect(err).ToNot(HaveOccurred())

			By("Inspecting the object")
			value, found, err := unstructured.NestedInt64(item.Object, "spec", "replicas")
			Expect(err).ToNot(HaveOccurred())
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
			Expect(err).ToNot(HaveOccurred())

			By("Deleting the object")
			err = cl.Delete(context.Background(), item)
			Expect(err).ToNot(HaveOccurred())

			By("Getting the object")
			item = &unstructured.Unstructured{}
			item.SetAPIVersion("custom/v7")
			item.SetKind("Image")
			item.SetName("my-item")
			err = cl.Get(context.Background(), client.ObjectKeyFromObject(item), item)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should be able to retrieve objects by PartialObjectMetadata", func() {
			By("Creating a Resource")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
			}
			err := cl.Create(context.Background(), secret)
			Expect(err).ToNot(HaveOccurred())

			By("Fetching the resource using a PartialObjectMeta")
			partialObjMeta := &metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
			}
			partialObjMeta.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))

			err = cl.Get(context.Background(), client.ObjectKeyFromObject(partialObjMeta), partialObjMeta)
			Expect(err).ToNot(HaveOccurred())

			Expect(partialObjMeta.Kind).To(Equal("Secret"))
			Expect(partialObjMeta.APIVersion).To(Equal("v1"))
		})

		It("should support filtering by labels and their values", func() {
			By("Listing deployments with a particular label and value")
			list := &appsv1.DeploymentList{}
			err := cl.List(context.Background(), list, client.InNamespace("ns1"),
				client.MatchingLabels(map[string]string{
					"test-label": "label-value",
				}))
			Expect(err).ToNot(HaveOccurred())
			Expect(list.Items).To(HaveLen(1))
			Expect(list.Items).To(ConsistOf(*dep2))
		})

		It("should support filtering by label existence", func() {
			By("Listing deployments with a particular label")
			list := &appsv1.DeploymentList{}
			err := cl.List(context.Background(), list, client.InNamespace("ns1"),
				client.HasLabels{"test-label"})
			Expect(err).ToNot(HaveOccurred())
			Expect(list.Items).To(HaveLen(1))
			Expect(list.Items).To(ConsistOf(*dep2))
		})

		It("should reject apply patches, they are not supported in the fake client", func() {
			By("Creating a new configmap")
			cm := &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-test-cm",
					Namespace: "ns2",
				},
			}
			err := cl.Create(context.Background(), cm)
			Expect(err).ToNot(HaveOccurred())

			cm.Data = map[string]string{"foo": "bar"}
			err = cl.Patch(context.Background(), cm, client.Apply, client.ForceOwnership)
			Expect(err).To(MatchError(ContainSubstring("apply patches are not supported in the fake client")))
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
			Expect(err).ToNot(HaveOccurred())

			By("Getting the new configmap")
			namespacedName := types.NamespacedName{
				Name:      "new-test-cm",
				Namespace: "ns2",
			}
			obj := &corev1.ConfigMap{}
			err = cl.Get(context.Background(), namespacedName, obj)
			Expect(err).ToNot(HaveOccurred())
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
			Expect(err).To(HaveOccurred())
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
			Expect(err).ToNot(HaveOccurred())

			By("Listing configmaps with a particular label")
			list := &corev1.ConfigMapList{}
			err = cl.List(context.Background(), list, client.InNamespace("ns2"),
				client.MatchingLabels(map[string]string{
					"test-label": "label-value",
				}))
			Expect(err).ToNot(HaveOccurred())
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
			Expect(err).ToNot(HaveOccurred())

			By("Getting the new configmap")
			namespacedName := types.NamespacedName{
				Name:      "test-cm",
				Namespace: "ns2",
			}
			obj := &corev1.ConfigMap{}
			err = cl.Get(context.Background(), namespacedName, obj)
			Expect(err).ToNot(HaveOccurred())
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
			Expect(err).ToNot(HaveOccurred())

			By("Getting the configmap")
			namespacedName := types.NamespacedName{
				Name:      "test-cm",
				Namespace: "ns2",
			}
			obj := &corev1.ConfigMap{}
			err = cl.Get(context.Background(), namespacedName, obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(obj).To(Equal(newcm))
			Expect(obj.ObjectMeta.ResourceVersion).To(Equal("1000"))
		})

		It("should allow patch with non-set ResourceVersion for a resource that doesn't allow unconditional updates", func() {
			schemeBuilder := &scheme.Builder{GroupVersion: schema.GroupVersion{Group: "test", Version: "v1"}}
			schemeBuilder.Register(&WithPointerMeta{}, &WithPointerMetaList{})

			scheme := runtime.NewScheme()
			Expect(schemeBuilder.AddToScheme(scheme)).NotTo(HaveOccurred())

			cl := NewClientBuilder().WithScheme(scheme).Build()
			original := &WithPointerMeta{
				ObjectMeta: &metav1.ObjectMeta{
					Name:      "obj",
					Namespace: "ns2",
				}}

			err := cl.Create(context.Background(), original)
			Expect(err).ToNot(HaveOccurred())

			newObj := &WithPointerMeta{
				ObjectMeta: &metav1.ObjectMeta{
					Name:      original.Name,
					Namespace: original.Namespace,
					Annotations: map[string]string{
						"foo": "bar",
					},
				}}
			Expect(cl.Patch(context.Background(), newObj, client.MergeFrom(original))).To(Succeed())

			patched := &WithPointerMeta{}
			Expect(cl.Get(context.Background(), client.ObjectKeyFromObject(original), patched)).To(Succeed())
			Expect(patched.Annotations).To(Equal(map[string]string{"foo": "bar"}))
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
			Expect(err).ToNot(HaveOccurred())
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
			Expect(err).ToNot(HaveOccurred())
			Expect(list.Items).To(HaveLen(2))
			Expect(list.Items).To(ConsistOf(*dep, *dep2))
		})

		It("should successfully Delete with a matching ResourceVersion", func() {
			goodRV := trackerAddResourceVersion
			By("Deleting with a matching ResourceVersion Precondition")
			err := cl.Delete(context.Background(), dep, client.Preconditions{ResourceVersion: &goodRV})
			Expect(err).ToNot(HaveOccurred())

			list := &appsv1.DeploymentList{}
			err = cl.List(context.Background(), list, client.InNamespace("ns1"))
			Expect(err).ToNot(HaveOccurred())
			Expect(list.Items).To(HaveLen(1))
			Expect(list.Items).To(ConsistOf(*dep2))
		})

		It("should be able to Delete with no ResourceVersion Precondition", func() {
			By("Deleting a deployment")
			err := cl.Delete(context.Background(), dep)
			Expect(err).ToNot(HaveOccurred())

			By("Listing all deployments in the namespace")
			list := &appsv1.DeploymentList{}
			err = cl.List(context.Background(), list, client.InNamespace("ns1"))
			Expect(err).ToNot(HaveOccurred())
			Expect(list.Items).To(HaveLen(1))
			Expect(list.Items).To(ConsistOf(*dep2))
		})

		It("should be able to Delete with no opts even if object's ResourceVersion doesn't match server", func() {
			By("Deleting a deployment")
			depCopy := dep.DeepCopy()
			depCopy.ResourceVersion = "bogus"
			err := cl.Delete(context.Background(), depCopy)
			Expect(err).ToNot(HaveOccurred())

			By("Listing all deployments in the namespace")
			list := &appsv1.DeploymentList{}
			err = cl.List(context.Background(), list, client.InNamespace("ns1"))
			Expect(err).ToNot(HaveOccurred())
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
			Expect(err).ToNot(HaveOccurred())

			By("Deleting the object")
			err = cl.Delete(context.Background(), newObj)
			Expect(err).ToNot(HaveOccurred())

			By("Getting the object")
			obj := &corev1.ConfigMap{}
			err = cl.Get(context.Background(), namespacedName, obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(obj.DeletionTimestamp).NotTo(BeNil())

			By("Removing the finalizer")
			obj.Finalizers = []string{}
			err = cl.Update(context.Background(), obj)
			Expect(err).ToNot(HaveOccurred())

			By("Getting the object")
			obj = &corev1.ConfigMap{}
			err = cl.Get(context.Background(), namespacedName, obj)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should reject changes to deletionTimestamp on Update", func() {
			namespacedName := types.NamespacedName{
				Name:      "test-cm",
				Namespace: "reject-with-deletiontimestamp",
			}
			By("Updating a new object")
			newObj := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedName.Name,
					Namespace: namespacedName.Namespace,
				},
				Data: map[string]string{
					"test-key": "new-value",
				},
			}
			err := cl.Create(context.Background(), newObj)
			Expect(err).ToNot(HaveOccurred())

			By("Getting the object")
			obj := &corev1.ConfigMap{}
			err = cl.Get(context.Background(), namespacedName, obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(obj.DeletionTimestamp).To(BeNil())

			By("Adding deletionTimestamp")
			now := metav1.Now()
			obj.DeletionTimestamp = &now
			err = cl.Update(context.Background(), obj)
			Expect(err).To(HaveOccurred())

			By("Deleting the object")
			err = cl.Delete(context.Background(), newObj)
			Expect(err).ToNot(HaveOccurred())

			By("Changing the deletionTimestamp to new value")
			obj = &corev1.ConfigMap{}
			t := metav1.NewTime(time.Now().Add(time.Second))
			obj.DeletionTimestamp = &t
			err = cl.Update(context.Background(), obj)
			Expect(err).To(HaveOccurred())

			By("Removing deletionTimestamp")
			obj.DeletionTimestamp = nil
			err = cl.Update(context.Background(), obj)
			Expect(err).To(HaveOccurred())

		})

		It("should be able to Delete a Collection", func() {
			By("Deleting a deploymentList")
			err := cl.DeleteAllOf(context.Background(), &appsv1.Deployment{}, client.InNamespace("ns1"))
			Expect(err).ToNot(HaveOccurred())

			By("Listing all deployments in the namespace")
			list := &appsv1.DeploymentList{}
			err = cl.List(context.Background(), list, client.InNamespace("ns1"))
			Expect(err).ToNot(HaveOccurred())
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
				Expect(err).ToNot(HaveOccurred())
			}

			By("Deleting the object")
			err := cl.DeleteAllOf(context.Background(), &corev1.ConfigMap{}, client.InNamespace("delete-collection-with-finalizers"))
			Expect(err).ToNot(HaveOccurred())

			configmaps := corev1.ConfigMapList{}
			err = cl.List(context.Background(), &configmaps, client.InNamespace("delete-collection-with-finalizers"))
			Expect(err).ToNot(HaveOccurred())

			Expect(configmaps.Items).To(HaveLen(5))
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
				Expect(err).ToNot(HaveOccurred())

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
				Expect(err).ToNot(HaveOccurred())

				By("Getting the new configmap")
				namespacedName := types.NamespacedName{
					Name:      "test-cm",
					Namespace: "ns2",
				}
				obj := &corev1.ConfigMap{}
				err = cl.Get(context.Background(), namespacedName, obj)
				Expect(err).ToNot(HaveOccurred())
				Expect(obj).To(Equal(cm))
				Expect(obj.ObjectMeta.ResourceVersion).To(Equal(trackerAddResourceVersion))
			})

			It("Should not Delete the object", func() {
				By("Deleting a configmap with DryRun with Delete()")
				err := cl.Delete(context.Background(), cm, client.DryRunAll)
				Expect(err).ToNot(HaveOccurred())

				By("Deleting a configmap with DryRun with DeleteAllOf()")
				err = cl.DeleteAllOf(context.Background(), cm, client.DryRunAll)
				Expect(err).ToNot(HaveOccurred())

				By("Getting the configmap")
				namespacedName := types.NamespacedName{
					Name:      "test-cm",
					Namespace: "ns2",
				}
				obj := &corev1.ConfigMap{}
				err = cl.Get(context.Background(), namespacedName, obj)
				Expect(err).ToNot(HaveOccurred())
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

		It("should ignore deletionTimestamp without finalizer on Create", func() {
			namespacedName := types.NamespacedName{
				Name:      "test-cm",
				Namespace: "ignore-deletiontimestamp",
			}
			By("Creating a new object")
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
			Expect(err).ToNot(HaveOccurred())

			By("Getting the object")
			obj := &corev1.ConfigMap{}
			err = cl.Get(context.Background(), namespacedName, obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(obj.DeletionTimestamp).To(BeNil())

		})

		It("should reject deletionTimestamp without finalizers on Build", func() {
			namespacedName := types.NamespacedName{
				Name:      "test-cm",
				Namespace: "reject-deletiontimestamp-no-finalizers",
			}
			By("Build with a new object without finalizer")
			now := metav1.Now()
			obj := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:              namespacedName.Name,
					Namespace:         namespacedName.Namespace,
					DeletionTimestamp: &now,
				},
				Data: map[string]string{
					"test-key": "new-value",
				},
			}

			Expect(func() { NewClientBuilder().WithObjects(obj).Build() }).To(Panic())

			By("Build with a new object with finalizer")
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

			cl := NewClientBuilder().WithObjects(newObj).Build()

			By("Getting the object")
			obj = &corev1.ConfigMap{}
			err := cl.Get(context.Background(), namespacedName, obj)
			Expect(err).ToNot(HaveOccurred())

		})

		It("should reject changes to deletionTimestamp on Patch", func() {
			namespacedName := types.NamespacedName{
				Name:      "test-cm",
				Namespace: "reject-deletiontimestamp",
			}
			By("Creating a new object")
			now := metav1.Now()
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
			Expect(err).ToNot(HaveOccurred())

			By("Add a deletionTimestamp")
			obj := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:              namespacedName.Name,
					Namespace:         namespacedName.Namespace,
					Finalizers:        []string{},
					DeletionTimestamp: &now,
				},
			}
			err = cl.Patch(context.Background(), obj, client.MergeFrom(newObj))
			Expect(err).To(HaveOccurred())

			By("Deleting the object")
			err = cl.Delete(context.Background(), newObj)
			Expect(err).ToNot(HaveOccurred())

			By("Getting the object")
			obj = &corev1.ConfigMap{}
			err = cl.Get(context.Background(), namespacedName, obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(obj.DeletionTimestamp).NotTo(BeNil())

			By("Changing the deletionTimestamp to new value")
			newObj = &corev1.ConfigMap{}
			t := metav1.NewTime(time.Now().Add(time.Second))
			newObj.DeletionTimestamp = &t
			err = cl.Patch(context.Background(), newObj, client.MergeFrom(obj))
			Expect(err).To(HaveOccurred())

			By("Removing deletionTimestamp")
			newObj = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:              namespacedName.Name,
					Namespace:         namespacedName.Namespace,
					DeletionTimestamp: nil,
				},
			}
			err = cl.Patch(context.Background(), newObj, client.MergeFrom(obj))
			Expect(err).To(HaveOccurred())

		})

		It("should handle finalizers on Patch", func() {
			namespacedName := types.NamespacedName{
				Name:      "test-cm",
				Namespace: "delete-with-finalizers",
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
			Expect(err).ToNot(HaveOccurred())

			By("Deleting the object")
			err = cl.Delete(context.Background(), newObj)
			Expect(err).ToNot(HaveOccurred())

			By("Removing the finalizer")
			obj := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:       namespacedName.Name,
					Namespace:  namespacedName.Namespace,
					Finalizers: []string{},
				},
			}
			err = cl.Patch(context.Background(), obj, client.MergeFrom(newObj))
			Expect(err).ToNot(HaveOccurred())

			By("Getting the object")
			obj = &corev1.ConfigMap{}
			err = cl.Get(context.Background(), namespacedName, obj)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should remove finalizers of the object on Patch", func() {
			namespacedName := types.NamespacedName{
				Name:      "test-cm",
				Namespace: "patch-finalizers-in-obj",
			}
			By("Creating a new object")
			obj := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:       namespacedName.Name,
					Namespace:  namespacedName.Namespace,
					Finalizers: []string{"finalizers.sigs.k8s.io/test"},
				},
				Data: map[string]string{
					"test-key": "new-value",
				},
			}
			err := cl.Create(context.Background(), obj)
			Expect(err).ToNot(HaveOccurred())

			By("Removing the finalizer")
			mergePatch, err := json.Marshal(map[string]interface{}{
				"metadata": map[string]interface{}{
					"$deleteFromPrimitiveList/finalizers": []string{
						"finalizers.sigs.k8s.io/test",
					},
				},
			})
			Expect(err).ToNot(HaveOccurred())
			err = cl.Patch(context.Background(), obj, client.RawPatch(types.StrategicMergePatchType, mergePatch))
			Expect(err).ToNot(HaveOccurred())

			By("Check the finalizer has been removed in the object")
			Expect(obj.Finalizers).To(BeEmpty())

			By("Check the finalizer has been removed in client")
			newObj := &corev1.ConfigMap{}
			err = cl.Get(context.Background(), namespacedName, newObj)
			Expect(err).ToNot(HaveOccurred())
			Expect(newObj.Finalizers).To(BeEmpty())
		})

	}

	Context("with default scheme.Scheme", func() {
		BeforeEach(func() {
			cl = NewClientBuilder().
				WithObjects(dep, dep2, cm).
				Build()
		})
		AssertClientWithoutIndexBehavior()
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
		AssertClientWithoutIndexBehavior()
	})

	Context("with Indexes", func() {
		depReplicasIndexer := func(obj client.Object) []string {
			dep, ok := obj.(*appsv1.Deployment)
			if !ok {
				panic(fmt.Errorf("indexer function for type %T's spec.replicas field received"+
					" object of type %T, this should never happen", appsv1.Deployment{}, obj))
			}
			indexVal := ""
			if dep.Spec.Replicas != nil {
				indexVal = strconv.Itoa(int(*dep.Spec.Replicas))
			}
			return []string{indexVal}
		}

		depStrategyTypeIndexer := func(obj client.Object) []string {
			dep, ok := obj.(*appsv1.Deployment)
			if !ok {
				panic(fmt.Errorf("indexer function for type %T's spec.strategy.type field received"+
					" object of type %T, this should never happen", appsv1.Deployment{}, obj))
			}
			return []string{string(dep.Spec.Strategy.Type)}
		}

		var cb *ClientBuilder
		BeforeEach(func() {
			cb = NewClientBuilder().
				WithObjects(dep, dep2, cm).
				WithIndex(&appsv1.Deployment{}, "spec.replicas", depReplicasIndexer)
		})

		Context("client has just one Index", func() {
			BeforeEach(func() { cl = cb.Build() })

			Context("behavior that doesn't use an Index", func() {
				AssertClientWithoutIndexBehavior()
			})

			Context("filtered List using field selector", func() {
				It("errors when there's no Index for the GroupVersionResource", func() {
					listOpts := &client.ListOptions{
						FieldSelector: fields.OneTermEqualSelector("key", "val"),
					}
					err := cl.List(context.Background(), &corev1.ConfigMapList{}, listOpts)
					Expect(err).To(HaveOccurred())
				})

				It("errors when there's no Index matching the field name", func() {
					listOpts := &client.ListOptions{
						FieldSelector: fields.OneTermEqualSelector("spec.paused", "false"),
					}
					err := cl.List(context.Background(), &appsv1.DeploymentList{}, listOpts)
					Expect(err).To(HaveOccurred())
				})

				It("errors when field selector uses two requirements", func() {
					listOpts := &client.ListOptions{
						FieldSelector: fields.AndSelectors(
							fields.OneTermEqualSelector("spec.replicas", "1"),
							fields.OneTermEqualSelector("spec.strategy.type", string(appsv1.RecreateDeploymentStrategyType)),
						)}
					err := cl.List(context.Background(), &appsv1.DeploymentList{}, listOpts)
					Expect(err).To(HaveOccurred())
				})

				It("returns two deployments that match the only field selector requirement", func() {
					listOpts := &client.ListOptions{
						FieldSelector: fields.OneTermEqualSelector("spec.replicas", "1"),
					}
					list := &appsv1.DeploymentList{}
					Expect(cl.List(context.Background(), list, listOpts)).To(Succeed())
					Expect(list.Items).To(ConsistOf(*dep, *dep2))
				})

				It("returns no object because no object matches the only field selector requirement", func() {
					listOpts := &client.ListOptions{
						FieldSelector: fields.OneTermEqualSelector("spec.replicas", "2"),
					}
					list := &appsv1.DeploymentList{}
					Expect(cl.List(context.Background(), list, listOpts)).To(Succeed())
					Expect(list.Items).To(BeEmpty())
				})

				It("returns deployment that matches both the field and label selectors", func() {
					listOpts := &client.ListOptions{
						FieldSelector: fields.OneTermEqualSelector("spec.replicas", "1"),
						LabelSelector: labels.SelectorFromSet(dep2.Labels),
					}
					list := &appsv1.DeploymentList{}
					Expect(cl.List(context.Background(), list, listOpts)).To(Succeed())
					Expect(list.Items).To(ConsistOf(*dep2))
				})

				It("returns no object even if field selector matches because label selector doesn't", func() {
					listOpts := &client.ListOptions{
						FieldSelector: fields.OneTermEqualSelector("spec.replicas", "1"),
						LabelSelector: labels.Nothing(),
					}
					list := &appsv1.DeploymentList{}
					Expect(cl.List(context.Background(), list, listOpts)).To(Succeed())
					Expect(list.Items).To(BeEmpty())
				})

				It("returns no object even if label selector matches because field selector doesn't", func() {
					listOpts := &client.ListOptions{
						FieldSelector: fields.OneTermEqualSelector("spec.replicas", "2"),
						LabelSelector: labels.Everything(),
					}
					list := &appsv1.DeploymentList{}
					Expect(cl.List(context.Background(), list, listOpts)).To(Succeed())
					Expect(list.Items).To(BeEmpty())
				})
			})
		})

		Context("client has two Indexes", func() {
			BeforeEach(func() {
				cl = cb.WithIndex(&appsv1.Deployment{}, "spec.strategy.type", depStrategyTypeIndexer).Build()
			})

			Context("behavior that doesn't use an Index", func() {
				AssertClientWithoutIndexBehavior()
			})

			Context("filtered List using field selector", func() {
				It("uses the second index to retrieve the indexed objects when there are matches", func() {
					listOpts := &client.ListOptions{
						FieldSelector: fields.OneTermEqualSelector("spec.strategy.type", string(appsv1.RecreateDeploymentStrategyType)),
					}
					list := &appsv1.DeploymentList{}
					Expect(cl.List(context.Background(), list, listOpts)).To(Succeed())
					Expect(list.Items).To(ConsistOf(*dep))
				})

				It("uses the second index to retrieve the indexed objects when there are no matches", func() {
					listOpts := &client.ListOptions{
						FieldSelector: fields.OneTermEqualSelector("spec.strategy.type", string(appsv1.RollingUpdateDeploymentStrategyType)),
					}
					list := &appsv1.DeploymentList{}
					Expect(cl.List(context.Background(), list, listOpts)).To(Succeed())
					Expect(list.Items).To(BeEmpty())
				})

				It("no error when field selector uses two requirements", func() {
					listOpts := &client.ListOptions{
						FieldSelector: fields.AndSelectors(
							fields.OneTermEqualSelector("spec.replicas", "1"),
							fields.OneTermEqualSelector("spec.strategy.type", string(appsv1.RecreateDeploymentStrategyType)),
						)}
					list := &appsv1.DeploymentList{}
					Expect(cl.List(context.Background(), list, listOpts)).To(Succeed())
					Expect(list.Items).To(ConsistOf(*dep))
				})
			})
		})
	})

	It("should set the ResourceVersion to 999 when adding an object to the tracker", func() {
		cl := NewClientBuilder().WithObjects(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "cm"}}).Build()

		retrieved := &corev1.Secret{}
		Expect(cl.Get(context.Background(), types.NamespacedName{Name: "cm"}, retrieved)).To(Succeed())

		reference := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "cm",
				ResourceVersion: "999",
			},
		}
		Expect(retrieved).To(Equal(reference))
	})

	It("should be able to build with given tracker and get resource", func() {
		clientSet := fake.NewSimpleClientset(dep)
		cl := NewClientBuilder().WithRuntimeObjects(dep2).WithObjectTracker(clientSet.Tracker()).Build()

		By("Getting a deployment")
		namespacedName := types.NamespacedName{
			Name:      "test-deployment",
			Namespace: "ns1",
		}
		obj := &appsv1.Deployment{}
		err := cl.Get(context.Background(), namespacedName, obj)
		Expect(err).ToNot(HaveOccurred())
		Expect(obj).To(Equal(dep))

		By("Getting a deployment from clientSet")
		csDep2, err := clientSet.AppsV1().Deployments("ns1").Get(context.Background(), "test-deployment-2", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(csDep2).To(Equal(dep2))

		By("Getting a new deployment")
		namespacedName3 := types.NamespacedName{
			Name:      "test-deployment-3",
			Namespace: "ns1",
		}

		dep3 := &appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deployment-3",
				Namespace: "ns1",
				Labels: map[string]string{
					"test-label": "label-value",
				},
				ResourceVersion: trackerAddResourceVersion,
			},
		}

		_, err = clientSet.AppsV1().Deployments("ns1").Create(context.Background(), dep3, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		obj = &appsv1.Deployment{}
		err = cl.Get(context.Background(), namespacedName3, obj)
		Expect(err).ToNot(HaveOccurred())
		Expect(obj).To(Equal(dep3))
	})

	It("should not change the status of typed objects that have a status subresource on update", func() {
		obj := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod",
			},
		}
		cl := NewClientBuilder().WithStatusSubresource(obj).WithObjects(obj).Build()

		obj.Status.Phase = "Running"
		Expect(cl.Update(context.Background(), obj)).To(Succeed())

		Expect(cl.Get(context.Background(), client.ObjectKeyFromObject(obj), obj)).To(Succeed())

		Expect(obj.Status).To(BeEquivalentTo(corev1.PodStatus{}))
	})

	It("should return a conflict error when an incorrect RV is used on status update", func() {
		obj := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "node",
				ResourceVersion: trackerAddResourceVersion,
			},
		}
		cl := NewClientBuilder().WithStatusSubresource(obj).WithObjects(obj).Build()

		obj.Status.Phase = corev1.NodeRunning
		obj.ResourceVersion = "invalid"
		err := cl.Status().Update(context.Background(), obj)
		Expect(apierrors.IsConflict(err)).To(BeTrue())
	})

	It("should not change non-status field of typed objects that have a status subresource on status update", func() {
		obj := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node",
			},
			Spec: corev1.NodeSpec{
				PodCIDR: "old-cidr",
			},
			Status: corev1.NodeStatus{
				NodeInfo: corev1.NodeSystemInfo{
					MachineID: "machine-id",
				},
			},
		}
		cl := NewClientBuilder().WithStatusSubresource(obj).WithObjects(obj).Build()
		objOriginal := obj.DeepCopy()

		obj.Spec.PodCIDR = cidrFromStatusUpdate
		obj.Annotations = map[string]string{
			"some-annotation-key": "some-annotation-value",
		}
		obj.Labels = map[string]string{
			"some-label-key": "some-label-value",
		}

		obj.Status.NodeInfo.MachineID = machineIDFromStatusUpdate
		Expect(cl.Status().Update(context.Background(), obj)).NotTo(HaveOccurred())

		actual := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: obj.Name}}
		Expect(cl.Get(context.Background(), client.ObjectKeyFromObject(actual), actual)).NotTo(HaveOccurred())

		objOriginal.APIVersion = actual.APIVersion
		objOriginal.Kind = actual.Kind
		objOriginal.ResourceVersion = actual.ResourceVersion
		objOriginal.Status.NodeInfo.MachineID = machineIDFromStatusUpdate
		Expect(cmp.Diff(objOriginal, actual)).To(BeEmpty())
	})

	It("should be able to update an object after updating an object's status", func() {
		obj := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node",
			},
			Spec: corev1.NodeSpec{
				PodCIDR: "old-cidr",
			},
			Status: corev1.NodeStatus{
				NodeInfo: corev1.NodeSystemInfo{
					MachineID: "machine-id",
				},
			},
		}
		cl := NewClientBuilder().WithStatusSubresource(obj).WithObjects(obj).Build()
		expectedObj := obj.DeepCopy()

		obj.Status.NodeInfo.MachineID = machineIDFromStatusUpdate
		Expect(cl.Status().Update(context.Background(), obj)).NotTo(HaveOccurred())

		obj.Annotations = map[string]string{
			"some-annotation-key": "some",
		}
		expectedObj.Annotations = map[string]string{
			"some-annotation-key": "some",
		}
		Expect(cl.Update(context.Background(), obj)).NotTo(HaveOccurred())

		actual := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: obj.Name}}
		Expect(cl.Get(context.Background(), client.ObjectKeyFromObject(actual), actual)).NotTo(HaveOccurred())

		expectedObj.APIVersion = actual.APIVersion
		expectedObj.Kind = actual.Kind
		expectedObj.ResourceVersion = actual.ResourceVersion
		expectedObj.Status.NodeInfo.MachineID = machineIDFromStatusUpdate
		Expect(cmp.Diff(expectedObj, actual)).To(BeEmpty())
	})

	It("should be able to update an object's status after updating an object", func() {
		obj := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node",
			},
			Spec: corev1.NodeSpec{
				PodCIDR: "old-cidr",
			},
			Status: corev1.NodeStatus{
				NodeInfo: corev1.NodeSystemInfo{
					MachineID: "machine-id",
				},
			},
		}
		cl := NewClientBuilder().WithStatusSubresource(obj).WithObjects(obj).Build()
		expectedObj := obj.DeepCopy()

		obj.Annotations = map[string]string{
			"some-annotation-key": "some",
		}
		expectedObj.Annotations = map[string]string{
			"some-annotation-key": "some",
		}
		Expect(cl.Update(context.Background(), obj)).NotTo(HaveOccurred())

		obj.Spec.PodCIDR = cidrFromStatusUpdate
		obj.Status.NodeInfo.MachineID = machineIDFromStatusUpdate
		Expect(cl.Status().Update(context.Background(), obj)).NotTo(HaveOccurred())

		actual := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: obj.Name}}
		Expect(cl.Get(context.Background(), client.ObjectKeyFromObject(actual), actual)).NotTo(HaveOccurred())

		expectedObj.APIVersion = actual.APIVersion
		expectedObj.Kind = actual.Kind
		expectedObj.ResourceVersion = actual.ResourceVersion
		expectedObj.Status.NodeInfo.MachineID = machineIDFromStatusUpdate
		Expect(cmp.Diff(expectedObj, actual)).To(BeEmpty())
	})

	It("Should only override status fields of typed objects that have a status subresource on status update", func() {
		obj := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node",
			},
			Spec: corev1.NodeSpec{
				PodCIDR: "old-cidr",
			},
			Status: corev1.NodeStatus{
				NodeInfo: corev1.NodeSystemInfo{
					MachineID: "machine-id",
				},
			},
		}
		cl := NewClientBuilder().WithStatusSubresource(obj).WithObjects(obj).Build()
		objOriginal := obj.DeepCopy()

		obj.Status.Phase = corev1.NodeRunning
		Expect(cl.Status().Update(context.Background(), obj)).NotTo(HaveOccurred())

		actual := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: obj.Name}}
		Expect(cl.Get(context.Background(), client.ObjectKeyFromObject(actual), actual)).NotTo(HaveOccurred())

		objOriginal.APIVersion = actual.APIVersion
		objOriginal.Kind = actual.Kind
		objOriginal.ResourceVersion = actual.ResourceVersion
		Expect(cmp.Diff(objOriginal, actual)).ToNot(BeEmpty())
		Expect(objOriginal.Status.NodeInfo.MachineID).To(Equal(actual.Status.NodeInfo.MachineID))
		Expect(objOriginal.Status.Phase).ToNot(Equal(actual.Status.Phase))
	})

	It("should be able to change typed objects that have a scale subresource on patch", func() {
		obj := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "deploy",
			},
		}
		cl := NewClientBuilder().WithObjects(obj).Build()
		objOriginal := obj.DeepCopy()

		patch := []byte(fmt.Sprintf(`{"spec":{"replicas":%d}}`, 2))
		Expect(cl.SubResource("scale").Patch(context.Background(), obj, client.RawPatch(types.MergePatchType, patch))).NotTo(HaveOccurred())

		actual := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: obj.Name}}
		Expect(cl.Get(context.Background(), client.ObjectKeyFromObject(actual), actual)).To(Succeed())

		objOriginal.APIVersion = actual.APIVersion
		objOriginal.Kind = actual.Kind
		objOriginal.ResourceVersion = actual.ResourceVersion
		objOriginal.Spec.Replicas = ptr.To(int32(2))
		Expect(cmp.Diff(objOriginal, actual)).To(BeEmpty())
	})

	It("should not change the status of typed objects that have a status subresource on patch", func() {
		obj := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node",
			},
		}
		Expect(cl.Create(context.Background(), obj)).To(Succeed())
		original := obj.DeepCopy()

		obj.Status.Phase = "Running"
		Expect(cl.Patch(context.Background(), obj, client.MergeFrom(original))).To(Succeed())

		Expect(cl.Get(context.Background(), client.ObjectKeyFromObject(obj), obj)).To(Succeed())

		Expect(obj.Status).To(BeEquivalentTo(corev1.PodStatus{}))
	})

	It("should not change non-status field of typed objects that have a status subresource on status patch", func() {
		obj := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node",
			},
			Spec: corev1.NodeSpec{
				PodCIDR: "old-cidr",
			},
		}
		cl := NewClientBuilder().WithStatusSubresource(obj).WithObjects(obj).Build()
		objOriginal := obj.DeepCopy()

		obj.Spec.PodCIDR = cidrFromStatusUpdate
		obj.Status.NodeInfo.MachineID = "machine-id"
		Expect(cl.Status().Patch(context.Background(), obj, client.MergeFrom(objOriginal))).NotTo(HaveOccurred())

		actual := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: obj.Name}}
		Expect(cl.Get(context.Background(), client.ObjectKeyFromObject(actual), actual)).NotTo(HaveOccurred())

		objOriginal.APIVersion = actual.APIVersion
		objOriginal.Kind = actual.Kind
		objOriginal.ResourceVersion = actual.ResourceVersion
		objOriginal.Status.NodeInfo.MachineID = "machine-id"
		Expect(cmp.Diff(objOriginal, actual)).To(BeEmpty())
	})

	It("should not change the status of unstructured objects that are configured to have a status subresource on update", func() {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion("foo/v1")
		obj.SetKind("Foo")
		obj.SetName("a-foo")

		err := unstructured.SetNestedField(obj.Object, map[string]any{"state": "old"}, "status")
		Expect(err).NotTo(HaveOccurred())

		cl := NewClientBuilder().WithStatusSubresource(obj).WithObjects(obj).Build()

		err = unstructured.SetNestedField(obj.Object, map[string]any{"state": "new"}, "status")
		Expect(err).ToNot(HaveOccurred())

		Expect(cl.Update(context.Background(), obj)).To(Succeed())

		Expect(cl.Get(context.Background(), client.ObjectKeyFromObject(obj), obj)).To(Succeed())

		Expect(obj.Object["status"]).To(BeEquivalentTo(map[string]any{"state": "old"}))
	})

	It("should not change non-status fields of unstructured objects that are configured to have a status subresource on status update", func() {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion("foo/v1")
		obj.SetKind("Foo")
		obj.SetName("a-foo")

		err := unstructured.SetNestedField(obj.Object, "original", "spec")
		Expect(err).NotTo(HaveOccurred())

		cl := NewClientBuilder().WithStatusSubresource(obj).WithObjects(obj).Build()

		err = unstructured.SetNestedField(obj.Object, "from-status-update", "spec")
		Expect(err).NotTo(HaveOccurred())
		err = unstructured.SetNestedField(obj.Object, map[string]any{"state": "new"}, "status")
		Expect(err).ToNot(HaveOccurred())

		Expect(cl.Status().Update(context.Background(), obj)).To(Succeed())
		Expect(cl.Get(context.Background(), client.ObjectKeyFromObject(obj), obj)).To(Succeed())

		Expect(obj.Object["status"]).To(BeEquivalentTo(map[string]any{"state": "new"}))
		Expect(obj.Object["spec"]).To(BeEquivalentTo("original"))
	})

	It("should not change the status of known unstructured objects that have a status subresource on update", func() {
		obj := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod",
			},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyAlways,
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
			},
		}
		cl := NewClientBuilder().WithStatusSubresource(obj).WithObjects(obj).Build()

		// update using unstructured
		u := &unstructured.Unstructured{}
		u.SetAPIVersion("v1")
		u.SetKind("Pod")
		u.SetName(obj.Name)
		err := cl.Get(context.Background(), client.ObjectKeyFromObject(u), u)
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(u.Object, string(corev1.RestartPolicyNever), "spec", "restartPolicy")
		Expect(err).NotTo(HaveOccurred())
		err = unstructured.SetNestedField(u.Object, string(corev1.PodRunning), "status", "phase")
		Expect(err).NotTo(HaveOccurred())

		Expect(cl.Update(context.Background(), u)).To(Succeed())

		actual := &corev1.Pod{}
		Expect(cl.Get(context.Background(), client.ObjectKeyFromObject(obj), actual)).To(Succeed())
		obj.APIVersion = u.GetAPIVersion()
		obj.Kind = u.GetKind()
		obj.ResourceVersion = actual.ResourceVersion
		// only the spec mutation should persist
		obj.Spec.RestartPolicy = corev1.RestartPolicyNever
		Expect(cmp.Diff(obj, actual)).To(BeEmpty())
	})

	It("should not change non-status field of known unstructured objects that have a status subresource on status update", func() {
		obj := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod",
			},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyAlways,
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
			},
		}
		cl := NewClientBuilder().WithStatusSubresource(obj).WithObjects(obj).Build()

		// status update using unstructured
		u := &unstructured.Unstructured{}
		u.SetAPIVersion("v1")
		u.SetKind("Pod")
		u.SetName(obj.Name)
		err := cl.Get(context.Background(), client.ObjectKeyFromObject(u), u)
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(u.Object, string(corev1.RestartPolicyNever), "spec", "restartPolicy")
		Expect(err).NotTo(HaveOccurred())
		err = unstructured.SetNestedField(u.Object, string(corev1.PodRunning), "status", "phase")
		Expect(err).NotTo(HaveOccurred())

		Expect(cl.Status().Update(context.Background(), u)).To(Succeed())

		actual := &corev1.Pod{}
		Expect(cl.Get(context.Background(), client.ObjectKeyFromObject(obj), actual)).To(Succeed())
		obj.ResourceVersion = actual.ResourceVersion
		// only the status mutation should persist
		obj.Status.Phase = corev1.PodRunning
		Expect(cmp.Diff(obj, actual)).To(BeEmpty())
	})

	It("should not change the status of unstructured objects that are configured to have a status subresource on patch", func() {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion("foo/v1")
		obj.SetKind("Foo")
		obj.SetName("a-foo")
		cl := NewClientBuilder().WithStatusSubresource(obj).Build()

		Expect(cl.Create(context.Background(), obj)).To(Succeed())
		original := obj.DeepCopy()

		err := unstructured.SetNestedField(obj.Object, map[string]interface{}{"count": int64(2)}, "status")
		Expect(err).ToNot(HaveOccurred())
		Expect(cl.Patch(context.Background(), obj, client.MergeFrom(original))).To(Succeed())

		Expect(cl.Get(context.Background(), client.ObjectKeyFromObject(obj), obj)).To(Succeed())

		Expect(obj.Object["status"]).To(BeNil())

	})

	It("should not change non-status fields of unstructured objects that are configured to have a status subresource on status patch", func() {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion("foo/v1")
		obj.SetKind("Foo")
		obj.SetName("a-foo")

		err := unstructured.SetNestedField(obj.Object, "original", "spec")
		Expect(err).NotTo(HaveOccurred())

		cl := NewClientBuilder().WithStatusSubresource(obj).WithObjects(obj).Build()
		original := obj.DeepCopy()

		err = unstructured.SetNestedField(obj.Object, "from-status-update", "spec")
		Expect(err).NotTo(HaveOccurred())
		err = unstructured.SetNestedField(obj.Object, map[string]any{"state": "new"}, "status")
		Expect(err).ToNot(HaveOccurred())

		Expect(cl.Status().Patch(context.Background(), obj, client.MergeFrom(original))).To(Succeed())
		Expect(cl.Get(context.Background(), client.ObjectKeyFromObject(obj), obj)).To(Succeed())

		Expect(obj.Object["status"]).To(BeEquivalentTo(map[string]any{"state": "new"}))
		Expect(obj.Object["spec"]).To(BeEquivalentTo("original"))
	})

	It("should return not found on status update of resources that don't have a status subresource", func() {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion("foo/v1")
		obj.SetKind("Foo")
		obj.SetName("a-foo")

		cl := NewClientBuilder().WithObjects(obj).Build()

		err := cl.Status().Update(context.Background(), obj)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	evictionTypes := []client.Object{
		&policyv1beta1.Eviction{},
		&policyv1.Eviction{},
	}
	for _, tp := range evictionTypes {
		It("should delete a pod through the eviction subresource", func() {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo"}}

			cl := NewClientBuilder().WithObjects(pod).Build()

			err := cl.SubResource("eviction").Create(context.Background(), pod, tp)
			Expect(err).NotTo(HaveOccurred())

			err = cl.Get(context.Background(), client.ObjectKeyFromObject(pod), pod)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should return not found when attempting to evict a pod that doesn't exist", func() {
			cl := NewClientBuilder().Build()

			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo"}}
			err := cl.SubResource("eviction").Create(context.Background(), pod, tp)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should return not found when attempting to evict something other than a pod", func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "foo"}}
			cl := NewClientBuilder().WithObjects(ns).Build()

			err := cl.SubResource("eviction").Create(context.Background(), ns, tp)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should return an error when using the wrong subresource", func() {
			cl := NewClientBuilder().Build()

			err := cl.SubResource("eviction-subresource").Create(context.Background(), &corev1.Namespace{}, tp)
			Expect(err).To(HaveOccurred())
		})
	}

	It("should error when creating an eviction with the wrong type", func() {
		cl := NewClientBuilder().Build()
		err := cl.SubResource("eviction").Create(context.Background(), &corev1.Pod{}, &corev1.Namespace{})
		Expect(apierrors.IsBadRequest(err)).To(BeTrue())
	})

	It("should create a ServiceAccount token through the token subresource", func() {
		sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "foo"}}
		cl := NewClientBuilder().WithObjects(sa).Build()

		tokenRequest := &authenticationv1.TokenRequest{}
		err := cl.SubResource("token").Create(context.Background(), sa, tokenRequest)
		Expect(err).NotTo(HaveOccurred())

		Expect(tokenRequest.Status.Token).NotTo(Equal(""))
		Expect(tokenRequest.Status.ExpirationTimestamp).NotTo(Equal(metav1.Time{}))
	})

	It("should return not found when creating a token for a ServiceAccount that doesn't exist", func() {
		sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "foo"}}
		cl := NewClientBuilder().Build()

		err := cl.SubResource("token").Create(context.Background(), sa, &authenticationv1.TokenRequest{})
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("should error when creating a token with the wrong subresource type", func() {
		cl := NewClientBuilder().Build()
		err := cl.SubResource("token").Create(context.Background(), &corev1.ServiceAccount{}, &corev1.Namespace{})
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsBadRequest(err)).To(BeTrue())
	})

	It("should error when creating a token with the wrong type", func() {
		cl := NewClientBuilder().Build()
		err := cl.SubResource("token").Create(context.Background(), &corev1.Secret{}, &authenticationv1.TokenRequest{})
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("should leave typemeta empty on typed get", func() {
		cl := NewClientBuilder().WithObjects(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "foo",
		}}).Build()

		var pod corev1.Pod
		Expect(cl.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "foo"}, &pod)).NotTo(HaveOccurred())

		Expect(pod.TypeMeta).To(Equal(metav1.TypeMeta{}))
	})

	It("should leave typemeta empty on typed list", func() {
		cl := NewClientBuilder().WithObjects(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "foo",
		}}).Build()

		var podList corev1.PodList
		Expect(cl.List(context.Background(), &podList)).NotTo(HaveOccurred())
		Expect(podList.ListMeta).To(Equal(metav1.ListMeta{}))
		Expect(podList.Items[0].TypeMeta).To(Equal(metav1.TypeMeta{}))
	})

	It("should be able to Get an object that has pointer fields for metadata", func() {
		schemeBuilder := &scheme.Builder{GroupVersion: schema.GroupVersion{Group: "test", Version: "v1"}}
		schemeBuilder.Register(&WithPointerMeta{}, &WithPointerMetaList{})
		scheme := runtime.NewScheme()
		Expect(schemeBuilder.AddToScheme(scheme)).NotTo(HaveOccurred())

		cl := NewClientBuilder().
			WithScheme(scheme).
			WithObjects(&WithPointerMeta{ObjectMeta: &metav1.ObjectMeta{
				Name: "foo",
			}}).
			Build()

		var object WithPointerMeta
		Expect(cl.Get(context.Background(), client.ObjectKey{Name: "foo"}, &object)).NotTo(HaveOccurred())
	})

	It("should be able to List an object type that has pointer fields for metadata", func() {
		schemeBuilder := &scheme.Builder{GroupVersion: schema.GroupVersion{Group: "test", Version: "v1"}}
		schemeBuilder.Register(&WithPointerMeta{}, &WithPointerMetaList{})
		scheme := runtime.NewScheme()
		Expect(schemeBuilder.AddToScheme(scheme)).NotTo(HaveOccurred())

		cl := NewClientBuilder().
			WithScheme(scheme).
			WithObjects(&WithPointerMeta{ObjectMeta: &metav1.ObjectMeta{
				Name: "foo",
			}}).
			Build()

		var objectList WithPointerMetaList
		Expect(cl.List(context.Background(), &objectList)).NotTo(HaveOccurred())
		Expect(objectList.Items).To(HaveLen(1))
	})

	It("should be able to List an object type that has pointer fields for metadata with no results", func() {
		schemeBuilder := &scheme.Builder{GroupVersion: schema.GroupVersion{Group: "test", Version: "v1"}}
		schemeBuilder.Register(&WithPointerMeta{}, &WithPointerMetaList{})
		scheme := runtime.NewScheme()
		Expect(schemeBuilder.AddToScheme(scheme)).NotTo(HaveOccurred())

		cl := NewClientBuilder().
			WithScheme(scheme).
			Build()

		var objectList WithPointerMetaList
		Expect(cl.List(context.Background(), &objectList)).NotTo(HaveOccurred())
		Expect(objectList.Items).To(BeEmpty())
	})

	It("should be able to Patch an object type that has pointer fields for metadata", func() {
		schemeBuilder := &scheme.Builder{GroupVersion: schema.GroupVersion{Group: "test", Version: "v1"}}
		schemeBuilder.Register(&WithPointerMeta{}, &WithPointerMetaList{})
		scheme := runtime.NewScheme()
		Expect(schemeBuilder.AddToScheme(scheme)).NotTo(HaveOccurred())

		obj := &WithPointerMeta{ObjectMeta: &metav1.ObjectMeta{
			Name: "foo",
		}}
		cl := NewClientBuilder().
			WithScheme(scheme).
			WithObjects(obj).
			Build()

		original := obj.DeepCopy()
		obj.Labels = map[string]string{"foo": "bar"}
		Expect(cl.Patch(context.Background(), obj, client.MergeFrom(original))).NotTo(HaveOccurred())

		Expect(cl.Get(context.Background(), client.ObjectKey{Name: "foo"}, obj)).NotTo(HaveOccurred())
		Expect(obj.Labels).To(Equal(map[string]string{"foo": "bar"}))
	})

	It("should be able to Update an object type that has pointer fields for metadata", func() {
		schemeBuilder := &scheme.Builder{GroupVersion: schema.GroupVersion{Group: "test", Version: "v1"}}
		schemeBuilder.Register(&WithPointerMeta{}, &WithPointerMetaList{})
		scheme := runtime.NewScheme()
		Expect(schemeBuilder.AddToScheme(scheme)).NotTo(HaveOccurred())

		obj := &WithPointerMeta{ObjectMeta: &metav1.ObjectMeta{
			Name: "foo",
		}}
		cl := NewClientBuilder().
			WithScheme(scheme).
			WithObjects(obj).
			Build()

		Expect(cl.Get(context.Background(), client.ObjectKey{Name: "foo"}, obj)).NotTo(HaveOccurred())

		obj.Labels = map[string]string{"foo": "bar"}
		Expect(cl.Update(context.Background(), obj)).NotTo(HaveOccurred())

		Expect(cl.Get(context.Background(), client.ObjectKey{Name: "foo"}, obj)).NotTo(HaveOccurred())
		Expect(obj.Labels).To(Equal(map[string]string{"foo": "bar"}))
	})

	It("should be able to Delete an object type that has pointer fields for metadata", func() {
		schemeBuilder := &scheme.Builder{GroupVersion: schema.GroupVersion{Group: "test", Version: "v1"}}
		schemeBuilder.Register(&WithPointerMeta{}, &WithPointerMetaList{})
		scheme := runtime.NewScheme()
		Expect(schemeBuilder.AddToScheme(scheme)).NotTo(HaveOccurred())

		obj := &WithPointerMeta{ObjectMeta: &metav1.ObjectMeta{
			Name: "foo",
		}}
		cl := NewClientBuilder().
			WithScheme(scheme).
			WithObjects(obj).
			Build()

		Expect(cl.Delete(context.Background(), obj)).NotTo(HaveOccurred())

		err := cl.Get(context.Background(), client.ObjectKey{Name: "foo"}, obj)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("disallows scale subresources on unsupported built-in types", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(apiextensions.AddToScheme(scheme)).To(Succeed())

		obj := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
			},
		}
		cl := NewClientBuilder().WithScheme(scheme).WithObjects(obj).Build()

		scale := &autoscalingv1.Scale{Spec: autoscalingv1.ScaleSpec{Replicas: 2}}
		expectedErr := "unimplemented scale subresource for resource *v1.Pod"
		Expect(cl.SubResource(subResourceScale).Get(context.Background(), obj, scale).Error()).To(Equal(expectedErr))
		Expect(cl.SubResource(subResourceScale).Update(context.Background(), obj, client.WithSubResourceBody(scale)).Error()).To(Equal(expectedErr))
	})

	It("disallows scale subresources on non-existing objects", func() {
		obj := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](2),
			},
		}
		cl := NewClientBuilder().Build()

		scale := &autoscalingv1.Scale{Spec: autoscalingv1.ScaleSpec{Replicas: 2}}
		expectedErr := "deployments.apps \"foo\" not found"
		Expect(cl.SubResource(subResourceScale).Get(context.Background(), obj, scale).Error()).To(Equal(expectedErr))
		Expect(cl.SubResource(subResourceScale).Update(context.Background(), obj, client.WithSubResourceBody(scale)).Error()).To(Equal(expectedErr))
	})

	scalableObjs := []client.Object{
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](2),
			},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
			},
			Spec: appsv1.ReplicaSetSpec{
				Replicas: ptr.To[int32](2),
			},
		},
		&corev1.ReplicationController{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
			},
			Spec: corev1.ReplicationControllerSpec{
				Replicas: ptr.To[int32](2),
			},
		},
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: ptr.To[int32](2),
			},
		},
	}
	for _, obj := range scalableObjs {
		It(fmt.Sprintf("should be able to Get scale subresources for resource %T", obj), func() {
			cl := NewClientBuilder().WithObjects(obj).Build()

			scaleActual := &autoscalingv1.Scale{}
			Expect(cl.SubResource(subResourceScale).Get(context.Background(), obj, scaleActual)).NotTo(HaveOccurred())

			scaleExpected := &autoscalingv1.Scale{
				ObjectMeta: metav1.ObjectMeta{
					Name:            obj.GetName(),
					UID:             obj.GetUID(),
					ResourceVersion: obj.GetResourceVersion(),
				},
				Spec: autoscalingv1.ScaleSpec{
					Replicas: 2,
				},
			}
			Expect(cmp.Diff(scaleExpected, scaleActual)).To(BeEmpty())
		})

		It(fmt.Sprintf("should be able to Update scale subresources for resource %T", obj), func() {
			cl := NewClientBuilder().WithObjects(obj).Build()

			scaleExpected := &autoscalingv1.Scale{Spec: autoscalingv1.ScaleSpec{Replicas: 3}}
			Expect(cl.SubResource(subResourceScale).Update(context.Background(), obj, client.WithSubResourceBody(scaleExpected))).NotTo(HaveOccurred())

			objActual := obj.DeepCopyObject().(client.Object)
			Expect(cl.Get(context.Background(), client.ObjectKeyFromObject(objActual), objActual)).To(Succeed())

			objExpected := obj.DeepCopyObject().(client.Object)
			switch expected := objExpected.(type) {
			case *appsv1.Deployment:
				expected.ResourceVersion = objActual.GetResourceVersion()
				expected.Spec.Replicas = ptr.To(int32(3))
			case *appsv1.ReplicaSet:
				expected.ResourceVersion = objActual.GetResourceVersion()
				expected.Spec.Replicas = ptr.To(int32(3))
			case *corev1.ReplicationController:
				expected.ResourceVersion = objActual.GetResourceVersion()
				expected.Spec.Replicas = ptr.To(int32(3))
			case *appsv1.StatefulSet:
				expected.ResourceVersion = objActual.GetResourceVersion()
				expected.Spec.Replicas = ptr.To(int32(3))
			}
			Expect(cmp.Diff(objExpected, objActual)).To(BeEmpty())

			scaleActual := &autoscalingv1.Scale{}
			Expect(cl.SubResource(subResourceScale).Get(context.Background(), obj, scaleActual)).NotTo(HaveOccurred())

			// When we called Update, these were derived but we need them now to compare.
			scaleExpected.Name = scaleActual.Name
			scaleExpected.ResourceVersion = scaleActual.ResourceVersion
			Expect(cmp.Diff(scaleExpected, scaleActual)).To(BeEmpty())
		})
	}
})

type WithPointerMetaList struct {
	*metav1.ListMeta
	*metav1.TypeMeta
	Items []*WithPointerMeta
}

func (t *WithPointerMetaList) DeepCopy() *WithPointerMetaList {
	l := &WithPointerMetaList{
		ListMeta: t.ListMeta.DeepCopy(),
	}
	if t.TypeMeta != nil {
		l.TypeMeta = &metav1.TypeMeta{
			APIVersion: t.APIVersion,
			Kind:       t.Kind,
		}
	}
	for _, item := range t.Items {
		l.Items = append(l.Items, item.DeepCopy())
	}

	return l
}

func (t *WithPointerMetaList) DeepCopyObject() runtime.Object {
	return t.DeepCopy()
}

type WithPointerMeta struct {
	*metav1.TypeMeta
	*metav1.ObjectMeta
}

func (t *WithPointerMeta) DeepCopy() *WithPointerMeta {
	w := &WithPointerMeta{
		ObjectMeta: t.ObjectMeta.DeepCopy(),
	}
	if t.TypeMeta != nil {
		w.TypeMeta = &metav1.TypeMeta{
			APIVersion: t.APIVersion,
			Kind:       t.Kind,
		}
	}

	return w
}

func (t *WithPointerMeta) DeepCopyObject() runtime.Object {
	return t.DeepCopy()
}

var _ = Describe("Fake client builder", func() {
	It("panics when an index with the same name and GroupVersionKind is registered twice", func() {
		// We need any realistic GroupVersionKind, the choice of apps/v1 Deployment is arbitrary.
		cb := NewClientBuilder().WithIndex(&appsv1.Deployment{},
			"test-name",
			func(client.Object) []string { return nil })

		Expect(func() {
			cb.WithIndex(&appsv1.Deployment{},
				"test-name",
				func(client.Object) []string { return []string{"foo"} })
		}).To(Panic())
	})

	It("should wrap the fake client with an interceptor when WithInterceptorFuncs is called", func() {
		var called bool
		cli := NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				called = true
				return nil
			},
		}).Build()
		err := cli.Get(context.Background(), client.ObjectKey{}, &corev1.Pod{})
		Expect(err).NotTo(HaveOccurred())
		Expect(called).To(BeTrue())
	})
})
