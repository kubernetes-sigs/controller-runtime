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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kscheme "k8s.io/client-go/kubernetes/scheme"
)

const serverSideTimeoutSeconds = 10

func deleteDeployment(dep *appsv1.Deployment, ns string) {
	_, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
	if err == nil {
		err = clientset.AppsV1().Deployments(ns).Delete(dep.Name, &metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())
	}
}

func deleteNamespace(ns *corev1.Namespace) {
	_, err := clientset.CoreV1().Namespaces().Get(ns.Name, metav1.GetOptions{})
	if err == nil {
		err = clientset.CoreV1().Namespaces().Delete(ns.Name, &metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())
	}
}

var _ = Describe("Client", func() {

	var scheme *runtime.Scheme
	var dep *appsv1.Deployment
	var pod *corev1.Pod
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
		// Pod is invalid without a container field in the PodSpec
		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("pod-%v", count), Namespace: ns},
			Spec:       corev1.PodSpec{},
		}
		node = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("node-name-%v", count)},
			Spec:       corev1.NodeSpec{},
		}
		scheme = kscheme.Scheme

		close(done)
	}, serverSideTimeoutSeconds)

	var delOptions *metav1.DeleteOptions
	AfterEach(func(done Done) {
		// Cleanup
		var zero int64 = 0
		policy := metav1.DeletePropagationForeground
		delOptions = &metav1.DeleteOptions{
			GracePeriodSeconds: &zero,
			PropagationPolicy:  &policy,
		}
		deleteDeployment(dep, ns)
		_, err := clientset.CoreV1().Nodes().Get(node.Name, metav1.GetOptions{})
		if err == nil {
			err = clientset.CoreV1().Nodes().Delete(node.Name, delOptions)
			Expect(err).NotTo(HaveOccurred())
		}
		close(done)
	}, serverSideTimeoutSeconds)

	// TODO(seans): Cast "cl" as "client" struct from "Client" interface. Then validate the
	// instance values for the "client" struct.
	Describe("New", func() {
		It("should return a new Client", func(done Done) {
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			close(done)
		})

		It("should fail if the config is nil", func(done Done) {
			cl, err := client.New(nil, client.Options{})
			Expect(err).To(HaveOccurred())
			Expect(cl).To(BeNil())

			close(done)
		})

		// TODO(seans): cast as client struct and inspect Scheme
		It("should use the provided Scheme if provided", func(done Done) {
			cl, err := client.New(cfg, client.Options{Scheme: scheme})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			close(done)
		})

		// TODO(seans): cast as client struct and inspect Scheme
		It("should default the Scheme if not provided", func(done Done) {
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			close(done)
		})

		It("should use the provided Mapper if provided", func() {

		})

		// TODO(seans): cast as client struct and inspect Mapper
		It("should create a Mapper if not provided", func(done Done) {
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			close(done)
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

		It("should create a new object from an unstructured object", func(done Done) {
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			By("encoding the Deployment as unstructured")
			var u runtime.Unstructured = &unstructured.Unstructured{}
			scheme.Convert(dep, u, nil)

			By("creating the unstructured Deployment")
			err = cl.Create(context.TODO(), u)
			Expect(err).NotTo(HaveOccurred())

			By("fetching newly created unstructured Deployment")
			actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).NotTo(BeNil())

			close(done)
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

		It("should fail if the object does not pass server-side validation", func(done Done) {
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			By("creating the pod, since required field Containers is empty")
			err = cl.Create(context.TODO(), pod)
			Expect(err).To(HaveOccurred())
			// TODO(seans): Add test to validate the returned error. Problems currently with
			// different returned error locally versus travis.

			close(done)
		}, serverSideTimeoutSeconds)

		It("should fail if the object cannot be mapped to a GVK", func() {
			By("creating client with empty Scheme")
			emptyScheme := runtime.NewScheme()
			cl, err := client.New(cfg, client.Options{Scheme: emptyScheme})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			By("creating the object fails")
			err = cl.Create(context.TODO(), dep)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no kind is registered for the type"))
		})

		It("should fail if the GVK cannot be mapped to a Resource", func() {
			// TODO(seans3): implement these
			// Example: ListOptions
		})
	})

	Describe("Update", func() {
		It("should update an existing object from a go struct", func(done Done) {
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			By("initially creating a Deployment")
			dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
			Expect(err).NotTo(HaveOccurred())

			By("updating the Deployment")
			dep.Annotations = map[string]string{"foo": "bar"}
			err = cl.Update(context.TODO(), dep)
			Expect(err).NotTo(HaveOccurred())

			By("validating updated Deployment has new annotation")
			actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).NotTo(BeNil())
			Expect(actual.Annotations["foo"]).To(Equal("bar"))

			close(done)
		})

		It("should update an existing new object from an unstructured object", func(done Done) {
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			By("initially creating a Deployment")
			dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
			Expect(err).NotTo(HaveOccurred())

			By("updating and encoding the Deployment as unstructured")
			var u runtime.Unstructured = &unstructured.Unstructured{}
			dep.Annotations = map[string]string{"foo": "bar"}
			scheme.Convert(dep, u, nil)

			By("updating the Deployment")
			err = cl.Update(context.TODO(), u)
			Expect(err).NotTo(HaveOccurred())

			By("fetching newly created unstructured Deployment has new annotation")
			actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).NotTo(BeNil())
			Expect(actual.Annotations["foo"]).To(Equal("bar"))

			close(done)
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

			By("validate updated Node had new annotation")
			actual, err := clientset.CoreV1().Nodes().Get(node.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).NotTo(BeNil())
			Expect(actual.Annotations["foo"]).To(Equal("bar"))

			close(done)
		})

		It("should fail if the object does not exists", func(done Done) {
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			By("updating non-existent object")
			err = cl.Update(context.TODO(), dep)
			Expect(err).To(HaveOccurred())

			close(done)
		})

		It("should fail if the object does not pass server-side validation", func() {

		})

		It("should fail if the object doesn't have meta", func() {

		})

		It("should fail if the object cannot be mapped to a GVK", func(done Done) {
			By("creating client with empty Scheme")
			emptyScheme := runtime.NewScheme()
			cl, err := client.New(cfg, client.Options{Scheme: emptyScheme})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			By("initially creating a Deployment")
			dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
			Expect(err).NotTo(HaveOccurred())

			By("updating the Deployment")
			dep.Annotations = map[string]string{"foo": "bar"}
			err = cl.Update(context.TODO(), dep)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no kind is registered for the type"))

			close(done)
		})

		It("should fail if the GVK cannot be mapped to a Resource", func() {

		})
	})

	Describe("Delete", func() {
		It("should delete an existing object from a go struct", func(done Done) {
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			By("initially creating a Deployment")
			dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
			Expect(err).NotTo(HaveOccurred())

			By("deleting the Deployment")
			depName := dep.Name
			err = cl.Delete(context.TODO(), dep)
			Expect(err).NotTo(HaveOccurred())

			By("validating the Deployment no longer exists")
			_, err = clientset.AppsV1().Deployments(ns).Get(depName, metav1.GetOptions{})
			Expect(err).To(HaveOccurred())

			close(done)
		})

		It("should delete an existing from an unstructured object", func(done Done) {
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			By("initially creating a Deployment")
			dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
			Expect(err).NotTo(HaveOccurred())

			By("encoding the Deployment as unstructured")
			var u runtime.Unstructured = &unstructured.Unstructured{}
			scheme.Convert(dep, u, nil)

			By("deleting the unstructured Deployment")
			depName := dep.Name
			err = cl.Delete(context.TODO(), u)
			Expect(err).NotTo(HaveOccurred())

			By("fetching newly created unstructured Deployment")
			_, err = clientset.AppsV1().Deployments(ns).Get(depName, metav1.GetOptions{})
			Expect(err).To(HaveOccurred())

			close(done)
		})

		It("should delete an existing object non-namespace object from a go struct", func(done Done) {
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			By("initially creating a Node")
			node, err := clientset.CoreV1().Nodes().Create(node)
			Expect(err).NotTo(HaveOccurred())

			By("deleting the Node")
			nodeName := node.Name
			err = cl.Delete(context.TODO(), node)
			Expect(err).NotTo(HaveOccurred())

			By("validating the Node no longer exists")
			_, err = clientset.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
			Expect(err).To(HaveOccurred())

			close(done)
		})

		It("should fail if the object does not exists", func(done Done) {
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			By("Deleting node before it is ever created")
			err = cl.Delete(context.TODO(), node)
			Expect(err).To(HaveOccurred())

			close(done)
		})

		It("should fail if the object doesn't have meta", func() {

		})

		It("should fail if the object cannot be mapped to a GVK", func(done Done) {
			By("creating client with empty Scheme")
			emptyScheme := runtime.NewScheme()
			cl, err := client.New(cfg, client.Options{Scheme: emptyScheme})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			By("initially creating a Deployment")
			dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
			Expect(err).NotTo(HaveOccurred())

			By("deleting the Deployment fails")
			err = cl.Delete(context.TODO(), dep)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no kind is registered for the type"))

			close(done)
		})

		It("should fail if the GVK cannot be mapped to a Resource", func() {

		})
	})

	Describe("Get", func() {
		It("should fetch an existing object for a go struct", func(done Done) {
			By("first creating the Deployment")
			dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
			Expect(err).NotTo(HaveOccurred())

			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			By("fetching the created Deployment")
			var actual appsv1.Deployment
			key := client.ObjectKey{Namespace: ns, Name: dep.Name}
			err = cl.Get(context.TODO(), key, &actual)
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).NotTo(BeNil())

			By("validating the fetched deployment equals the created one")
			Expect(dep).To(Equal(&actual))

			close(done)
		})

		It("should fetch an existing object for an unstructured", func(done Done) {
			By("first creating the Deployment")
			dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
			Expect(err).NotTo(HaveOccurred())

			By("encoding the Deployment as unstructured")
			var u runtime.Unstructured = &unstructured.Unstructured{}
			scheme.Convert(dep, u, nil)

			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			By("fetching the created Deployment")
			var actual appsv1.Deployment
			key := client.ObjectKey{Namespace: ns, Name: dep.Name}
			err = cl.Get(context.TODO(), key, &actual)
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).NotTo(BeNil())

			By("validating the fetched Deployment equals the created one")
			Expect(dep).To(Equal(&actual))

			close(done)
		})

		It("should fetch an existing non-namespace object for a go struct", func(done Done) {
			By("first creating the object")
			node, err := clientset.CoreV1().Nodes().Create(node)
			Expect(err).NotTo(HaveOccurred())

			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			By("retrieving node through client")
			var actual corev1.Node
			key := client.ObjectKey{Namespace: ns, Name: node.Name}
			err = cl.Get(context.TODO(), key, &actual)
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).NotTo(BeNil())

			Expect(node).To(Equal(&actual))

			close(done)
		})

		It("should fetch an existing non-namespace object for an unstructured", func(done Done) {
			By("first creating the Node")
			node, err := clientset.CoreV1().Nodes().Create(node)
			Expect(err).NotTo(HaveOccurred())

			By("encoding the Node as unstructured")
			var u runtime.Unstructured = &unstructured.Unstructured{}
			scheme.Convert(node, u, nil)

			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			By("fetching the created Node")
			var actual corev1.Node
			key := client.ObjectKey{Namespace: ns, Name: node.Name}
			err = cl.Get(context.TODO(), key, &actual)
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).NotTo(BeNil())

			By("validating the fetched Node equals the created one")
			Expect(node).To(Equal(&actual))

			close(done)
		})

		It("should fail if the object does not exists", func(done Done) {
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			By("fetching object that has not been created yet")
			key := client.ObjectKey{Namespace: ns, Name: dep.Name}
			var actual appsv1.Deployment
			err = cl.Get(context.TODO(), key, &actual)
			Expect(err).To(HaveOccurred())

			close(done)
		})

		It("should fail if the object doesn't have meta", func() {

		})

		It("should fail if the object cannot be mapped to a GVK", func() {
			By("first creating the Deployment")
			dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
			Expect(err).NotTo(HaveOccurred())

			By("creating a client with an empty Scheme")
			emptyScheme := runtime.NewScheme()
			cl, err := client.New(cfg, client.Options{Scheme: emptyScheme})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			By("fetching the created Deployment fails")
			var actual appsv1.Deployment
			key := client.ObjectKey{Namespace: ns, Name: dep.Name}
			err = cl.Get(context.TODO(), key, &actual)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no kind is registered for the type"))
		})

		It("should fail if the GVK cannot be mapped to a Resource", func() {

		})
	})

	Describe("List", func() {
		It("should fetch collection of objects", func(done Done) {
			By("creating an initial object")
			dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
			Expect(err).NotTo(HaveOccurred())

			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())

			By("listing all objects of that type in the cluster")
			deps := &appsv1.DeploymentList{}
			Expect(cl.List(context.Background(), nil, deps)).NotTo(HaveOccurred())

			Expect(deps.Items).NotTo(BeEmpty())
			hasDep := false
			for _, item := range deps.Items {
				if item.Name == dep.Name && item.Namespace == dep.Namespace {
					hasDep = true
					break
				}
			}
			Expect(hasDep).To(BeTrue())

			close(done)
		}, serverSideTimeoutSeconds)

		It("should return an empty list if there are no matching objects", func(done Done) {
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())

			By("listing all Deployments in the cluster")
			deps := &appsv1.DeploymentList{}
			Expect(cl.List(context.Background(), nil, deps)).NotTo(HaveOccurred())

			By("validating no Deployments are returned")
			Expect(deps.Items).To(BeEmpty())

			close(done)
		}, serverSideTimeoutSeconds)

		// TODO(seans): get label selector test working
		// It("should filter results by label selector", func(done Done) {
		// 	By("creating a Deployment with the app=frontend label")
		// 	depFrontend := &appsv1.Deployment{
		// 		ObjectMeta: metav1.ObjectMeta{Name: "deployment-frontend", Namespace: ns},
		// 		Spec: appsv1.DeploymentSpec{
		// 			Selector: &metav1.LabelSelector{
		// 				MatchLabels: map[string]string{"app": "frontend"},
		// 			},
		// 			Template: corev1.PodTemplateSpec{
		// 				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "frontend"}},
		// 				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
		// 			},
		// 		},
		// 	}
		// 	depFrontend, err := clientset.AppsV1().Deployments(ns).Create(depFrontend)
		// 	Expect(err).NotTo(HaveOccurred())

		// 	By("creating a Deployment with the app=backend label")
		// 	depBackend := &appsv1.Deployment{
		// 		ObjectMeta: metav1.ObjectMeta{Name: "deployment-backend", Namespace: ns},
		// 		Spec: appsv1.DeploymentSpec{
		// 			Selector: &metav1.LabelSelector{
		// 				MatchLabels: map[string]string{"app": "backend"},
		// 			},
		// 			Template: corev1.PodTemplateSpec{
		// 				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "backend"}},
		// 				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
		// 			},
		// 		},
		// 	}
		// 	depBackend, err = clientset.AppsV1().Deployments(ns).Create(depBackend)
		// 	Expect(err).NotTo(HaveOccurred())

		// 	cl, err := client.New(cfg, client.Options{})
		// 	Expect(err).NotTo(HaveOccurred())

		// 	By("listing all Deployments with label app=backend")
		// 	deps := &appsv1.DeploymentList{}
		// 	labels := map[string]string{"app": "backend"}
		// 	lo := client.InNamespace(ns).MatchingLabels(labels)
		// 	Expect(cl.List(context.Background(), lo, deps)).NotTo(HaveOccurred())

		// 	By("only the Deployment with the backend label is returned")
		// 	Expect(deps.Items).NotTo(BeEmpty())
		// 	Expect(1).To(Equal(len(deps.Items)))
		// 	actual := deps.Items[0]
		// 	Expect(actual.Name).To(Equal("deployment-backend"))

		// 	deleteDeployment(depFrontend, ns)
		// 	deleteDeployment(depBackend, ns)

		// 	close(done)
		// }, serverSideTimeoutSeconds)

		It("should filter results by namespace selector", func(done Done) {
			By("creating a Deployment in test-namespace-1")
			tns1 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-1"}}
			_, err := clientset.CoreV1().Namespaces().Create(tns1)
			Expect(err).NotTo(HaveOccurred())
			depFrontend := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "deployment-frontend", Namespace: "test-namespace-1"},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "frontend"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "frontend"}},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
					},
				},
			}
			depFrontend, err = clientset.AppsV1().Deployments("test-namespace-1").Create(depFrontend)
			Expect(err).NotTo(HaveOccurred())

			By("creating a Deployment in test-namespace-2")
			tns2 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-2"}}
			_, err = clientset.CoreV1().Namespaces().Create(tns2)
			Expect(err).NotTo(HaveOccurred())
			depBackend := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "deployment-backend", Namespace: "test-namespace-2"},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "backend"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "backend"}},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
					},
				},
			}
			depBackend, err = clientset.AppsV1().Deployments("test-namespace-2").Create(depBackend)
			Expect(err).NotTo(HaveOccurred())

			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())

			By("listing all Deployments in test-namespace-1")
			deps := &appsv1.DeploymentList{}
			lo := client.InNamespace("test-namespace-1")
			Expect(cl.List(context.Background(), lo, deps)).NotTo(HaveOccurred())

			By("only the Deployment in test-namespace-1 is returned")
			Expect(deps.Items).NotTo(BeEmpty())
			Expect(1).To(Equal(len(deps.Items)))
			actual := deps.Items[0]
			Expect(actual.Name).To(Equal("deployment-frontend"))

			deleteDeployment(depFrontend, "test-namespace-1")
			deleteDeployment(depBackend, "test-namespace-2")
			deleteNamespace(tns1)
			deleteNamespace(tns2)

			close(done)
		}, serverSideTimeoutSeconds)

		It("should filter results by field selector", func(done Done) {
			By("creating a Deployment with name deployment-frontend")
			depFrontend := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "deployment-frontend", Namespace: ns},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "frontend"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "frontend"}},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
					},
				},
			}
			depFrontend, err := clientset.AppsV1().Deployments(ns).Create(depFrontend)
			Expect(err).NotTo(HaveOccurred())

			By("creating a Deployment with name deployment-backend")
			depBackend := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "deployment-backend", Namespace: ns},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "backend"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "backend"}},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
					},
				},
			}
			depBackend, err = clientset.AppsV1().Deployments(ns).Create(depBackend)
			Expect(err).NotTo(HaveOccurred())

			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())

			By("listing all Deployments with field metadata.name=deployment-backend")
			deps := &appsv1.DeploymentList{}
			lo := client.MatchingField("metadata.name", "deployment-backend")
			Expect(cl.List(context.Background(), lo, deps)).NotTo(HaveOccurred())

			By("only the Deployment with the backend field is returned")
			Expect(deps.Items).NotTo(BeEmpty())
			Expect(1).To(Equal(len(deps.Items)))
			actual := deps.Items[0]
			Expect(actual.Name).To(Equal("deployment-backend"))

			deleteDeployment(depFrontend, ns)
			deleteDeployment(depBackend, ns)

			close(done)
		}, serverSideTimeoutSeconds)

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
		It("should be able to set a LabelSelector", func() {
			lo := &client.ListOptions{}
			err := lo.SetLabelSelector("foo=bar")
			Expect(err).NotTo(HaveOccurred())
			Expect(lo.LabelSelector.String()).To(Equal("foo=bar"))
		})

		It("should be able to set a FieldSelector", func() {
			lo := &client.ListOptions{}
			err := lo.SetFieldSelector("field1=bar")
			Expect(err).NotTo(HaveOccurred())
			Expect(lo.FieldSelector.String()).To(Equal("field1=bar"))
		})

		It("should be converted to metav1.ListOptions", func() {
			lo := &client.ListOptions{}
			labels := map[string]string{"foo": "bar"}
			mlo := lo.MatchingLabels(labels).
				MatchingField("field1", "bar").
				InNamespace("test-namespace").
				AsListOptions()
			Expect(mlo).NotTo(BeNil())
			Expect(mlo.LabelSelector).To(Equal("foo=bar"))
			Expect(mlo.FieldSelector).To(Equal("field1=bar"))
		})

		It("should be able to set MatchingLabels", func() {
			lo := &client.ListOptions{}
			Expect(lo.LabelSelector).To(BeNil())
			labels := map[string]string{"foo": "bar"}
			lo = lo.MatchingLabels(labels)
			Expect(lo.LabelSelector.String()).To(Equal("foo=bar"))
		})

		It("should be able to set MatchingField", func() {
			lo := &client.ListOptions{}
			Expect(lo.FieldSelector).To(BeNil())
			lo = lo.MatchingField("field1", "bar")
			Expect(lo.FieldSelector.String()).To(Equal("field1=bar"))
		})

		It("should be able to set InNamespace", func() {
			lo := &client.ListOptions{}
			lo = lo.InNamespace("test-namespace")
			Expect(lo.Namespace).To(Equal("test-namespace"))
		})

		It("should be created from MatchingLabels", func() {
			labels := map[string]string{"foo": "bar"}
			lo := client.MatchingLabels(labels)
			Expect(lo).NotTo(BeNil())
			Expect(lo.LabelSelector.String()).To(Equal("foo=bar"))
		})

		It("should be created from MatchingField", func() {
			lo := client.MatchingField("field1", "bar")
			Expect(lo).NotTo(BeNil())
			Expect(lo.FieldSelector.String()).To(Equal("field1=bar"))
		})

		It("should be created from InNamespace", func() {
			lo := client.InNamespace("test")
			Expect(lo).NotTo(BeNil())
			Expect(lo.Namespace).To(Equal("test"))
		})
	})
})
