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
	"encoding/json"
	"fmt"
	"sync/atomic"

	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	var replicaCount int32 = 2
	var ns = "default"
	var mergePatch []byte

	BeforeEach(func(done Done) {
		atomic.AddUint64(&count, 1)
		dep = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("deployment-name-%v", count), Namespace: ns},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicaCount,
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
		var err error
		mergePatch, err = json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"foo": "bar",
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

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

		PIt("should use the provided Mapper if provided", func() {

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
		Context("with structured objects", func() {
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
				Expect(apierrors.IsAlreadyExists(err)).To(BeTrue())

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

			PIt("should fail if the GVK cannot be mapped to a Resource", func() {
				// TODO(seans3): implement these
				// Example: ListOptions
			})

			Context("with the DryRun option", func() {
				It("should not create a new object", func(done Done) {
					cl, err := client.New(cfg, client.Options{})
					Expect(err).NotTo(HaveOccurred())
					Expect(cl).NotTo(BeNil())

					By("creating the object (with DryRun)")
					err = cl.Create(context.TODO(), dep, client.CreateDryRunAll)
					Expect(err).NotTo(HaveOccurred())

					actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
					Expect(err).To(HaveOccurred())
					Expect(apierrors.IsNotFound(err)).To(BeTrue())
					Expect(actual).To(Equal(&appsv1.Deployment{}))

					close(done)
				})
			})
		})

		Context("with unstructured objects", func() {
			It("should create a new object from a go struct", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("encoding the deployment as unstructured")
				u := &unstructured.Unstructured{}
				Expect(scheme.Convert(dep, u, nil)).To(Succeed())
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})

				By("creating the object")
				err = cl.Create(context.TODO(), u)
				Expect(err).NotTo(HaveOccurred())

				actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				close(done)
			})

			It("should create a new non-namespace object ", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("encoding the deployment as unstructured")
				u := &unstructured.Unstructured{}
				Expect(scheme.Convert(node, u, nil)).To(Succeed())
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Kind:    "Node",
					Version: "v1",
				})

				By("creating the object")
				err = cl.Create(context.TODO(), node)
				Expect(err).NotTo(HaveOccurred())

				actual, err := clientset.CoreV1().Nodes().Get(node.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				au := &unstructured.Unstructured{}
				Expect(scheme.Convert(actual, au, nil)).To(Succeed())
				Expect(scheme.Convert(node, u, nil)).To(Succeed())
				By("writing the result back to the go struct")

				Expect(u).To(Equal(au))

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

				By("encoding the deployment as unstructured")
				u := &unstructured.Unstructured{}
				Expect(scheme.Convert(old, u, nil)).To(Succeed())
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})

				By("creating the object a second time")
				err = cl.Create(context.TODO(), u)
				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsAlreadyExists(err)).To(BeTrue())

				close(done)
			})

			It("should fail if the object does not pass server-side validation", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("creating the pod, since required field Containers is empty")
				u := &unstructured.Unstructured{}
				Expect(scheme.Convert(pod, u, nil)).To(Succeed())
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Pod",
				})
				err = cl.Create(context.TODO(), u)
				Expect(err).To(HaveOccurred())
				// TODO(seans): Add test to validate the returned error. Problems currently with
				// different returned error locally versus travis.

				close(done)
			}, serverSideTimeoutSeconds)

		})

		Context("with the DryRun option", func() {
			It("should not create a new object from a go struct", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("encoding the deployment as unstructured")
				u := &unstructured.Unstructured{}
				Expect(scheme.Convert(dep, u, nil)).To(Succeed())
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})

				By("creating the object")
				err = cl.Create(context.TODO(), u, client.CreateDryRunAll)
				Expect(err).NotTo(HaveOccurred())

				actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
				Expect(actual).To(Equal(&appsv1.Deployment{}))

				close(done)
			})
		})
	})

	Describe("Update", func() {
		Context("with structured objects", func() {
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

			PIt("should fail if the object does not pass server-side validation", func() {

			})

			PIt("should fail if the object doesn't have meta", func() {

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

			PIt("should fail if the GVK cannot be mapped to a Resource", func() {

			})
		})
		Context("with unstructured objects", func() {
			It("should update an existing object from a go struct", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				By("updating the Deployment")
				u := &unstructured.Unstructured{}
				Expect(scheme.Convert(dep, u, nil)).To(Succeed())
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})
				u.SetAnnotations(map[string]string{"foo": "bar"})
				err = cl.Update(context.TODO(), u)
				Expect(err).NotTo(HaveOccurred())

				By("validating updated Deployment has new annotation")
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
				u := &unstructured.Unstructured{}
				Expect(scheme.Convert(node, u, nil)).To(Succeed())
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Kind:    "Node",
					Version: "v1",
				})
				u.SetAnnotations(map[string]string{"foo": "bar"})
				err = cl.Update(context.TODO(), u)
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
				u := &unstructured.Unstructured{}
				Expect(scheme.Convert(dep, u, nil)).To(Succeed())
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})
				err = cl.Update(context.TODO(), dep)
				Expect(err).To(HaveOccurred())

				close(done)
			})
		})
	})

	Describe("StatusClient", func() {
		Context("with structured objects", func() {
			It("should update status of an existing object", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				By("updating the status of Deployment")
				dep.Status.Replicas = 1
				err = cl.Status().Update(context.TODO(), dep)
				Expect(err).NotTo(HaveOccurred())

				By("validating updated Deployment has new status")
				actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Status.Replicas).To(BeEquivalentTo(1))

				close(done)
			})

			It("should not update spec of an existing object", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				By("updating the spec and status of Deployment")
				var rc int32 = 1
				dep.Status.Replicas = 1
				dep.Spec.Replicas = &rc
				err = cl.Status().Update(context.TODO(), dep)
				Expect(err).NotTo(HaveOccurred())

				By("validating updated Deployment has new status and unchanged spec")
				actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Status.Replicas).To(BeEquivalentTo(1))
				Expect(*actual.Spec.Replicas).To(BeEquivalentTo(replicaCount))

				close(done)
			})

			It("should update an existing object non-namespace object", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				node, err := clientset.CoreV1().Nodes().Create(node)
				Expect(err).NotTo(HaveOccurred())

				By("updating status of the object")
				node.Status.Phase = corev1.NodeRunning
				err = cl.Status().Update(context.TODO(), node)
				Expect(err).NotTo(HaveOccurred())

				By("validate updated Node had new annotation")
				actual, err := clientset.CoreV1().Nodes().Get(node.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Status.Phase).To(Equal(corev1.NodeRunning))

				close(done)
			})

			It("should fail if the object does not exists", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("updating status of a non-existent object")
				err = cl.Status().Update(context.TODO(), dep)
				Expect(err).To(HaveOccurred())

				close(done)
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

				By("updating status of the Deployment")
				dep.Status.Replicas = 1
				err = cl.Status().Update(context.TODO(), dep)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no kind is registered for the type"))

				close(done)
			})

			PIt("should fail if the GVK cannot be mapped to a Resource", func() {

			})

			PIt("should fail if an API does not implement Status subresource", func() {

			})
		})

		Context("with unstructured objects", func() {
			It("should update status of an existing object", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				By("updating the status of Deployment")
				u := &unstructured.Unstructured{}
				dep.Status.Replicas = 1
				Expect(scheme.Convert(dep, u, nil)).To(Succeed())
				err = cl.Status().Update(context.TODO(), u)
				Expect(err).NotTo(HaveOccurred())

				By("validating updated Deployment has new status")
				actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Status.Replicas).To(BeEquivalentTo(1))

				close(done)
			})

			It("should not update spec of an existing object", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				By("updating the spec and status of Deployment")
				u := &unstructured.Unstructured{}
				var rc int32 = 1
				dep.Status.Replicas = 1
				dep.Spec.Replicas = &rc
				Expect(scheme.Convert(dep, u, nil)).To(Succeed())
				err = cl.Status().Update(context.TODO(), u)
				Expect(err).NotTo(HaveOccurred())

				By("validating updated Deployment has new status and unchanged spec")
				actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Status.Replicas).To(BeEquivalentTo(1))
				Expect(*actual.Spec.Replicas).To(BeEquivalentTo(replicaCount))

				close(done)
			})

			It("should update an existing object non-namespace object", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				node, err := clientset.CoreV1().Nodes().Create(node)
				Expect(err).NotTo(HaveOccurred())

				By("updating status of the object")
				u := &unstructured.Unstructured{}
				node.Status.Phase = corev1.NodeRunning
				Expect(scheme.Convert(node, u, nil)).To(Succeed())
				err = cl.Status().Update(context.TODO(), u)
				Expect(err).NotTo(HaveOccurred())

				By("validate updated Node had new annotation")
				actual, err := clientset.CoreV1().Nodes().Get(node.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Status.Phase).To(Equal(corev1.NodeRunning))

				close(done)
			})

			It("should fail if the object does not exists", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("updating status of a non-existent object")
				u := &unstructured.Unstructured{}
				Expect(scheme.Convert(dep, u, nil)).To(Succeed())
				err = cl.Status().Update(context.TODO(), u)
				Expect(err).To(HaveOccurred())

				close(done)
			})

			PIt("should fail if the GVK cannot be mapped to a Resource", func() {

			})

			PIt("should fail if an API does not implement Status subresource", func() {

			})

		})
	})

	Describe("Delete", func() {
		Context("with structured objects", func() {
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

			PIt("should fail if the object doesn't have meta", func() {

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

			PIt("should fail if the GVK cannot be mapped to a Resource", func() {

			})
		})
		Context("with unstructured objects", func() {
			It("should delete an existing object from a go struct", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				By("deleting the Deployment")
				depName := dep.Name
				u := &unstructured.Unstructured{}
				Expect(scheme.Convert(dep, u, nil)).To(Succeed())
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})
				err = cl.Delete(context.TODO(), u)
				Expect(err).NotTo(HaveOccurred())

				By("validating the Deployment no longer exists")
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
				u := &unstructured.Unstructured{}
				Expect(scheme.Convert(node, u, nil)).To(Succeed())
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Kind:    "Node",
					Version: "v1",
				})
				err = cl.Delete(context.TODO(), u)
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
				u := &unstructured.Unstructured{}
				Expect(scheme.Convert(node, u, nil)).To(Succeed())
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Kind:    "Node",
					Version: "v1",
				})
				err = cl.Delete(context.TODO(), node)
				Expect(err).To(HaveOccurred())

				close(done)
			})
		})
	})

	Describe("Patch", func() {
		Context("with structured objects", func() {
			It("should patch an existing object from a go struct", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				By("patching the Deployment")
				err = cl.Patch(context.TODO(), dep, client.ConstantPatch(types.MergePatchType, mergePatch))
				Expect(err).NotTo(HaveOccurred())

				By("validating patched Deployment has new annotation")
				actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Annotations["foo"]).To(Equal("bar"))

				close(done)
			})

			It("should patch an existing object non-namespace object from a go struct", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Node")
				node, err := clientset.CoreV1().Nodes().Create(node)
				Expect(err).NotTo(HaveOccurred())

				By("patching the Node")
				nodeName := node.Name
				err = cl.Patch(context.TODO(), node, client.ConstantPatch(types.MergePatchType, mergePatch))
				Expect(err).NotTo(HaveOccurred())

				By("validating the Node no longer exists")
				actual, err := clientset.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Annotations["foo"]).To(Equal("bar"))

				close(done)
			})

			It("should fail if the object does not exists", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("Patching node before it is ever created")
				err = cl.Patch(context.TODO(), node, client.ConstantPatch(types.MergePatchType, mergePatch))
				Expect(err).To(HaveOccurred())

				close(done)
			})

			PIt("should fail if the object doesn't have meta", func() {

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

				By("patching the Deployment fails")
				err = cl.Patch(context.TODO(), dep, client.ConstantPatch(types.MergePatchType, mergePatch))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no kind is registered for the type"))

				close(done)
			})

			PIt("should fail if the GVK cannot be mapped to a Resource", func() {

			})

			It("should respect passed in update options", func() {
				By("creating a new client")
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				By("patching the Deployment with dry-run")
				err = cl.Patch(context.TODO(), dep, client.ConstantPatch(types.MergePatchType, mergePatch), client.PatchDryRunAll)
				Expect(err).NotTo(HaveOccurred())

				By("validating patched Deployment doesn't have the new annotation")
				actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Annotations).NotTo(HaveKey("foo"))
			})
		})
		Context("with unstructured objects", func() {
			It("should patch an existing object from a go struct", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				By("patching the Deployment")
				depName := dep.Name
				u := &unstructured.Unstructured{}
				Expect(scheme.Convert(dep, u, nil)).To(Succeed())
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})
				err = cl.Patch(context.TODO(), u, client.ConstantPatch(types.MergePatchType, mergePatch))
				Expect(err).NotTo(HaveOccurred())

				By("validating patched Deployment has new annotation")
				actual, err := clientset.AppsV1().Deployments(ns).Get(depName, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Annotations["foo"]).To(Equal("bar"))

				close(done)
			})

			It("should patch an existing object non-namespace object from a go struct", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Node")
				node, err := clientset.CoreV1().Nodes().Create(node)
				Expect(err).NotTo(HaveOccurred())

				By("patching the Node")
				nodeName := node.Name
				u := &unstructured.Unstructured{}
				Expect(scheme.Convert(node, u, nil)).To(Succeed())
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Kind:    "Node",
					Version: "v1",
				})
				err = cl.Patch(context.TODO(), u, client.ConstantPatch(types.MergePatchType, mergePatch))
				Expect(err).NotTo(HaveOccurred())

				By("validating pathed Node has new annotation")
				actual, err := clientset.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Annotations["foo"]).To(Equal("bar"))

				close(done)
			})

			It("should fail if the object does not exist", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("Patching node before it is ever created")
				u := &unstructured.Unstructured{}
				Expect(scheme.Convert(node, u, nil)).To(Succeed())
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Kind:    "Node",
					Version: "v1",
				})
				err = cl.Patch(context.TODO(), node, client.ConstantPatch(types.MergePatchType, mergePatch))
				Expect(err).To(HaveOccurred())

				close(done)
			})

			It("should respect passed-in update options", func() {
				By("creating a new client")
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				By("patching the Deployment")
				depName := dep.Name
				u := &unstructured.Unstructured{}
				Expect(scheme.Convert(dep, u, nil)).To(Succeed())
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})
				err = cl.Patch(context.TODO(), u, client.ConstantPatch(types.MergePatchType, mergePatch), client.PatchDryRunAll)
				Expect(err).NotTo(HaveOccurred())

				By("validating patched Deployment does not have the new annotation")
				actual, err := clientset.AppsV1().Deployments(ns).Get(depName, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Annotations).NotTo(HaveKey("foo"))
			})
		})
	})

	Describe("Get", func() {
		Context("with structured objects", func() {
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

			PIt("should fail if the object doesn't have meta", func() {

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

			PIt("should fail if the GVK cannot be mapped to a Resource", func() {

			})
		})

		Context("with unstructured objects", func() {
			It("should fetch an existing object", func(done Done) {
				By("first creating the Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("encoding the Deployment as unstructured")
				var u runtime.Unstructured = &unstructured.Unstructured{}
				Expect(scheme.Convert(dep, u, nil)).To(Succeed())

				By("fetching the created Deployment")
				var actual unstructured.Unstructured
				actual.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})
				key := client.ObjectKey{Namespace: ns, Name: dep.Name}
				err = cl.Get(context.TODO(), key, &actual)
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())

				By("validating the fetched Deployment equals the created one")
				Expect(u).To(Equal(&actual))

				close(done)
			})

			It("should fetch an existing non-namespace object", func(done Done) {
				By("first creating the Node")
				node, err := clientset.CoreV1().Nodes().Create(node)
				Expect(err).NotTo(HaveOccurred())

				By("encoding the Node as unstructured")
				var u runtime.Unstructured = &unstructured.Unstructured{}
				Expect(scheme.Convert(node, u, nil)).To(Succeed())

				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("fetching the created Node")
				var actual unstructured.Unstructured
				actual.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Kind:    "Node",
					Version: "v1",
				})
				key := client.ObjectKey{Namespace: ns, Name: node.Name}
				err = cl.Get(context.TODO(), key, &actual)
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())

				By("validating the fetched Node equals the created one")
				Expect(u).To(Equal(&actual))

				close(done)
			})

			It("should fail if the object does not exists", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("fetching object that has not been created yet")
				key := client.ObjectKey{Namespace: ns, Name: dep.Name}
				u := &unstructured.Unstructured{}
				err = cl.Get(context.TODO(), key, u)
				Expect(err).To(HaveOccurred())

				close(done)
			})
		})
	})

	Describe("List", func() {
		Context("with structured objects", func() {
			It("should fetch collection of objects", func(done Done) {
				By("creating an initial object")
				dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())

				By("listing all objects of that type in the cluster")
				deps := &appsv1.DeploymentList{}
				Expect(cl.List(context.Background(), deps)).NotTo(HaveOccurred())

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

			It("should fetch unstructured collection of objects", func(done Done) {
				By("create an initial object")
				_, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())

				By("listing all objects of that type in the cluster")
				deps := &unstructured.UnstructuredList{}
				deps.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "DeploymentList",
					Version: "v1",
				})
				err = cl.List(context.Background(), deps)
				Expect(err).NotTo(HaveOccurred())

				Expect(deps.Items).NotTo(BeEmpty())
				hasDep := false
				for _, item := range deps.Items {
					if item.GetName() == dep.Name && item.GetNamespace() == dep.Namespace {
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
				Expect(cl.List(context.Background(), deps)).NotTo(HaveOccurred())

				By("validating no Deployments are returned")
				Expect(deps.Items).To(BeEmpty())

				close(done)
			}, serverSideTimeoutSeconds)

			// TODO(seans): get label selector test working
			It("should filter results by label selector", func(done Done) {
				By("creating a Deployment with the app=frontend label")
				depFrontend := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deployment-frontend",
						Namespace: ns,
						Labels:    map[string]string{"app": "frontend"},
					},
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

				By("creating a Deployment with the app=backend label")
				depBackend := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deployment-backend",
						Namespace: ns,
						Labels:    map[string]string{"app": "backend"},
					},
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

				By("listing all Deployments with label app=backend")
				deps := &appsv1.DeploymentList{}
				labels := map[string]string{"app": "backend"}
				err = cl.List(context.Background(), deps, client.MatchingLabels(labels))
				Expect(err).NotTo(HaveOccurred())

				By("only the Deployment with the backend label is returned")
				Expect(deps.Items).NotTo(BeEmpty())
				Expect(1).To(Equal(len(deps.Items)))
				actual := deps.Items[0]
				Expect(actual.Name).To(Equal("deployment-backend"))

				deleteDeployment(depFrontend, ns)
				deleteDeployment(depBackend, ns)

				close(done)
			}, serverSideTimeoutSeconds)

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
				err = cl.List(context.Background(), deps, client.InNamespace("test-namespace-1"))
				Expect(err).NotTo(HaveOccurred())

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
				err = cl.List(context.Background(), deps,
					client.MatchingField("metadata.name", "deployment-backend"))
				Expect(err).NotTo(HaveOccurred())

				By("only the Deployment with the backend field is returned")
				Expect(deps.Items).NotTo(BeEmpty())
				Expect(1).To(Equal(len(deps.Items)))
				actual := deps.Items[0]
				Expect(actual.Name).To(Equal("deployment-backend"))

				deleteDeployment(depFrontend, ns)
				deleteDeployment(depBackend, ns)

				close(done)
			}, serverSideTimeoutSeconds)

			It("should filter results by namespace selector and label selector", func(done Done) {
				By("creating a Deployment in test-namespace-3 with the app=frontend label")
				tns3 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-3"}}
				_, err := clientset.CoreV1().Namespaces().Create(tns3)
				Expect(err).NotTo(HaveOccurred())
				depFrontend3 := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deployment-frontend",
						Namespace: "test-namespace-3",
						Labels:    map[string]string{"app": "frontend"},
					},
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
				depFrontend3, err = clientset.AppsV1().Deployments("test-namespace-3").Create(depFrontend3)
				Expect(err).NotTo(HaveOccurred())

				By("creating a Deployment in test-namespace-3 with the app=backend label")
				depBackend3 := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deployment-backend",
						Namespace: "test-namespace-3",
						Labels:    map[string]string{"app": "backend"},
					},
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
				depBackend3, err = clientset.AppsV1().Deployments("test-namespace-3").Create(depBackend3)
				Expect(err).NotTo(HaveOccurred())

				By("creating a Deployment in test-namespace-4 with the app=frontend label")
				tns4 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-4"}}
				_, err = clientset.CoreV1().Namespaces().Create(tns4)
				Expect(err).NotTo(HaveOccurred())
				depFrontend4 := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deployment-frontend",
						Namespace: "test-namespace-4",
						Labels:    map[string]string{"app": "frontend"},
					},
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
				depFrontend4, err = clientset.AppsV1().Deployments("test-namespace-4").Create(depFrontend4)
				Expect(err).NotTo(HaveOccurred())

				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())

				By("listing all Deployments in test-namespace-3 with label app=frontend")
				deps := &appsv1.DeploymentList{}
				labels := map[string]string{"app": "frontend"}
				err = cl.List(context.Background(), deps,
					client.InNamespace("test-namespace-3"),
					client.MatchingLabels(labels),
				)
				Expect(err).NotTo(HaveOccurred())

				By("only the Deployment in test-namespace-3 with label app=frontend is returned")
				Expect(deps.Items).NotTo(BeEmpty())
				Expect(1).To(Equal(len(deps.Items)))
				actual := deps.Items[0]
				Expect(actual.Name).To(Equal("deployment-frontend"))
				Expect(actual.Namespace).To(Equal("test-namespace-3"))

				deleteDeployment(depFrontend3, "test-namespace-3")
				deleteDeployment(depBackend3, "test-namespace-3")
				deleteDeployment(depFrontend4, "test-namespace-4")
				deleteNamespace(tns3)
				deleteNamespace(tns4)

				close(done)
			}, serverSideTimeoutSeconds)

			PIt("should fail if the object doesn't have meta", func() {

			})

			PIt("should fail if the object cannot be mapped to a GVK", func() {

			})

			PIt("should fail if the GVK cannot be mapped to a Resource", func() {

			})
		})

		Context("with unstructured objects", func() {
			It("should fetch collection of objects", func(done Done) {
				By("create an initial object")
				_, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())

				By("listing all objects of that type in the cluster")
				deps := &unstructured.UnstructuredList{}
				deps.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "DeploymentList",
					Version: "v1",
				})
				err = cl.List(context.Background(), deps)
				Expect(err).NotTo(HaveOccurred())

				Expect(deps.Items).NotTo(BeEmpty())
				hasDep := false
				for _, item := range deps.Items {
					if item.GetName() == dep.Name && item.GetNamespace() == dep.Namespace {
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
				deps := &unstructured.UnstructuredList{}
				deps.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "DeploymentList",
					Version: "v1",
				})
				Expect(cl.List(context.Background(), deps)).NotTo(HaveOccurred())

				By("validating no Deployments are returned")
				Expect(deps.Items).To(BeEmpty())

				close(done)
			}, serverSideTimeoutSeconds)

			It("should filter results by namespace selector", func(done Done) {
				By("creating a Deployment in test-namespace-5")
				tns1 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-5"}}
				_, err := clientset.CoreV1().Namespaces().Create(tns1)
				Expect(err).NotTo(HaveOccurred())
				depFrontend := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "deployment-frontend", Namespace: "test-namespace-5"},
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
				depFrontend, err = clientset.AppsV1().Deployments("test-namespace-5").Create(depFrontend)
				Expect(err).NotTo(HaveOccurred())

				By("creating a Deployment in test-namespace-6")
				tns2 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-6"}}
				_, err = clientset.CoreV1().Namespaces().Create(tns2)
				Expect(err).NotTo(HaveOccurred())
				depBackend := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "deployment-backend", Namespace: "test-namespace-6"},
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
				depBackend, err = clientset.AppsV1().Deployments("test-namespace-6").Create(depBackend)
				Expect(err).NotTo(HaveOccurred())

				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())

				By("listing all Deployments in test-namespace-5")
				deps := &unstructured.UnstructuredList{}
				deps.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "DeploymentList",
					Version: "v1",
				})
				err = cl.List(context.Background(), deps, client.InNamespace("test-namespace-5"))
				Expect(err).NotTo(HaveOccurred())

				By("only the Deployment in test-namespace-5 is returned")
				Expect(deps.Items).NotTo(BeEmpty())
				Expect(1).To(Equal(len(deps.Items)))
				actual := deps.Items[0]
				Expect(actual.GetName()).To(Equal("deployment-frontend"))

				deleteDeployment(depFrontend, "test-namespace-5")
				deleteDeployment(depBackend, "test-namespace-6")
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
				deps := &unstructured.UnstructuredList{}
				deps.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "DeploymentList",
					Version: "v1",
				})
				err = cl.List(context.Background(), deps,
					client.MatchingField("metadata.name", "deployment-backend"))
				Expect(err).NotTo(HaveOccurred())

				By("only the Deployment with the backend field is returned")
				Expect(deps.Items).NotTo(BeEmpty())
				Expect(1).To(Equal(len(deps.Items)))
				actual := deps.Items[0]
				Expect(actual.GetName()).To(Equal("deployment-backend"))

				deleteDeployment(depFrontend, ns)
				deleteDeployment(depBackend, ns)

				close(done)
			}, serverSideTimeoutSeconds)

			It("should filter results by namespace selector and label selector", func(done Done) {
				By("creating a Deployment in test-namespace-7 with the app=frontend label")
				tns3 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-7"}}
				_, err := clientset.CoreV1().Namespaces().Create(tns3)
				Expect(err).NotTo(HaveOccurred())
				depFrontend3 := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deployment-frontend",
						Namespace: "test-namespace-7",
						Labels:    map[string]string{"app": "frontend"},
					},
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
				depFrontend3, err = clientset.AppsV1().Deployments("test-namespace-7").Create(depFrontend3)
				Expect(err).NotTo(HaveOccurred())

				By("creating a Deployment in test-namespace-7 with the app=backend label")
				depBackend3 := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deployment-backend",
						Namespace: "test-namespace-7",
						Labels:    map[string]string{"app": "backend"},
					},
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
				depBackend3, err = clientset.AppsV1().Deployments("test-namespace-7").Create(depBackend3)
				Expect(err).NotTo(HaveOccurred())

				By("creating a Deployment in test-namespace-8 with the app=frontend label")
				tns4 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-8"}}
				_, err = clientset.CoreV1().Namespaces().Create(tns4)
				Expect(err).NotTo(HaveOccurred())
				depFrontend4 := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deployment-frontend",
						Namespace: "test-namespace-8",
						Labels:    map[string]string{"app": "frontend"},
					},
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
				depFrontend4, err = clientset.AppsV1().Deployments("test-namespace-8").Create(depFrontend4)
				Expect(err).NotTo(HaveOccurred())

				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())

				By("listing all Deployments in test-namespace-8 with label app=frontend")
				deps := &unstructured.UnstructuredList{}
				deps.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "DeploymentList",
					Version: "v1",
				})
				labels := map[string]string{"app": "frontend"}
				err = cl.List(context.Background(), deps,
					client.InNamespace("test-namespace-7"), client.MatchingLabels(labels))
				Expect(err).NotTo(HaveOccurred())

				By("only the Deployment in test-namespace-7 with label app=frontend is returned")
				Expect(deps.Items).NotTo(BeEmpty())
				Expect(1).To(Equal(len(deps.Items)))
				actual := deps.Items[0]
				Expect(actual.GetName()).To(Equal("deployment-frontend"))
				Expect(actual.GetNamespace()).To(Equal("test-namespace-7"))

				deleteDeployment(depFrontend3, "test-namespace-7")
				deleteDeployment(depBackend3, "test-namespace-7")
				deleteDeployment(depFrontend4, "test-namespace-8")
				deleteNamespace(tns3)
				deleteNamespace(tns4)

				close(done)
			}, serverSideTimeoutSeconds)

			PIt("should fail if the object doesn't have meta", func() {

			})
		})
	})

	Describe("CreateOptions", func() {
		It("should allow setting DryRun to 'all'", func() {
			co := &client.CreateOptions{}
			client.CreateDryRunAll(co)
			all := []string{metav1.DryRunAll}
			Expect(co.AsCreateOptions().DryRun).To(Equal(all))
		})

		It("should produce empty metav1.CreateOptions if nil", func() {
			var co *client.CreateOptions
			Expect(co.AsCreateOptions()).To(Equal(&metav1.CreateOptions{}))
			co = &client.CreateOptions{}
			Expect(co.AsCreateOptions()).To(Equal(&metav1.CreateOptions{}))
		})
	})

	Describe("DeleteOptions", func() {
		It("should allow setting GracePeriodSeconds", func() {
			do := &client.DeleteOptions{}
			client.GracePeriodSeconds(1)(do)
			gp := int64(1)
			Expect(do.AsDeleteOptions().GracePeriodSeconds).To(Equal(&gp))
		})

		It("should allow setting Precondition", func() {
			do := &client.DeleteOptions{}
			pc := metav1.NewUIDPreconditions("uid")
			client.Preconditions(pc)(do)
			Expect(do.AsDeleteOptions().Preconditions).To(Equal(pc))
			Expect(do.Preconditions).To(Equal(pc))
		})

		It("should allow setting PropagationPolicy", func() {
			do := &client.DeleteOptions{}
			client.PropagationPolicy(metav1.DeletePropagationForeground)(do)
			dp := metav1.DeletePropagationForeground
			Expect(do.AsDeleteOptions().PropagationPolicy).To(Equal(&dp))
		})

		It("should produce empty metav1.DeleteOptions if nil", func() {
			var do *client.DeleteOptions
			Expect(do.AsDeleteOptions()).To(Equal(&metav1.DeleteOptions{}))
			do = &client.DeleteOptions{}
			Expect(do.AsDeleteOptions()).To(Equal(&metav1.DeleteOptions{}))
		})

		It("should merge multiple options together", func() {
			gp := int64(1)
			pc := metav1.NewUIDPreconditions("uid")
			dp := metav1.DeletePropagationForeground
			do := &client.DeleteOptions{}
			do.ApplyOptions([]client.DeleteOptionFunc{
				client.GracePeriodSeconds(gp),
				client.Preconditions(pc),
				client.PropagationPolicy(dp),
			})
			Expect(do.GracePeriodSeconds).To(Equal(&gp))
			Expect(do.Preconditions).To(Equal(pc))
			Expect(do.PropagationPolicy).To(Equal(&dp))
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
			lo := &client.ListOptions{}
			client.MatchingLabels(labels)(lo)
			Expect(lo).NotTo(BeNil())
			Expect(lo.LabelSelector.String()).To(Equal("foo=bar"))
		})

		It("should be created from MatchingField", func() {
			lo := &client.ListOptions{}
			client.MatchingField("field1", "bar")(lo)
			Expect(lo).NotTo(BeNil())
			Expect(lo.FieldSelector.String()).To(Equal("field1=bar"))
		})

		It("should be created from InNamespace", func() {
			lo := &client.ListOptions{}
			client.InNamespace("test")(lo)
			Expect(lo).NotTo(BeNil())
			Expect(lo.Namespace).To(Equal("test"))
		})

		It("should allow pre-built ListOptions", func() {
			lo := &client.ListOptions{}
			newLo := &client.ListOptions{}
			client.UseListOptions(newLo.InNamespace("test"))(lo)
			Expect(lo).NotTo(BeNil())
			Expect(lo.Namespace).To(Equal("test"))
		})
	})

	Describe("UpdateOptions", func() {
		It("should allow setting DryRun to 'all'", func() {
			uo := &client.UpdateOptions{}
			client.UpdateDryRunAll(uo)
			all := []string{metav1.DryRunAll}
			Expect(uo.AsUpdateOptions().DryRun).To(Equal(all))
		})

		It("should produce empty metav1.UpdateOptions if nil", func() {
			var co *client.UpdateOptions
			Expect(co.AsUpdateOptions()).To(Equal(&metav1.UpdateOptions{}))
			co = &client.UpdateOptions{}
			Expect(co.AsUpdateOptions()).To(Equal(&metav1.UpdateOptions{}))
		})
	})

	Describe("PatchOptions", func() {
		It("should allow setting DryRun to 'all'", func() {
			po := &client.PatchOptions{}
			client.PatchDryRunAll(po)
			all := []string{metav1.DryRunAll}
			Expect(po.AsPatchOptions().DryRun).To(Equal(all))
		})

		It("should allow setting Force to 'true'", func() {
			po := &client.PatchOptions{}
			client.ForceOwnership(po)
			mpo := po.AsPatchOptions()
			Expect(mpo.Force).NotTo(BeNil())
			Expect(*mpo.Force).To(BeTrue())
		})

		It("should allow setting the field manager", func() {
			po := &client.PatchOptions{}
			client.FieldOwner("some-owner")(po)
			Expect(po.AsPatchOptions().FieldManager).To(Equal("some-owner"))
		})

		It("should produce empty metav1.PatchOptions if nil", func() {
			var po *client.PatchOptions
			Expect(po.AsPatchOptions()).To(Equal(&metav1.PatchOptions{}))
			po = &client.PatchOptions{}
			Expect(po.AsPatchOptions()).To(Equal(&metav1.PatchOptions{}))
		})
	})
})

var _ = Describe("DelegatingReader", func() {
	Describe("Get", func() {
		It("should call cache reader when structured object", func() {
			cachedReader := &fakeReader{}
			clientReader := &fakeReader{}
			dReader := client.DelegatingReader{
				CacheReader:  cachedReader,
				ClientReader: clientReader,
			}
			var actual appsv1.Deployment
			key := client.ObjectKey{Namespace: "ns", Name: "name"}
			Expect(dReader.Get(context.TODO(), key, &actual)).To(Succeed())
			Expect(1).To(Equal(cachedReader.Called))
			Expect(0).To(Equal(clientReader.Called))
		})
		It("should call client reader when structured object", func() {
			cachedReader := &fakeReader{}
			clientReader := &fakeReader{}
			dReader := client.DelegatingReader{
				CacheReader:  cachedReader,
				ClientReader: clientReader,
			}
			var actual unstructured.Unstructured
			key := client.ObjectKey{Namespace: "ns", Name: "name"}
			Expect(dReader.Get(context.TODO(), key, &actual)).To(Succeed())
			Expect(0).To(Equal(cachedReader.Called))
			Expect(1).To(Equal(clientReader.Called))
		})
	})
	Describe("List", func() {
		It("should call cache reader when structured object", func() {
			cachedReader := &fakeReader{}
			clientReader := &fakeReader{}
			dReader := client.DelegatingReader{
				CacheReader:  cachedReader,
				ClientReader: clientReader,
			}
			var actual appsv1.DeploymentList
			Expect(dReader.List(context.Background(), &actual)).To(Succeed())
			Expect(1).To(Equal(cachedReader.Called))
			Expect(0).To(Equal(clientReader.Called))

		})
		It("should call client reader when structured object", func() {
			cachedReader := &fakeReader{}
			clientReader := &fakeReader{}
			dReader := client.DelegatingReader{
				CacheReader:  cachedReader,
				ClientReader: clientReader,
			}

			var actual unstructured.UnstructuredList
			Expect(dReader.List(context.Background(), &actual)).To(Succeed())
			Expect(0).To(Equal(cachedReader.Called))
			Expect(1).To(Equal(clientReader.Called))

		})
	})
})

var _ = Describe("Patch", func() {
	Describe("CreateMergePatch", func() {
		var cm *corev1.ConfigMap

		BeforeEach(func() {
			cm = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: metav1.NamespaceDefault,
					Name:      "cm",
				},
			}
		})

		It("creates a merge patch with the modifications applied during the mutation", func() {
			const (
				annotationKey   = "test"
				annotationValue = "foo"
			)

			By("creating a merge patch")
			patch := client.MergeFrom(cm.DeepCopy())

			By("returning a patch with type MergePatch")
			Expect(patch.Type()).To(Equal(types.MergePatchType))

			By("retrieving modifying the config map")
			metav1.SetMetaDataAnnotation(&cm.ObjectMeta, annotationKey, annotationValue)

			By("computing the patch data")
			data, err := patch.Data(cm)

			By("returning no error")
			Expect(err).NotTo(HaveOccurred())

			By("returning a patch with data only containing the annotation change")
			Expect(data).To(Equal([]byte(fmt.Sprintf(`{"metadata":{"annotations":{"%s":"%s"}}}`, annotationKey, annotationValue))))
		})
	})
})

var _ = Describe("IgnoreNotFound", func() {
	It("should return nil on a 'NotFound' error", func() {
		By("creating a NotFound error")
		err := apierrors.NewNotFound(schema.GroupResource{}, "")

		By("returning no error")
		Expect(client.IgnoreNotFound(err)).To(Succeed())
	})

	It("should return the error on a status other than not found", func() {
		By("creating a BadRequest error")
		err := apierrors.NewBadRequest("")

		By("returning an error")
		Expect(client.IgnoreNotFound(err)).To(HaveOccurred())
	})

	It("should return the error on a non-status error", func() {
		By("creating an fmt error")
		err := fmt.Errorf("arbitrary error")

		By("returning an error")
		Expect(client.IgnoreNotFound(err)).To(HaveOccurred())
	})
})

type fakeReader struct {
	Called int
}

func (f *fakeReader) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	f.Called = f.Called + 1
	return nil
}

func (f *fakeReader) List(ctx context.Context, list runtime.Object, opts ...client.ListOptionFunc) error {
	f.Called = f.Called + 1
	return nil
}
