/*
Copyright The Kubernetes Authors.

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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Delete response", func() {
	const finalizer = "example.com/hold"

	newConfigMap := func(finalizers ...string) *corev1.ConfigMap {
		return &corev1.ConfigMap{
			Name: "cm", Namespace: "default", Finalizers: finalizers,
			Data: map[string]string{"key": "value"},
		}
	}
	configMapGVK := corev1.SchemeGroupVersion.WithKind("ConfigMap")

	It("should leave obj unchanged if the object was removed from storage", func(ctx SpecContext) {
		cl := NewClientBuilder().WithObjects(newConfigMap()).Build()
		obj := &corev1.ConfigMap{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(newConfigMap()), obj)).To(Succeed())
		obj.Labels = map[string]string{"local": "change"}
		before := obj.DeepCopy()

		Expect(cl.Delete(ctx, obj)).To(Succeed())
		Expect(obj).To(Equal(before))
		Expect(apierrors.IsNotFound(cl.Get(ctx, client.ObjectKeyFromObject(obj), &corev1.ConfigMap{}))).To(BeTrue())
	})

	It("should update obj with the terminating object if it is held by finalizers", func(ctx SpecContext) {
		cl := NewClientBuilder().WithObjects(newConfigMap(finalizer)).Build()
		obj := &corev1.ConfigMap{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(newConfigMap()), obj)).To(Succeed())
		rvBefore := obj.ResourceVersion

		Expect(cl.Delete(ctx, obj)).To(Succeed())

		stored := &corev1.ConfigMap{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(obj), stored)).To(Succeed())
		Expect(stored.DeletionTimestamp).NotTo(BeNil())
		Expect(obj).To(Equal(stored))
		Expect(obj.ResourceVersion).NotTo(Equal(rvBefore))
	})

	It("should not change an object that is already terminating, but update obj with it", func(ctx SpecContext) {
		cl := NewClientBuilder().WithObjects(newConfigMap(finalizer)).Build()
		Expect(cl.Delete(ctx, newConfigMap())).To(Succeed())
		terminating := &corev1.ConfigMap{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(newConfigMap()), terminating)).To(Succeed())
		Expect(terminating.DeletionTimestamp).NotTo(BeNil())

		obj := terminating.DeepCopy()
		obj.Labels = map[string]string{"local": "change"}
		Expect(cl.Delete(ctx, obj)).To(Succeed())
		Expect(obj).To(Equal(terminating))

		stored := &corev1.ConfigMap{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(obj), stored)).To(Succeed())
		Expect(stored).To(Equal(terminating))
	})

	It("should keep a namespace with spec finalizers and mark it terminating", func(ctx SpecContext) {
		namespace := &corev1.Namespace{
			Name: "ns",
			Spec: corev1.NamespaceSpec{Finalizers: []corev1.FinalizerName{corev1.FinalizerKubernetes}},
		}
		cl := NewClientBuilder().WithObjects(namespace).Build()
		obj := &corev1.Namespace{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(namespace), obj)).To(Succeed())

		dryRun := obj.DeepCopy()
		Expect(cl.Delete(ctx, dryRun, client.DryRunAll)).To(Succeed())
		Expect(dryRun.DeletionTimestamp).NotTo(BeNil())
		Expect(dryRun.Status.Phase).To(Equal(corev1.NamespaceTerminating))
		stored := &corev1.Namespace{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(namespace), stored)).To(Succeed())
		Expect(stored.DeletionTimestamp).To(BeNil())

		Expect(cl.Delete(ctx, obj)).To(Succeed())
		Expect(obj.DeletionTimestamp).NotTo(BeNil())
		Expect(obj.Status.Phase).To(Equal(corev1.NamespaceTerminating))
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(namespace), stored)).To(Succeed())
		Expect(obj).To(Equal(stored))
	})

	It("should remove a namespace without finalizers", func(ctx SpecContext) {
		namespace := &corev1.Namespace{Name: "ns"}
		cl := NewClientBuilder().WithObjects(namespace).Build()
		obj := &corev1.Namespace{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(namespace), obj)).To(Succeed())
		before := obj.DeepCopy()

		Expect(cl.Delete(ctx, obj)).To(Succeed())
		Expect(obj).To(Equal(before))
		Expect(apierrors.IsNotFound(cl.Get(ctx, client.ObjectKeyFromObject(namespace), &corev1.Namespace{}))).To(BeTrue())
	})

	It("should update unstructured objects and preserve their GVK", func(ctx SpecContext) {
		cm := newConfigMap(finalizer)
		cl := NewClientBuilder().WithObjects(cm).Build()

		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(configMapGVK)
		u.SetNamespace(cm.Namespace)
		u.SetName(cm.Name)
		Expect(cl.Delete(ctx, u)).To(Succeed())
		Expect(u.GroupVersionKind()).To(Equal(configMapGVK))
		Expect(u.GetDeletionTimestamp()).NotTo(BeNil())
		data, _, err := unstructured.NestedStringMap(u.Object, "data")
		Expect(err).NotTo(HaveOccurred())
		Expect(data).To(Equal(cm.Data))
	})

	It("should update PartialObjectMetadata and preserve its GVK", func(ctx SpecContext) {
		cm := newConfigMap(finalizer)
		cl := NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(cm).Build()

		m := &metav1.PartialObjectMetadata{}
		m.SetGroupVersionKind(configMapGVK)
		m.SetNamespace(cm.Namespace)
		m.SetName(cm.Name)
		Expect(cl.Delete(ctx, m)).To(Succeed())
		Expect(m.GroupVersionKind()).To(Equal(configMapGVK))
		Expect(m.GetDeletionTimestamp()).NotTo(BeNil())
		Expect(m.GetFinalizers()).To(ConsistOf(finalizer))
	})

	It("should update obj with the object as it would remain on a dry-run delete", func(ctx SpecContext) {
		cl := NewClientBuilder().WithObjects(newConfigMap(finalizer)).Build()
		obj := newConfigMap()

		Expect(cl.Delete(ctx, obj, client.DryRunAll)).To(Succeed())
		Expect(obj.DeletionTimestamp).NotTo(BeNil())
		Expect(obj.Finalizers).To(ConsistOf(finalizer))

		stored := &corev1.ConfigMap{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(obj), stored)).To(Succeed())
		Expect(stored.DeletionTimestamp).To(BeNil())
	})

	It("should leave obj unchanged on a dry-run delete of an object without finalizers", func(ctx SpecContext) {
		cl := NewClientBuilder().WithObjects(newConfigMap()).Build()
		obj := newConfigMap()
		before := obj.DeepCopy()

		Expect(cl.Delete(ctx, obj, client.DryRunAll)).To(Succeed())
		Expect(obj).To(Equal(before))
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(obj), &corev1.ConfigMap{})).To(Succeed())
	})

	It("should leave obj unchanged if a precondition fails", func(ctx SpecContext) {
		cl := NewClientBuilder().WithObjects(newConfigMap(finalizer)).Build()
		obj := newConfigMap()
		before := obj.DeepCopy()

		err := cl.Delete(ctx, obj, client.Preconditions{ResourceVersion: new("1")})
		Expect(apierrors.IsConflict(err)).To(BeTrue(), "expected a conflict, got %v", err)
		Expect(obj).To(Equal(before))
	})

	It("should leave a bound pod without finalizers unchanged", func(ctx SpecContext) {
		pod := &corev1.Pod{Name: "pod", Namespace: "default", Spec: corev1.PodSpec{NodeName: "node"}}
		cl := NewClientBuilder().WithObjects(pod).Build()
		obj := &corev1.Pod{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(pod), obj)).To(Succeed())
		obj.Labels = map[string]string{"local": "change"}
		before := obj.DeepCopy()

		Expect(cl.Delete(ctx, obj)).To(Succeed())
		Expect(obj).To(Equal(before))
		Expect(apierrors.IsNotFound(cl.Get(ctx, client.ObjectKeyFromObject(pod), &corev1.Pod{}))).To(BeTrue())
	})

	It("should update a pod held by finalizers", func(ctx SpecContext) {
		pod := &corev1.Pod{Name: "pod", Namespace: "default", Finalizers: []string{finalizer}}
		cl := NewClientBuilder().WithObjects(pod).Build()
		obj := &corev1.Pod{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(pod), obj)).To(Succeed())

		Expect(cl.Delete(ctx, obj)).To(Succeed())

		stored := &corev1.Pod{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(pod), stored)).To(Succeed())
		Expect(stored.DeletionTimestamp).NotTo(BeNil())
		Expect(obj).To(Equal(stored))
	})

	It("should leave the pod unchanged on an eviction, which responds with a Status", func(ctx SpecContext) {
		pod := &corev1.Pod{Name: "pod", Namespace: "default", Finalizers: []string{finalizer}}
		cl := NewClientBuilder().WithObjects(pod).Build()
		obj := &corev1.Pod{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(pod), obj)).To(Succeed())
		before := obj.DeepCopy()

		Expect(cl.SubResource("eviction").Create(ctx, obj, &policyv1.Eviction{})).To(Succeed())
		Expect(obj).To(Equal(before))

		stored := &corev1.Pod{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(pod), stored)).To(Succeed())
		Expect(stored.DeletionTimestamp).NotTo(BeNil())
	})

	It("should leave obj unchanged if the object does not exist", func(ctx SpecContext) {
		cl := NewClientBuilder().Build()
		obj := newConfigMap(finalizer)
		before := obj.DeepCopy()

		err := cl.Delete(ctx, obj)
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected NotFound, got %v", err)
		Expect(obj).To(Equal(before))
	})
})
