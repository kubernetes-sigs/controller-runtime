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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Fake client", func() {
	var dep *appsv1.Deployment
	var cm *corev1.ConfigMap
	var cl client.Client

	BeforeEach(func(done Done) {
		dep = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deployment",
				Namespace: "ns1",
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
		cl = NewFakeClient(dep, cm)
		close(done)
	})

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
		list := &metav1.List{}
		err := cl.List(nil, &client.ListOptions{
			Namespace: "ns1",
			Raw: &metav1.ListOptions{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
				},
			},
		}, list)
		Expect(err).To(BeNil())
		Expect(list.Items).To(HaveLen(1))
		j, err := json.Marshal(dep)
		Expect(err).To(BeNil())
		expectedDep := runtime.RawExtension{Raw: j}
		Expect(list.Items).To(ConsistOf(expectedDep))
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
		list := &metav1.List{}
		err = cl.List(nil, &client.ListOptions{
			Namespace: "ns1",
			Raw: &metav1.ListOptions{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
				},
			},
		}, list)
		Expect(err).To(BeNil())
		Expect(list.Items).To(HaveLen(0))
	})
})
