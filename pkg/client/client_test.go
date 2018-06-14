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

package client_test

import (
	"context"

	"fmt"

	"sync/atomic"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Client", func() {
	var dep *appsv1.Deployment
	var node *corev1.Node
	var count uint64 = 0
	var ns = "default"

	BeforeEach(func(done Done) {
		atomic.AddUint64(&count, 1)
		dep = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("deployment-name-%v", count), Namespace: ns},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"foo": "bar"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "bar"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
				},
			},
		}
		node = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("node-name-%v", count)},
			Spec:       corev1.NodeSpec{},
		}
		close(done)
	})

	var delOptions *metav1.DeleteOptions
	AfterEach(func(done Done) {
		// Cleanup
		var zero int64 = 0
		policy := metav1.DeletePropagationForeground
		delOptions = &metav1.DeleteOptions{
			GracePeriodSeconds: &zero,
			PropagationPolicy:  &policy,
		}
		_, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
		if err == nil {
			err = clientset.AppsV1().Deployments(ns).Delete(dep.Name, &metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
		}
		_, err = clientset.CoreV1().Nodes().Get(node.Name, metav1.GetOptions{})
		if err == nil {
			err = clientset.CoreV1().Nodes().Delete(node.Name, delOptions)
			Expect(err).NotTo(HaveOccurred())
		}
		close(done)
	})

	Describe("New", func() {
		// TODO(seans3): implement these

		It("should return a new Client", func() {

		})

		It("should return an error if Config is not specified", func() {

		})

		It("use the provided Scheme if provided", func() {

		})

		It("default the Scheme if not provided", func() {

		})

		It("use the provided Mapper if provided", func() {

		})

		It("create a Mapper if not provided", func() {

		})

		It("should fail if the config is nil", func() {
			cl, err := client.New(nil, client.Options{})
			Expect(err).To(HaveOccurred())
			Expect(cl).To(BeNil())
		})
	})

	Describe("Create", func() {
		It("should create a new object from a go struct", func(done Done) {
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			By("creating the object")
			err = cl.Create(context.TODO(), dep)
			Expect(err).NotTo(HaveOccurred())

			actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).NotTo(BeNil())

			By("writing the result back to the go struct")
			Expect(dep).To(Equal(actual))

			close(done)
		})

		It("should create a new object from an unstructured object", func() {
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			By("creating the object")
			// TODO(seans): write this

			By("writing the result back to the go struct")
			// TODO(seans): write this
		})

		It("should create a new object non-namespace object from a go struct", func(done Done) {
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			By("creating the object")
			err = cl.Create(context.TODO(), node)
			Expect(err).NotTo(HaveOccurred())

			actual, err := clientset.CoreV1().Nodes().Get(node.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).NotTo(BeNil())

			By("writing the result back to the go struct")
			Expect(node).To(Equal(actual))

			close(done)
		})

		It("should fail if it cannot get a client", func() {
			// TODO(seans3): implement these
		})

		It("should fail if the object already exists", func(done Done) {
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			old := dep.DeepCopy()

			By("creating the object")
			err = cl.Create(context.TODO(), dep)
			Expect(err).NotTo(HaveOccurred())

			actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).NotTo(BeNil())

			By("creating the object a second time")
			err = cl.Create(context.TODO(), old)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already exists"))

			close(done)
		})

		It("should fail if the object does not pass server-side validation", func() {
			// TODO(seans3): implement these
		})

		It("should fail if the object cannot be mapped to a GVK", func() {
			// TODO(seans3): implement these
		})

		It("should fail if the GVK cannot be mapped to a Resource", func() {
			// TODO(seans3): implement these
		})
	})

	Describe("Update", func() {
		It("should update an existing object from a go struct", func(done Done) {
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
			Expect(err).NotTo(HaveOccurred())

			By("updating the object")
			dep.Annotations = map[string]string{"foo": "bar"}
			err = cl.Update(context.TODO(), dep)
			Expect(err).NotTo(HaveOccurred())

			By("writing the result back to the go struct")
			actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).NotTo(BeNil())

			close(done)
		})

		It("should update an existing new object from an unstructured object", func() {
			// TODO(seans3): implement these
		})

		It("should update an existing object non-namespace object from a go struct", func(done Done) {
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			node, err := clientset.CoreV1().Nodes().Create(node)
			Expect(err).NotTo(HaveOccurred())

			By("updating the object")
			node.Annotations = map[string]string{"foo": "bar"}
			err = cl.Update(context.TODO(), node)
			Expect(err).NotTo(HaveOccurred())

			By("writing the result back to the go struct")
			actual, err := clientset.CoreV1().Nodes().Get(node.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).NotTo(BeNil())

			close(done)
		})

		// TODO(seans3): implement these
		It("should fail if it cannot get a client", func() {

		})

		It("should fail if the object does not exists", func() {

		})

		It("should fail if the object does not pass server-side validation", func() {

		})

		It("should fail if the object doesn't have meta", func() {

		})

		It("should fail if the object cannot be mapped to a GVK", func() {

		})

		It("should fail if the GVK cannot be mapped to a Resource", func() {

		})
	})

	Describe("Delete", func() {
		// TODO(seans3): implement these
		It("should delete an existing object from a go struct", func() {

		})

		It("should update an existing new object from an unstructured object", func() {

		})

		It("should delete an existing object non-namespace object from a go struct", func() {

		})

		It("should fail if it cannot get a client", func() {

		})

		It("should fail if the object does not exists", func() {

		})

		It("should fail if the object doesn't have meta", func() {

		})

		It("should fail if the object cannot be mapped to a GVK", func() {

		})

		It("should fail if the GVK cannot be mapped to a Resource", func() {

		})
	})

	Describe("Get", func() {
		// TODO(seans3): implement these
		It("should fetch an existing object for a go struct", func() {
		})

		It("should fetch an existing object for an unstructured", func() {

		})

		It("should fetch an existing non-namespace object for a go struct", func() {

		})

		It("should fetch an existing non-namespace object for an unstructured", func() {

		})

		It("should fail if it cannot get a client", func() {

		})

		It("should fail if the object does not exists", func() {

		})

		It("should fail if the object doesn't have meta", func() {

		})

		It("should fail if the object cannot be mapped to a GVK", func() {

		})

		It("should fail if the GVK cannot be mapped to a Resource", func() {

		})
	})

	Describe("List", func() {
		It("should fetch collection of objects", func() {

		})

		It("should return an empty list if there are no matching objects", func() {

		})

		It("should filter results by label selector", func() {

		})

		It("should filter results by namespace selector", func() {

		})

		It("should filter results by field selector", func() {

		})

		It("should fail if it cannot get a client", func() {

		})

		It("should fail if the object doesn't have meta", func() {

		})

		It("should fail if the object cannot be mapped to a GVK", func() {

		})

		It("should fail if the GVK cannot be mapped to a Resource", func() {

		})
	})

	Describe("ListOptions", func() {
		// TODO(seans3): implement these
		It("should be able to set a LabelSelector", func() {

		})

		It("should be able to set a FieldSelector", func() {

		})

		It("should be converted to metav1.ListOptions", func() {

		})

		It("should be able to set MatchingLabels", func() {

		})

		It("should be able to set MatchingField", func() {

		})

		It("should be able to set InNamespace", func() {

		})

		It("should be created from MatchingLabels", func() {

		})

		It("should be created from MatchingField", func() {

		})

		It("should be created from InNamespace", func() {

		})
	})
})
