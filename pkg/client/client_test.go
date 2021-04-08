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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kscheme "k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const serverSideTimeoutSeconds = 10

func deleteDeployment(ctx context.Context, dep *appsv1.Deployment, ns string) {
	_, err := clientset.AppsV1().Deployments(ns).Get(ctx, dep.Name, metav1.GetOptions{})
	if err == nil {
		err = clientset.AppsV1().Deployments(ns).Delete(ctx, dep.Name, metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())
	}
}

func deleteNamespace(ctx context.Context, ns *corev1.Namespace) {
	ns, err := clientset.CoreV1().Namespaces().Get(ctx, ns.Name, metav1.GetOptions{})
	if err != nil {
		return
	}

	err = clientset.CoreV1().Namespaces().Delete(ctx, ns.Name, metav1.DeleteOptions{})
	Expect(err).NotTo(HaveOccurred())

	// finalize if necessary
	pos := -1
	finalizers := ns.Spec.Finalizers
	for i, fin := range finalizers {
		if fin == "kubernetes" {
			pos = i
			break
		}
	}
	if pos == -1 {
		// no need to finalize
		return
	}

	// re-get in order to finalize
	ns, err = clientset.CoreV1().Namespaces().Get(ctx, ns.Name, metav1.GetOptions{})
	if err != nil {
		return
	}

	ns.Spec.Finalizers = append(finalizers[:pos], finalizers[pos+1:]...)
	_, err = clientset.CoreV1().Namespaces().Finalize(ctx, ns, metav1.UpdateOptions{})
	Expect(err).NotTo(HaveOccurred())

WAIT_LOOP:
	for i := 0; i < 10; i++ {
		ns, err = clientset.CoreV1().Namespaces().Get(ctx, ns.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			// success!
			return
		}
		select {
		case <-ctx.Done():
			break WAIT_LOOP
			// failed to delete in time, see failure below
		case <-time.After(100 * time.Millisecond):
			// do nothing, try again
		}
	}
	Fail(fmt.Sprintf("timed out waiting for namespace %q to be deleted", ns.Name))
}

// metaOnlyFromObj returns PartialObjectMetadata from a concrete Go struct that
// returns a concrete *metav1.ObjectMeta from GetObjectMeta (yes, that plays a
// bit fast and loose, but the only other options are serializing and then
// deserializing, or manually calling all the accessor funcs, which are both a bit annoying).
func metaOnlyFromObj(obj interface {
	runtime.Object
	metav1.ObjectMetaAccessor
}, scheme *runtime.Scheme) *metav1.PartialObjectMetadata {
	metaObj := metav1.PartialObjectMetadata{}
	obj.GetObjectMeta().(*metav1.ObjectMeta).DeepCopyInto(&metaObj.ObjectMeta)
	kinds, _, err := scheme.ObjectKinds(obj)
	if err != nil {
		panic(err)
	}
	metaObj.SetGroupVersionKind(kinds[0])
	return &metaObj
}

var _ = Describe("Client", func() {

	var scheme *runtime.Scheme
	var depGvk schema.GroupVersionKind
	var dep *appsv1.Deployment
	var pod *corev1.Pod
	var node *corev1.Node
	var count uint64 = 0
	var replicaCount int32 = 2
	var ns = "default"
	ctx := context.TODO()

	BeforeEach(func(done Done) {
		atomic.AddUint64(&count, 1)
		dep = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("deployment-name-%v", count), Namespace: ns, Labels: map[string]string{"app": fmt.Sprintf("bar-%v", count)}},
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
		depGvk = schema.GroupVersionKind{
			Group:   "apps",
			Kind:    "Deployment",
			Version: "v1",
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
		deleteDeployment(ctx, dep, ns)
		_, err := clientset.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
		if err == nil {
			err = clientset.CoreV1().Nodes().Delete(ctx, node.Name, *delOptions)
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

				actual, err := clientset.AppsV1().Deployments(ns).Get(ctx, dep.Name, metav1.GetOptions{})
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

				actual, err := clientset.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
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

				actual, err := clientset.AppsV1().Deployments(ns).Get(ctx, dep.Name, metav1.GetOptions{})
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
					err = cl.Create(context.TODO(), dep, client.DryRunAll)
					Expect(err).NotTo(HaveOccurred())

					actual, err := clientset.AppsV1().Deployments(ns).Get(ctx, dep.Name, metav1.GetOptions{})
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

				actual, err := clientset.AppsV1().Deployments(ns).Get(ctx, dep.Name, metav1.GetOptions{})
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

				actual, err := clientset.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
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
				actual, err := clientset.AppsV1().Deployments(ns).Get(ctx, dep.Name, metav1.GetOptions{})
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

		Context("with metadata objects", func() {
			It("should fail with an error", func() {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())

				obj := metaOnlyFromObj(dep, scheme)
				Expect(cl.Create(context.TODO(), obj)).NotTo(Succeed())
			})
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
				err = cl.Create(context.TODO(), u, client.DryRunAll)
				Expect(err).NotTo(HaveOccurred())

				actual, err := clientset.AppsV1().Deployments(ns).Get(ctx, dep.Name, metav1.GetOptions{})
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
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("updating the Deployment")
				dep.Annotations = map[string]string{"foo": "bar"}
				err = cl.Update(context.TODO(), dep)
				Expect(err).NotTo(HaveOccurred())

				By("validating updated Deployment has new annotation")
				actual, err := clientset.AppsV1().Deployments(ns).Get(ctx, dep.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Annotations["foo"]).To(Equal("bar"))

				close(done)
			})

			It("should update and preserve type information", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("updating the Deployment")
				dep.SetGroupVersionKind(depGvk)
				err = cl.Update(context.TODO(), dep)
				Expect(err).NotTo(HaveOccurred())

				By("validating updated Deployment has type information")
				Expect(dep.GroupVersionKind()).To(Equal(depGvk))

				close(done)
			})

			It("should update an existing object non-namespace object from a go struct", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				node, err := clientset.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("updating the object")
				node.Annotations = map[string]string{"foo": "bar"}
				err = cl.Update(context.TODO(), node)
				Expect(err).NotTo(HaveOccurred())

				By("validate updated Node had new annotation")
				actual, err := clientset.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Annotations["foo"]).To(Equal("bar"))

				close(done)
			})

			It("should fail if the object does not exist", func(done Done) {
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
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
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
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
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
				actual, err := clientset.AppsV1().Deployments(ns).Get(ctx, dep.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Annotations["foo"]).To(Equal("bar"))

				close(done)
			})

			It("should update and preserve type information", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("updating the Deployment")
				u := &unstructured.Unstructured{}
				Expect(scheme.Convert(dep, u, nil)).To(Succeed())
				u.SetGroupVersionKind(depGvk)
				u.SetAnnotations(map[string]string{"foo": "bar"})
				err = cl.Update(context.TODO(), u)
				Expect(err).NotTo(HaveOccurred())

				By("validating updated Deployment has type information")
				Expect(u.GroupVersionKind()).To(Equal(depGvk))

				close(done)
			})

			It("should update an existing object non-namespace object from a go struct", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				node, err := clientset.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
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
				actual, err := clientset.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Annotations["foo"]).To(Equal("bar"))

				close(done)
			})
			It("should fail if the object does not exist", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("updating non-existent object")
				u := &unstructured.Unstructured{}
				Expect(scheme.Convert(dep, u, nil)).To(Succeed())
				u.SetGroupVersionKind(depGvk)
				err = cl.Update(context.TODO(), dep)
				Expect(err).To(HaveOccurred())

				close(done)
			})
		})
		Context("with metadata objects", func() {
			It("should fail with an error", func() {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())

				obj := metaOnlyFromObj(dep, scheme)

				Expect(cl.Update(context.TODO(), obj)).NotTo(Succeed())
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
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("updating the status of Deployment")
				dep.Status.Replicas = 1
				err = cl.Status().Update(context.TODO(), dep)
				Expect(err).NotTo(HaveOccurred())

				By("validating updated Deployment has new status")
				actual, err := clientset.AppsV1().Deployments(ns).Get(ctx, dep.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Status.Replicas).To(BeEquivalentTo(1))

				close(done)
			})

			It("should update status and preserve type information", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("updating the status of Deployment")
				dep.SetGroupVersionKind(depGvk)
				dep.Status.Replicas = 1
				err = cl.Status().Update(context.TODO(), dep)
				Expect(err).NotTo(HaveOccurred())

				By("validating updated Deployment has type information")
				Expect(dep.GroupVersionKind()).To(Equal(depGvk))

				close(done)
			})

			It("should patch status and preserve type information", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("patching the status of Deployment")
				dep.SetGroupVersionKind(depGvk)
				depPatch := client.MergeFrom(dep.DeepCopy())
				dep.Status.Replicas = 1
				err = cl.Status().Patch(context.TODO(), dep, depPatch)
				Expect(err).NotTo(HaveOccurred())

				By("validating updated Deployment has type information")
				Expect(dep.GroupVersionKind()).To(Equal(depGvk))

				close(done)
			})

			It("should not update spec of an existing object", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("updating the spec and status of Deployment")
				var rc int32 = 1
				dep.Status.Replicas = 1
				dep.Spec.Replicas = &rc
				err = cl.Status().Update(context.TODO(), dep)
				Expect(err).NotTo(HaveOccurred())

				By("validating updated Deployment has new status and unchanged spec")
				actual, err := clientset.AppsV1().Deployments(ns).Get(ctx, dep.Name, metav1.GetOptions{})
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

				node, err := clientset.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("updating status of the object")
				node.Status.Phase = corev1.NodeRunning
				err = cl.Status().Update(context.TODO(), node)
				Expect(err).NotTo(HaveOccurred())

				By("validate updated Node had new annotation")
				actual, err := clientset.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Status.Phase).To(Equal(corev1.NodeRunning))

				close(done)
			})

			It("should fail if the object does not exist", func(done Done) {
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
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
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
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("updating the status of Deployment")
				u := &unstructured.Unstructured{}
				dep.Status.Replicas = 1
				Expect(scheme.Convert(dep, u, nil)).To(Succeed())
				err = cl.Status().Update(context.TODO(), u)
				Expect(err).NotTo(HaveOccurred())

				By("validating updated Deployment has new status")
				actual, err := clientset.AppsV1().Deployments(ns).Get(ctx, dep.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Status.Replicas).To(BeEquivalentTo(1))

				close(done)
			})

			It("should update status and preserve type information", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("updating the status of Deployment")
				u := &unstructured.Unstructured{}
				dep.Status.Replicas = 1
				Expect(scheme.Convert(dep, u, nil)).To(Succeed())
				err = cl.Status().Update(context.TODO(), u)
				Expect(err).NotTo(HaveOccurred())

				By("validating updated Deployment has type information")
				Expect(u.GroupVersionKind()).To(Equal(depGvk))

				close(done)
			})

			It("should patch status and preserve type information", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("patching the status of Deployment")
				u := &unstructured.Unstructured{}
				depPatch := client.MergeFrom(dep.DeepCopy())
				dep.Status.Replicas = 1
				Expect(scheme.Convert(dep, u, nil)).To(Succeed())
				err = cl.Status().Patch(context.TODO(), u, depPatch)
				Expect(err).NotTo(HaveOccurred())

				By("validating updated Deployment has type information")
				Expect(u.GroupVersionKind()).To(Equal(depGvk))

				By("validating patched Deployment has new status")
				actual, err := clientset.AppsV1().Deployments(ns).Get(ctx, dep.Name, metav1.GetOptions{})
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
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
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
				actual, err := clientset.AppsV1().Deployments(ns).Get(ctx, dep.Name, metav1.GetOptions{})
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

				node, err := clientset.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("updating status of the object")
				u := &unstructured.Unstructured{}
				node.Status.Phase = corev1.NodeRunning
				Expect(scheme.Convert(node, u, nil)).To(Succeed())
				err = cl.Status().Update(context.TODO(), u)
				Expect(err).NotTo(HaveOccurred())

				By("validate updated Node had new annotation")
				actual, err := clientset.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Status.Phase).To(Equal(corev1.NodeRunning))

				close(done)
			})

			It("should fail if the object does not exist", func(done Done) {
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

		Context("with metadata objects", func() {
			It("should fail to update with an error", func() {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())

				obj := metaOnlyFromObj(dep, scheme)
				Expect(cl.Status().Update(context.TODO(), obj)).NotTo(Succeed())
			})

			It("should patch status and preserve type information", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("patching the status of Deployment")
				objPatch := client.MergeFrom(metaOnlyFromObj(dep, scheme))
				dep.Annotations = map[string]string{"some-new-annotation": "some-new-value"}
				obj := metaOnlyFromObj(dep, scheme)
				err = cl.Status().Patch(context.TODO(), obj, objPatch)
				Expect(err).NotTo(HaveOccurred())

				By("validating updated Deployment has type information")
				Expect(obj.GroupVersionKind()).To(Equal(depGvk))

				By("validating patched Deployment has new status")
				actual, err := clientset.AppsV1().Deployments(ns).Get(ctx, dep.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Annotations).To(HaveKeyWithValue("some-new-annotation", "some-new-value"))

				close(done)
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
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("deleting the Deployment")
				depName := dep.Name
				err = cl.Delete(context.TODO(), dep)
				Expect(err).NotTo(HaveOccurred())

				By("validating the Deployment no longer exists")
				_, err = clientset.AppsV1().Deployments(ns).Get(ctx, depName, metav1.GetOptions{})
				Expect(err).To(HaveOccurred())

				close(done)
			})

			It("should delete an existing object non-namespace object from a go struct", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Node")
				node, err := clientset.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("deleting the Node")
				nodeName := node.Name
				err = cl.Delete(context.TODO(), node)
				Expect(err).NotTo(HaveOccurred())

				By("validating the Node no longer exists")
				_, err = clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
				Expect(err).To(HaveOccurred())

				close(done)
			})

			It("should fail if the object does not exist", func(done Done) {
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
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("deleting the Deployment fails")
				err = cl.Delete(context.TODO(), dep)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no kind is registered for the type"))

				close(done)
			})

			PIt("should fail if the GVK cannot be mapped to a Resource", func() {

			})

			It("should delete a collection of objects", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating two Deployments")

				dep2 := dep.DeepCopy()
				dep2.Name = dep2.Name + "-2"

				dep, err = clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				dep2, err = clientset.AppsV1().Deployments(ns).Create(ctx, dep2, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				depName := dep.Name
				dep2Name := dep2.Name

				By("deleting Deployments")
				err = cl.DeleteAllOf(context.TODO(), dep, client.InNamespace(ns), client.MatchingLabels(dep.ObjectMeta.Labels))
				Expect(err).NotTo(HaveOccurred())

				By("validating the Deployment no longer exists")
				_, err = clientset.AppsV1().Deployments(ns).Get(ctx, depName, metav1.GetOptions{})
				Expect(err).To(HaveOccurred())
				_, err = clientset.AppsV1().Deployments(ns).Get(ctx, dep2Name, metav1.GetOptions{})
				Expect(err).To(HaveOccurred())

				close(done)
			})
		})
		Context("with unstructured objects", func() {
			It("should delete an existing object from a go struct", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
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
				_, err = clientset.AppsV1().Deployments(ns).Get(ctx, depName, metav1.GetOptions{})
				Expect(err).To(HaveOccurred())

				close(done)
			})

			It("should delete an existing object non-namespace object from a go struct", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Node")
				node, err := clientset.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
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
				_, err = clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
				Expect(err).To(HaveOccurred())

				close(done)
			})

			It("should fail if the object does not exist", func(done Done) {
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

			It("should delete a collection of object", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating two Deployments")

				dep2 := dep.DeepCopy()
				dep2.Name = dep2.Name + "-2"

				dep, err = clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				dep2, err = clientset.AppsV1().Deployments(ns).Create(ctx, dep2, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				depName := dep.Name
				dep2Name := dep2.Name

				By("deleting Deployments")
				u := &unstructured.Unstructured{}
				Expect(scheme.Convert(dep, u, nil)).To(Succeed())
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})
				err = cl.DeleteAllOf(context.TODO(), u, client.InNamespace(ns), client.MatchingLabels(dep.ObjectMeta.Labels))
				Expect(err).NotTo(HaveOccurred())

				By("validating the Deployment no longer exists")
				_, err = clientset.AppsV1().Deployments(ns).Get(ctx, depName, metav1.GetOptions{})
				Expect(err).To(HaveOccurred())
				_, err = clientset.AppsV1().Deployments(ns).Get(ctx, dep2Name, metav1.GetOptions{})
				Expect(err).To(HaveOccurred())

				close(done)
			})
		})
		Context("with metadata objects", func() {
			It("should delete an existing object from a go struct", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("deleting the Deployment")
				metaObj := metaOnlyFromObj(dep, scheme)
				err = cl.Delete(context.TODO(), metaObj)
				Expect(err).NotTo(HaveOccurred())

				By("validating the Deployment no longer exists")
				_, err = clientset.AppsV1().Deployments(ns).Get(ctx, dep.Name, metav1.GetOptions{})
				Expect(err).To(HaveOccurred())

				close(done)
			})

			It("should delete an existing object non-namespace object from a go struct", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Node")
				node, err := clientset.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("deleting the Node")
				metaObj := metaOnlyFromObj(node, scheme)
				err = cl.Delete(context.TODO(), metaObj)
				Expect(err).NotTo(HaveOccurred())

				By("validating the Node no longer exists")
				_, err = clientset.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
				Expect(err).To(HaveOccurred())

				close(done)
			})

			It("should fail if the object does not exist", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("Deleting node before it is ever created")
				metaObj := metaOnlyFromObj(node, scheme)
				err = cl.Delete(context.TODO(), metaObj)
				Expect(err).To(HaveOccurred())

				close(done)
			})

			It("should delete a collection of object", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating two Deployments")

				dep2 := dep.DeepCopy()
				dep2.Name = dep2.Name + "-2"

				dep, err = clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				dep2, err = clientset.AppsV1().Deployments(ns).Create(ctx, dep2, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				depName := dep.Name
				dep2Name := dep2.Name

				By("deleting Deployments")
				metaObj := metaOnlyFromObj(dep, scheme)
				err = cl.DeleteAllOf(context.TODO(), metaObj, client.InNamespace(ns), client.MatchingLabels(dep.ObjectMeta.Labels))
				Expect(err).NotTo(HaveOccurred())

				By("validating the Deployment no longer exists")
				_, err = clientset.AppsV1().Deployments(ns).Get(ctx, depName, metav1.GetOptions{})
				Expect(err).To(HaveOccurred())
				_, err = clientset.AppsV1().Deployments(ns).Get(ctx, dep2Name, metav1.GetOptions{})
				Expect(err).To(HaveOccurred())

				close(done)
			})
		})
	})

	Describe("Get", func() {
		Context("with structured objects", func() {
			It("should fetch an existing object for a go struct", func(done Done) {
				By("first creating the Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
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
				node, err := clientset.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
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

			It("should fail if the object does not exist", func(done Done) {
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
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
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
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
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
				node, err := clientset.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
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

			It("should fail if the object does not exist", func(done Done) {
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
		Context("with metadata objects", func() {
			It("should fetch an existing object for a go struct", func(done Done) {
				By("first creating the Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("fetching the created Deployment")
				var actual metav1.PartialObjectMetadata
				actual.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
				})
				key := client.ObjectKey{Namespace: ns, Name: dep.Name}
				err = cl.Get(context.TODO(), key, &actual)
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())

				By("validating the fetched deployment equals the created one")
				Expect(metaOnlyFromObj(dep, scheme)).To(Equal(&actual))

				close(done)
			})

			It("should fetch an existing non-namespace object for a go struct", func(done Done) {
				By("first creating the object")
				node, err := clientset.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("retrieving node through client")
				var actual metav1.PartialObjectMetadata
				actual.SetGroupVersionKind(schema.GroupVersionKind{
					Version: "v1",
					Kind:    "Node",
				})
				key := client.ObjectKey{Namespace: ns, Name: node.Name}
				err = cl.Get(context.TODO(), key, &actual)
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())

				Expect(metaOnlyFromObj(node, scheme)).To(Equal(&actual))

				close(done)
			})

			It("should fail if the object does not exist", func(done Done) {
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("fetching object that has not been created yet")
				key := client.ObjectKey{Namespace: ns, Name: dep.Name}
				var actual metav1.PartialObjectMetadata
				actual.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
				})
				err = cl.Get(context.TODO(), key, &actual)
				Expect(err).To(HaveOccurred())

				close(done)
			})

			PIt("should fail if the object doesn't have meta", func() {

			})

			PIt("should fail if the GVK cannot be mapped to a Resource", func() {

			})
		})
	})

	Describe("List", func() {
		Context("with structured objects", func() {
			It("should fetch collection of objects", func(done Done) {
				By("creating an initial object")
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
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
				_, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
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

			It("should fetch unstructured collection of objects, even if scheme is empty", func(done Done) {
				By("create an initial object")
				_, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				cl, err := client.New(cfg, client.Options{Scheme: runtime.NewScheme()})
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
				depFrontend, err := clientset.AppsV1().Deployments(ns).Create(ctx, depFrontend, metav1.CreateOptions{})
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
				depBackend, err = clientset.AppsV1().Deployments(ns).Create(ctx, depBackend, metav1.CreateOptions{})
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

				deleteDeployment(ctx, depFrontend, ns)
				deleteDeployment(ctx, depBackend, ns)

				close(done)
			}, serverSideTimeoutSeconds)

			It("should filter results by namespace selector", func(done Done) {
				By("creating a Deployment in test-namespace-1")
				tns1 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-1"}}
				_, err := clientset.CoreV1().Namespaces().Create(ctx, tns1, metav1.CreateOptions{})
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
				depFrontend, err = clientset.AppsV1().Deployments("test-namespace-1").Create(ctx, depFrontend, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("creating a Deployment in test-namespace-2")
				tns2 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-2"}}
				_, err = clientset.CoreV1().Namespaces().Create(ctx, tns2, metav1.CreateOptions{})
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
				depBackend, err = clientset.AppsV1().Deployments("test-namespace-2").Create(ctx, depBackend, metav1.CreateOptions{})
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

				deleteDeployment(ctx, depFrontend, "test-namespace-1")
				deleteDeployment(ctx, depBackend, "test-namespace-2")
				deleteNamespace(ctx, tns1)
				deleteNamespace(ctx, tns2)

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
				depFrontend, err := clientset.AppsV1().Deployments(ns).Create(ctx, depFrontend, metav1.CreateOptions{})
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
				depBackend, err = clientset.AppsV1().Deployments(ns).Create(ctx, depBackend, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())

				By("listing all Deployments with field metadata.name=deployment-backend")
				deps := &appsv1.DeploymentList{}
				err = cl.List(context.Background(), deps,
					client.MatchingFields{"metadata.name": "deployment-backend"})
				Expect(err).NotTo(HaveOccurred())

				By("only the Deployment with the backend field is returned")
				Expect(deps.Items).NotTo(BeEmpty())
				Expect(1).To(Equal(len(deps.Items)))
				actual := deps.Items[0]
				Expect(actual.Name).To(Equal("deployment-backend"))

				deleteDeployment(ctx, depFrontend, ns)
				deleteDeployment(ctx, depBackend, ns)

				close(done)
			}, serverSideTimeoutSeconds)

			It("should filter results by namespace selector and label selector", func(done Done) {
				By("creating a Deployment in test-namespace-3 with the app=frontend label")
				tns3 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-3"}}
				_, err := clientset.CoreV1().Namespaces().Create(ctx, tns3, metav1.CreateOptions{})
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
				depFrontend3, err = clientset.AppsV1().Deployments("test-namespace-3").Create(ctx, depFrontend3, metav1.CreateOptions{})
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
				depBackend3, err = clientset.AppsV1().Deployments("test-namespace-3").Create(ctx, depBackend3, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("creating a Deployment in test-namespace-4 with the app=frontend label")
				tns4 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-4"}}
				_, err = clientset.CoreV1().Namespaces().Create(ctx, tns4, metav1.CreateOptions{})
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
				depFrontend4, err = clientset.AppsV1().Deployments("test-namespace-4").Create(ctx, depFrontend4, metav1.CreateOptions{})
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

				deleteDeployment(ctx, depFrontend3, "test-namespace-3")
				deleteDeployment(ctx, depBackend3, "test-namespace-3")
				deleteDeployment(ctx, depFrontend4, "test-namespace-4")
				deleteNamespace(ctx, tns3)
				deleteNamespace(ctx, tns4)

				close(done)
			}, serverSideTimeoutSeconds)

			It("should filter results using limit and continue options", func() {

				makeDeployment := func(suffix string) *appsv1.Deployment {
					return &appsv1.Deployment{
						ObjectMeta: metav1.ObjectMeta{
							Name: fmt.Sprintf("deployment-%s", suffix),
						},
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
				}

				By("creating 4 deployments")
				dep1 := makeDeployment("1")
				dep1, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep1, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				defer deleteDeployment(ctx, dep1, ns)

				dep2 := makeDeployment("2")
				dep2, err = clientset.AppsV1().Deployments(ns).Create(ctx, dep2, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				defer deleteDeployment(ctx, dep2, ns)

				dep3 := makeDeployment("3")
				dep3, err = clientset.AppsV1().Deployments(ns).Create(ctx, dep3, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				defer deleteDeployment(ctx, dep3, ns)

				dep4 := makeDeployment("4")
				dep4, err = clientset.AppsV1().Deployments(ns).Create(ctx, dep4, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				defer deleteDeployment(ctx, dep4, ns)

				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())

				By("listing 1 deployment when limit=1 is used")
				deps := &appsv1.DeploymentList{}
				err = cl.List(context.Background(), deps,
					client.Limit(1),
				)
				Expect(err).NotTo(HaveOccurred())

				Expect(deps.Items).To(HaveLen(1))
				Expect(deps.Continue).NotTo(BeEmpty())
				Expect(deps.Items[0].Name).To(Equal(dep1.Name))

				continueToken := deps.Continue

				By("listing the next deployment when previous continuation token is used and limit=1")
				deps = &appsv1.DeploymentList{}
				err = cl.List(context.Background(), deps,
					client.Limit(1),
					client.Continue(continueToken),
				)
				Expect(err).NotTo(HaveOccurred())

				Expect(deps.Items).To(HaveLen(1))
				Expect(deps.Continue).NotTo(BeEmpty())
				Expect(deps.Items[0].Name).To(Equal(dep2.Name))

				continueToken = deps.Continue

				By("listing the 2 remaining deployments when previous continuation token is used without a limit")
				deps = &appsv1.DeploymentList{}
				err = cl.List(context.Background(), deps,
					client.Continue(continueToken),
				)
				Expect(err).NotTo(HaveOccurred())

				Expect(deps.Items).To(HaveLen(2))
				Expect(deps.Continue).To(BeEmpty())
				Expect(deps.Items[0].Name).To(Equal(dep3.Name))
				Expect(deps.Items[1].Name).To(Equal(dep4.Name))
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
				_, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
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
				_, err := clientset.CoreV1().Namespaces().Create(ctx, tns1, metav1.CreateOptions{})
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
				depFrontend, err = clientset.AppsV1().Deployments("test-namespace-5").Create(ctx, depFrontend, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("creating a Deployment in test-namespace-6")
				tns2 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-6"}}
				_, err = clientset.CoreV1().Namespaces().Create(ctx, tns2, metav1.CreateOptions{})
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
				depBackend, err = clientset.AppsV1().Deployments("test-namespace-6").Create(ctx, depBackend, metav1.CreateOptions{})
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

				deleteDeployment(ctx, depFrontend, "test-namespace-5")
				deleteDeployment(ctx, depBackend, "test-namespace-6")
				deleteNamespace(ctx, tns1)
				deleteNamespace(ctx, tns2)

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
				depFrontend, err := clientset.AppsV1().Deployments(ns).Create(ctx, depFrontend, metav1.CreateOptions{})
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
				depBackend, err = clientset.AppsV1().Deployments(ns).Create(ctx, depBackend, metav1.CreateOptions{})
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
					client.MatchingFields{"metadata.name": "deployment-backend"})
				Expect(err).NotTo(HaveOccurred())

				By("only the Deployment with the backend field is returned")
				Expect(deps.Items).NotTo(BeEmpty())
				Expect(1).To(Equal(len(deps.Items)))
				actual := deps.Items[0]
				Expect(actual.GetName()).To(Equal("deployment-backend"))

				deleteDeployment(ctx, depFrontend, ns)
				deleteDeployment(ctx, depBackend, ns)

				close(done)
			}, serverSideTimeoutSeconds)

			It("should filter results by namespace selector and label selector", func(done Done) {
				By("creating a Deployment in test-namespace-7 with the app=frontend label")
				tns3 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-7"}}
				_, err := clientset.CoreV1().Namespaces().Create(ctx, tns3, metav1.CreateOptions{})
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
				depFrontend3, err = clientset.AppsV1().Deployments("test-namespace-7").Create(ctx, depFrontend3, metav1.CreateOptions{})
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
				depBackend3, err = clientset.AppsV1().Deployments("test-namespace-7").Create(ctx, depBackend3, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("creating a Deployment in test-namespace-8 with the app=frontend label")
				tns4 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-8"}}
				_, err = clientset.CoreV1().Namespaces().Create(ctx, tns4, metav1.CreateOptions{})
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
				depFrontend4, err = clientset.AppsV1().Deployments("test-namespace-8").Create(ctx, depFrontend4, metav1.CreateOptions{})
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

				deleteDeployment(ctx, depFrontend3, "test-namespace-7")
				deleteDeployment(ctx, depBackend3, "test-namespace-7")
				deleteDeployment(ctx, depFrontend4, "test-namespace-8")
				deleteNamespace(ctx, tns3)
				deleteNamespace(ctx, tns4)

				close(done)
			}, serverSideTimeoutSeconds)

			PIt("should fail if the object doesn't have meta", func() {

			})

			PIt("should filter results by namespace selector", func() {

			})
		})
		Context("with metadata objects", func() {
			It("should fetch collection of objects", func(done Done) {
				By("creating an initial object")
				dep, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())

				By("listing all objects of that type in the cluster")
				metaList := &metav1.PartialObjectMetadataList{}
				metaList.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    "DeploymentList",
				})
				Expect(cl.List(context.Background(), metaList)).NotTo(HaveOccurred())

				Expect(metaList.Items).NotTo(BeEmpty())
				hasDep := false
				for _, item := range metaList.Items {
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
				metaList := &metav1.PartialObjectMetadataList{}
				metaList.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    "DeploymentList",
				})
				Expect(cl.List(context.Background(), metaList)).NotTo(HaveOccurred())

				By("validating no Deployments are returned")
				Expect(metaList.Items).To(BeEmpty())

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
				depFrontend, err := clientset.AppsV1().Deployments(ns).Create(ctx, depFrontend, metav1.CreateOptions{})
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
				depBackend, err = clientset.AppsV1().Deployments(ns).Create(ctx, depBackend, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())

				By("listing all Deployments with label app=backend")
				metaList := &metav1.PartialObjectMetadataList{}
				metaList.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    "DeploymentList",
				})
				labels := map[string]string{"app": "backend"}
				err = cl.List(context.Background(), metaList, client.MatchingLabels(labels))
				Expect(err).NotTo(HaveOccurred())

				By("only the Deployment with the backend label is returned")
				Expect(metaList.Items).NotTo(BeEmpty())
				Expect(1).To(Equal(len(metaList.Items)))
				actual := metaList.Items[0]
				Expect(actual.Name).To(Equal("deployment-backend"))

				deleteDeployment(ctx, depFrontend, ns)
				deleteDeployment(ctx, depBackend, ns)

				close(done)
			}, serverSideTimeoutSeconds)

			It("should filter results by namespace selector", func(done Done) {
				By("creating a Deployment in test-namespace-1")
				tns1 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-1"}}
				_, err := clientset.CoreV1().Namespaces().Create(ctx, tns1, metav1.CreateOptions{})
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
				depFrontend, err = clientset.AppsV1().Deployments("test-namespace-1").Create(ctx, depFrontend, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("creating a Deployment in test-namespace-2")
				tns2 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-2"}}
				_, err = clientset.CoreV1().Namespaces().Create(ctx, tns2, metav1.CreateOptions{})
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
				depBackend, err = clientset.AppsV1().Deployments("test-namespace-2").Create(ctx, depBackend, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())

				By("listing all Deployments in test-namespace-1")
				metaList := &metav1.PartialObjectMetadataList{}
				metaList.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    "DeploymentList",
				})
				err = cl.List(context.Background(), metaList, client.InNamespace("test-namespace-1"))
				Expect(err).NotTo(HaveOccurred())

				By("only the Deployment in test-namespace-1 is returned")
				Expect(metaList.Items).NotTo(BeEmpty())
				Expect(1).To(Equal(len(metaList.Items)))
				actual := metaList.Items[0]
				Expect(actual.Name).To(Equal("deployment-frontend"))

				deleteDeployment(ctx, depFrontend, "test-namespace-1")
				deleteDeployment(ctx, depBackend, "test-namespace-2")
				deleteNamespace(ctx, tns1)
				deleteNamespace(ctx, tns2)

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
				depFrontend, err := clientset.AppsV1().Deployments(ns).Create(ctx, depFrontend, metav1.CreateOptions{})
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
				depBackend, err = clientset.AppsV1().Deployments(ns).Create(ctx, depBackend, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())

				By("listing all Deployments with field metadata.name=deployment-backend")
				metaList := &metav1.PartialObjectMetadataList{}
				metaList.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    "DeploymentList",
				})
				err = cl.List(context.Background(), metaList,
					client.MatchingFields{"metadata.name": "deployment-backend"})
				Expect(err).NotTo(HaveOccurred())

				By("only the Deployment with the backend field is returned")
				Expect(metaList.Items).NotTo(BeEmpty())
				Expect(1).To(Equal(len(metaList.Items)))
				actual := metaList.Items[0]
				Expect(actual.Name).To(Equal("deployment-backend"))

				deleteDeployment(ctx, depFrontend, ns)
				deleteDeployment(ctx, depBackend, ns)

				close(done)
			}, serverSideTimeoutSeconds)

			It("should filter results by namespace selector and label selector", func(done Done) {
				By("creating a Deployment in test-namespace-3 with the app=frontend label")
				tns3 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-3"}}
				_, err := clientset.CoreV1().Namespaces().Create(ctx, tns3, metav1.CreateOptions{})
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
				depFrontend3, err = clientset.AppsV1().Deployments("test-namespace-3").Create(ctx, depFrontend3, metav1.CreateOptions{})
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
				depBackend3, err = clientset.AppsV1().Deployments("test-namespace-3").Create(ctx, depBackend3, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("creating a Deployment in test-namespace-4 with the app=frontend label")
				tns4 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-4"}}
				_, err = clientset.CoreV1().Namespaces().Create(ctx, tns4, metav1.CreateOptions{})
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
				depFrontend4, err = clientset.AppsV1().Deployments("test-namespace-4").Create(ctx, depFrontend4, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())

				By("listing all Deployments in test-namespace-3 with label app=frontend")
				metaList := &metav1.PartialObjectMetadataList{}
				metaList.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    "DeploymentList",
				})
				labels := map[string]string{"app": "frontend"}
				err = cl.List(context.Background(), metaList,
					client.InNamespace("test-namespace-3"),
					client.MatchingLabels(labels),
				)
				Expect(err).NotTo(HaveOccurred())

				By("only the Deployment in test-namespace-3 with label app=frontend is returned")
				Expect(metaList.Items).NotTo(BeEmpty())
				Expect(1).To(Equal(len(metaList.Items)))
				actual := metaList.Items[0]
				Expect(actual.Name).To(Equal("deployment-frontend"))
				Expect(actual.Namespace).To(Equal("test-namespace-3"))

				deleteDeployment(ctx, depFrontend3, "test-namespace-3")
				deleteDeployment(ctx, depBackend3, "test-namespace-3")
				deleteDeployment(ctx, depFrontend4, "test-namespace-4")
				deleteNamespace(ctx, tns3)
				deleteNamespace(ctx, tns4)

				close(done)
			}, serverSideTimeoutSeconds)

			It("should filter results using limit and continue options", func() {

				makeDeployment := func(suffix string) *appsv1.Deployment {
					return &appsv1.Deployment{
						ObjectMeta: metav1.ObjectMeta{
							Name: fmt.Sprintf("deployment-%s", suffix),
						},
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
				}

				By("creating 4 deployments")
				dep1 := makeDeployment("1")
				dep1, err := clientset.AppsV1().Deployments(ns).Create(ctx, dep1, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				defer deleteDeployment(ctx, dep1, ns)

				dep2 := makeDeployment("2")
				dep2, err = clientset.AppsV1().Deployments(ns).Create(ctx, dep2, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				defer deleteDeployment(ctx, dep2, ns)

				dep3 := makeDeployment("3")
				dep3, err = clientset.AppsV1().Deployments(ns).Create(ctx, dep3, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				defer deleteDeployment(ctx, dep3, ns)

				dep4 := makeDeployment("4")
				dep4, err = clientset.AppsV1().Deployments(ns).Create(ctx, dep4, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				defer deleteDeployment(ctx, dep4, ns)

				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())

				By("listing 1 deployment when limit=1 is used")
				metaList := &metav1.PartialObjectMetadataList{}
				metaList.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    "DeploymentList",
				})
				err = cl.List(context.Background(), metaList,
					client.Limit(1),
				)
				Expect(err).NotTo(HaveOccurred())

				Expect(metaList.Items).To(HaveLen(1))
				Expect(metaList.Continue).NotTo(BeEmpty())
				Expect(metaList.Items[0].Name).To(Equal(dep1.Name))

				continueToken := metaList.Continue

				By("listing the next deployment when previous continuation token is used and limit=1")
				metaList = &metav1.PartialObjectMetadataList{}
				metaList.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    "DeploymentList",
				})
				err = cl.List(context.Background(), metaList,
					client.Limit(1),
					client.Continue(continueToken),
				)
				Expect(err).NotTo(HaveOccurred())

				Expect(metaList.Items).To(HaveLen(1))
				Expect(metaList.Continue).NotTo(BeEmpty())
				Expect(metaList.Items[0].Name).To(Equal(dep2.Name))

				continueToken = metaList.Continue

				By("listing the 2 remaining deployments when previous continuation token is used without a limit")
				metaList = &metav1.PartialObjectMetadataList{}
				metaList.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    "DeploymentList",
				})
				err = cl.List(context.Background(), metaList,
					client.Continue(continueToken),
				)
				Expect(err).NotTo(HaveOccurred())

				Expect(metaList.Items).To(HaveLen(2))
				Expect(metaList.Continue).To(BeEmpty())
				Expect(metaList.Items[0].Name).To(Equal(dep3.Name))
				Expect(metaList.Items[1].Name).To(Equal(dep4.Name))
			}, serverSideTimeoutSeconds)

			PIt("should fail if the object doesn't have meta", func() {

			})

			PIt("should fail if the object cannot be mapped to a GVK", func() {

			})

			PIt("should fail if the GVK cannot be mapped to a Resource", func() {

			})
		})
	})

	Describe("CreateOptions", func() {
		It("should allow setting DryRun to 'all'", func() {
			co := &client.CreateOptions{}
			client.DryRunAll.ApplyToCreate(co)
			all := []string{metav1.DryRunAll}
			Expect(co.AsCreateOptions().DryRun).To(Equal(all))
		})

		It("should allow setting the field manager", func() {
			po := &client.CreateOptions{}
			client.FieldOwner("some-owner").ApplyToCreate(po)
			Expect(po.AsCreateOptions().FieldManager).To(Equal("some-owner"))
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
			client.GracePeriodSeconds(1).ApplyToDelete(do)
			gp := int64(1)
			Expect(do.AsDeleteOptions().GracePeriodSeconds).To(Equal(&gp))
		})

		It("should allow setting Precondition", func() {
			do := &client.DeleteOptions{}
			pc := metav1.NewUIDPreconditions("uid")
			client.Preconditions(*pc).ApplyToDelete(do)
			Expect(do.AsDeleteOptions().Preconditions).To(Equal(pc))
			Expect(do.Preconditions).To(Equal(pc))
		})

		It("should allow setting PropagationPolicy", func() {
			do := &client.DeleteOptions{}
			client.PropagationPolicy(metav1.DeletePropagationForeground).ApplyToDelete(do)
			dp := metav1.DeletePropagationForeground
			Expect(do.AsDeleteOptions().PropagationPolicy).To(Equal(&dp))
		})

		It("should allow setting DryRun", func() {
			do := &client.DeleteOptions{}
			client.DryRunAll.ApplyToDelete(do)
			all := []string{metav1.DryRunAll}
			Expect(do.AsDeleteOptions().DryRun).To(Equal(all))
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
			do.ApplyOptions([]client.DeleteOption{
				client.GracePeriodSeconds(gp),
				client.Preconditions(*pc),
				client.PropagationPolicy(dp),
			})
			Expect(do.GracePeriodSeconds).To(Equal(&gp))
			Expect(do.Preconditions).To(Equal(pc))
			Expect(do.PropagationPolicy).To(Equal(&dp))
		})
	})

	Describe("DeleteCollectionOptions", func() {
		It("should be convertable to list options", func() {
			gp := int64(1)
			do := &client.DeleteAllOfOptions{}
			do.ApplyOptions([]client.DeleteAllOfOption{
				client.GracePeriodSeconds(gp),
				client.MatchingLabels{"foo": "bar"},
			})

			listOpts := do.AsListOptions()
			Expect(listOpts).NotTo(BeNil())
			Expect(listOpts.LabelSelector).To(Equal("foo=bar"))
		})

		It("should be convertable to delete options", func() {
			gp := int64(1)
			do := &client.DeleteAllOfOptions{}
			do.ApplyOptions([]client.DeleteAllOfOption{
				client.GracePeriodSeconds(gp),
				client.MatchingLabels{"foo": "bar"},
			})

			deleteOpts := do.AsDeleteOptions()
			Expect(deleteOpts).NotTo(BeNil())
			Expect(deleteOpts.GracePeriodSeconds).To(Equal(&gp))
		})
	})

	Describe("ListOptions", func() {
		It("should be convertable to metav1.ListOptions", func() {
			lo := (&client.ListOptions{}).ApplyOptions([]client.ListOption{
				client.MatchingFields{"field1": "bar"},
				client.InNamespace("test-namespace"),
				client.MatchingLabels{"foo": "bar"},
				client.Limit(1),
				client.Continue("foo"),
			})
			mlo := lo.AsListOptions()
			Expect(mlo).NotTo(BeNil())
			Expect(mlo.LabelSelector).To(Equal("foo=bar"))
			Expect(mlo.FieldSelector).To(Equal("field1=bar"))
			Expect(mlo.Limit).To(Equal(int64(1)))
			Expect(mlo.Continue).To(Equal("foo"))
		})

		It("should be populated by MatchingLabels", func() {
			lo := &client.ListOptions{}
			client.MatchingLabels{"foo": "bar"}.ApplyToList(lo)
			Expect(lo).NotTo(BeNil())
			Expect(lo.LabelSelector.String()).To(Equal("foo=bar"))
		})

		It("should be populated by MatchingField", func() {
			lo := &client.ListOptions{}
			client.MatchingFields{"field1": "bar"}.ApplyToList(lo)
			Expect(lo).NotTo(BeNil())
			Expect(lo.FieldSelector.String()).To(Equal("field1=bar"))
		})

		It("should be populated by InNamespace", func() {
			lo := &client.ListOptions{}
			client.InNamespace("test").ApplyToList(lo)
			Expect(lo).NotTo(BeNil())
			Expect(lo.Namespace).To(Equal("test"))
		})

		It("should produce empty metav1.ListOptions if nil", func() {
			var do *client.ListOptions
			Expect(do.AsListOptions()).To(Equal(&metav1.ListOptions{}))
			do = &client.ListOptions{}
			Expect(do.AsListOptions()).To(Equal(&metav1.ListOptions{}))
		})

		It("should be populated by Limit", func() {
			lo := &client.ListOptions{}
			client.Limit(1).ApplyToList(lo)
			Expect(lo).NotTo(BeNil())
			Expect(lo.Limit).To(Equal(int64(1)))
		})

		It("should ignore Limit when converted to metav1.ListOptions and watch is true", func() {
			lo := &client.ListOptions{
				Raw: &metav1.ListOptions{Watch: true},
			}
			lo.ApplyOptions([]client.ListOption{
				client.Limit(1),
			})
			mlo := lo.AsListOptions()
			Expect(mlo).NotTo(BeNil())
			Expect(mlo.Limit).To(BeZero())
		})

		It("should be populated by Continue", func() {
			lo := &client.ListOptions{}
			client.Continue("foo").ApplyToList(lo)
			Expect(lo).NotTo(BeNil())
			Expect(lo.Continue).To(Equal("foo"))
		})

		It("should ignore Continue token when converted to metav1.ListOptions and watch is true", func() {
			lo := &client.ListOptions{
				Raw: &metav1.ListOptions{Watch: true},
			}
			lo.ApplyOptions([]client.ListOption{
				client.Continue("foo"),
			})
			mlo := lo.AsListOptions()
			Expect(mlo).NotTo(BeNil())
			Expect(mlo.Continue).To(BeEmpty())
		})

		It("should ignore both Limit and Continue token when converted to metav1.ListOptions and watch is true", func() {
			lo := &client.ListOptions{
				Raw: &metav1.ListOptions{Watch: true},
			}
			lo.ApplyOptions([]client.ListOption{
				client.Limit(1),
				client.Continue("foo"),
			})
			mlo := lo.AsListOptions()
			Expect(mlo).NotTo(BeNil())
			Expect(mlo.Limit).To(BeZero())
			Expect(mlo.Continue).To(BeEmpty())
		})
	})

	Describe("UpdateOptions", func() {
		It("should allow setting DryRun to 'all'", func() {
			uo := &client.UpdateOptions{}
			client.DryRunAll.ApplyToUpdate(uo)
			all := []string{metav1.DryRunAll}
			Expect(uo.AsUpdateOptions().DryRun).To(Equal(all))
		})

		It("should allow setting the field manager", func() {
			po := &client.UpdateOptions{}
			client.FieldOwner("some-owner").ApplyToUpdate(po)
			Expect(po.AsUpdateOptions().FieldManager).To(Equal("some-owner"))
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
			client.DryRunAll.ApplyToPatch(po)
			all := []string{metav1.DryRunAll}
			Expect(po.AsPatchOptions().DryRun).To(Equal(all))
		})

		It("should allow setting Force to 'true'", func() {
			po := &client.PatchOptions{}
			client.ForceOwnership.ApplyToPatch(po)
			mpo := po.AsPatchOptions()
			Expect(mpo.Force).NotTo(BeNil())
			Expect(*mpo.Force).To(BeTrue())
		})

		It("should allow setting the field manager", func() {
			po := &client.PatchOptions{}
			client.FieldOwner("some-owner").ApplyToPatch(po)
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

var _ = Describe("DelegatingClient", func() {
	Describe("Get", func() {
		It("should call cache reader when structured object", func() {
			cachedReader := &fakeReader{}
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			dReader, err := client.NewDelegatingClient(client.NewDelegatingClientInput{
				CacheReader: cachedReader,
				Client:      cl,
			})
			Expect(err).NotTo(HaveOccurred())
			var actual appsv1.Deployment
			key := client.ObjectKey{Namespace: "ns", Name: "name"}
			Expect(dReader.Get(context.TODO(), key, &actual)).To(Succeed())
			Expect(1).To(Equal(cachedReader.Called))
		})

		When("getting unstructured objects", func() {
			var dep *appsv1.Deployment

			BeforeEach(func() {
				dep = &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "deployment1",
						Labels: map[string]string{"app": "frontend"},
					},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "frontend"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "frontend"}},
							Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "x", Image: "x"}}},
						},
					},
				}
				var err error
				dep, err = clientset.AppsV1().Deployments("default").Create(context.Background(), dep, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
			})
			AfterEach(func() {
				Expect(clientset.AppsV1().Deployments("default").Delete(
					context.Background(),
					dep.Name,
					metav1.DeleteOptions{},
				)).To(Succeed())
			})
			It("should call client reader when not cached", func() {
				cachedReader := &fakeReader{}
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				dReader, err := client.NewDelegatingClient(client.NewDelegatingClientInput{
					CacheReader: cachedReader,
					Client:      cl,
				})
				Expect(err).NotTo(HaveOccurred())

				actual := &unstructured.Unstructured{}
				actual.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})
				actual.SetName(dep.Name)
				key := client.ObjectKey{Namespace: dep.Namespace, Name: dep.Name}
				Expect(dReader.Get(context.TODO(), key, actual)).To(Succeed())
				Expect(0).To(Equal(cachedReader.Called))
			})
			It("should call cache reader when cached", func() {
				cachedReader := &fakeReader{}
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				dReader, err := client.NewDelegatingClient(client.NewDelegatingClientInput{
					CacheReader:       cachedReader,
					Client:            cl,
					CacheUnstructured: true,
				})
				Expect(err).NotTo(HaveOccurred())

				actual := &unstructured.Unstructured{}
				actual.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})
				actual.SetName(dep.Name)
				key := client.ObjectKey{Namespace: dep.Namespace, Name: dep.Name}
				Expect(dReader.Get(context.TODO(), key, actual)).To(Succeed())
				Expect(1).To(Equal(cachedReader.Called))
			})
		})
	})
	Describe("List", func() {
		It("should call cache reader when structured object", func() {
			cachedReader := &fakeReader{}
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			dReader, err := client.NewDelegatingClient(client.NewDelegatingClientInput{
				CacheReader: cachedReader,
				Client:      cl,
			})
			Expect(err).NotTo(HaveOccurred())
			var actual appsv1.DeploymentList
			Expect(dReader.List(context.Background(), &actual)).To(Succeed())
			Expect(1).To(Equal(cachedReader.Called))
		})

		When("listing unstructured objects", func() {
			It("should call client reader when not cached", func() {
				cachedReader := &fakeReader{}
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				dReader, err := client.NewDelegatingClient(client.NewDelegatingClientInput{
					CacheReader: cachedReader,
					Client:      cl,
				})
				Expect(err).NotTo(HaveOccurred())

				actual := &unstructured.UnstructuredList{}
				actual.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "DeploymentList",
					Version: "v1",
				})
				Expect(dReader.List(context.Background(), actual)).To(Succeed())
				Expect(0).To(Equal(cachedReader.Called))
			})
			It("should call cache reader when cached", func() {
				cachedReader := &fakeReader{}
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				dReader, err := client.NewDelegatingClient(client.NewDelegatingClientInput{
					CacheReader:       cachedReader,
					Client:            cl,
					CacheUnstructured: true,
				})
				Expect(err).NotTo(HaveOccurred())

				actual := &unstructured.UnstructuredList{}
				actual.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "DeploymentList",
					Version: "v1",
				})
				Expect(dReader.List(context.Background(), actual)).To(Succeed())
				Expect(1).To(Equal(cachedReader.Called))
			})
		})
	})
})

var _ = Describe("Patch", func() {
	Describe("MergeFrom", func() {
		var cm *corev1.ConfigMap

		BeforeEach(func() {
			cm = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:       metav1.NamespaceDefault,
					Name:            "cm",
					ResourceVersion: "10",
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

		It("creates a merge patch with the modifications applied during the mutation, using optimistic locking", func() {
			const (
				annotationKey   = "test"
				annotationValue = "foo"
			)

			By("creating a merge patch")
			patch := client.MergeFromWithOptions(cm.DeepCopy(), client.MergeFromWithOptimisticLock{})

			By("returning a patch with type MergePatch")
			Expect(patch.Type()).To(Equal(types.MergePatchType))

			By("retrieving modifying the config map")
			metav1.SetMetaDataAnnotation(&cm.ObjectMeta, annotationKey, annotationValue)

			By("computing the patch data")
			data, err := patch.Data(cm)

			By("returning no error")
			Expect(err).NotTo(HaveOccurred())

			By("returning a patch with data containing the annotation change and the resourceVersion change")
			Expect(data).To(Equal([]byte(fmt.Sprintf(`{"metadata":{"annotations":{"%s":"%s"},"resourceVersion":"%s"}}`, annotationKey, annotationValue, cm.ResourceVersion))))
		})
	})

	Describe("StrategicMergeFrom", func() {
		var dep *appsv1.Deployment

		BeforeEach(func() {
			dep = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:       metav1.NamespaceDefault,
					Name:            "dep",
					ResourceVersion: "10",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{Containers: []corev1.Container{{
							Name:  "main",
							Image: "foo:v1",
						}, {
							Name:  "sidecar",
							Image: "bar:v1",
						}}},
					},
				},
			}
		})

		It("creates a strategic merge patch with the modifications applied during the mutation", func() {
			By("creating a strategic merge patch")
			patch := client.StrategicMergeFrom(dep.DeepCopy())

			By("returning a patch with type StrategicMergePatchType")
			Expect(patch.Type()).To(Equal(types.StrategicMergePatchType))

			By("updating the main container's image")
			for i, c := range dep.Spec.Template.Spec.Containers {
				if c.Name == "main" {
					c.Image = "foo:v2"
				}
				dep.Spec.Template.Spec.Containers[i] = c
			}

			By("computing the patch data")
			data, err := patch.Data(dep)

			By("returning no error")
			Expect(err).NotTo(HaveOccurred())

			By("returning a patch with data only containing the image change")
			Expect(data).To(Equal([]byte(`{"spec":{"template":{"spec":{"$setElementOrder/containers":[{"name":"main"},` +
				`{"name":"sidecar"}],"containers":[{"image":"foo:v2","name":"main"}]}}}}`)))
		})

		It("creates a strategic merge patch with the modifications applied during the mutation, using optimistic locking", func() {
			By("creating a strategic merge patch")
			patch := client.StrategicMergeFrom(dep.DeepCopy(), client.MergeFromWithOptimisticLock{})

			By("returning a patch with type StrategicMergePatchType")
			Expect(patch.Type()).To(Equal(types.StrategicMergePatchType))

			By("updating the main container's image")
			for i, c := range dep.Spec.Template.Spec.Containers {
				if c.Name == "main" {
					c.Image = "foo:v2"
				}
				dep.Spec.Template.Spec.Containers[i] = c
			}

			By("computing the patch data")
			data, err := patch.Data(dep)

			By("returning no error")
			Expect(err).NotTo(HaveOccurred())

			By("returning a patch with data containing the image change and the resourceVersion change")
			Expect(data).To(Equal([]byte(fmt.Sprintf(`{"metadata":{"resourceVersion":"%s"},`+
				`"spec":{"template":{"spec":{"$setElementOrder/containers":[{"name":"main"},{"name":"sidecar"}],"containers":[{"image":"foo:v2","name":"main"}]}}}}`,
				dep.ResourceVersion))))
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

func (f *fakeReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	f.Called = f.Called + 1
	return nil
}

func (f *fakeReader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	f.Called = f.Called + 1
	return nil
}
