/*
Copyright 2026 The Kubernetes Authors.

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

package controllerutil_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	corev1applyconfigurations "k8s.io/client-go/applyconfigurations/core/v1"
	metav1applyconfigurations "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// unregisteredOwner is a client.Object that is not registered in any scheme.
type unregisteredOwner struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

func (o *unregisteredOwner) DeepCopyObject() runtime.Object {
	c := *o
	return &c
}

var _ = Describe("Controllerutil apply configuration helpers", func() {
	var (
		owner *appsv1.Deployment
		meta  *metav1applyconfigurations.ObjectMetaApplyConfiguration
	)

	BeforeEach(func() {
		owner = &appsv1.Deployment{
			Kind: "Deployment", APIVersion: "apps/v1",
			Name: "foo", Namespace: "default", UID: "foo-uid",
		}
		meta = corev1applyconfigurations.ConfigMap("cm", "default").ObjectMetaApplyConfiguration
	})

	ownerRefOf := func(m *metav1applyconfigurations.ObjectMetaApplyConfiguration) []metav1.OwnerReference {
		refs := make([]metav1.OwnerReference, 0, len(m.OwnerReferences))
		for _, r := range m.OwnerReferences {
			ref := metav1.OwnerReference{Controller: r.Controller, BlockOwnerDeletion: r.BlockOwnerDeletion}
			if r.APIVersion != nil {
				ref.APIVersion = *r.APIVersion
			}
			if r.Kind != nil {
				ref.Kind = *r.Kind
			}
			if r.Name != nil {
				ref.Name = *r.Name
			}
			if r.UID != nil {
				ref.UID = *r.UID
			}
			refs = append(refs, ref)
		}
		return refs
	}

	Describe("SetOwnerReferenceForApply", func() {
		It("should set an owner reference on an empty list", func() {
			Expect(controllerutil.SetOwnerReferenceForApply(owner, meta, scheme.Scheme)).To(Succeed())
			Expect(ownerRefOf(meta)).To(ConsistOf(metav1.OwnerReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "foo",
				UID:        "foo-uid",
			}))
		})

		It("should not set controller or blockOwnerDeletion unless asked to", func() {
			Expect(controllerutil.SetOwnerReferenceForApply(owner, meta, scheme.Scheme)).To(Succeed())
			Expect(meta.OwnerReferences).To(HaveLen(1))
			Expect(meta.OwnerReferences[0].Controller).To(BeNil())
			Expect(meta.OwnerReferences[0].BlockOwnerDeletion).To(BeNil())
		})

		It("should set blockOwnerDeletion if it is specified as an option", func() {
			Expect(controllerutil.SetOwnerReferenceForApply(owner, meta, scheme.Scheme, controllerutil.WithBlockOwnerDeletion(true))).To(Succeed())
			Expect(meta.OwnerReferences).To(HaveLen(1))
			Expect(meta.OwnerReferences[0].BlockOwnerDeletion).To(HaveValue(BeTrue()))
		})

		It("should keep existing owner references of other owners", func() {
			other := &appsv1.StatefulSet{
				Kind: "StatefulSet", APIVersion: "apps/v1",
				Name: "bar", Namespace: "default", UID: "bar-uid",
			}
			Expect(controllerutil.SetOwnerReferenceForApply(other, meta, scheme.Scheme)).To(Succeed())
			Expect(controllerutil.SetOwnerReferenceForApply(owner, meta, scheme.Scheme)).To(Succeed())
			Expect(ownerRefOf(meta)).To(HaveLen(2))
		})

		It("should update the existing reference to the same owner instead of adding one", func() {
			Expect(controllerutil.SetOwnerReferenceForApply(owner, meta, scheme.Scheme)).To(Succeed())
			owner.UID = "new-uid"
			Expect(controllerutil.SetOwnerReferenceForApply(owner, meta, scheme.Scheme, controllerutil.WithBlockOwnerDeletion(true))).To(Succeed())
			refs := ownerRefOf(meta)
			Expect(refs).To(HaveLen(1))
			Expect(refs[0].UID).To(Equal(types.UID("new-uid")))
			Expect(refs[0].BlockOwnerDeletion).To(HaveValue(BeTrue()))
		})

		It("should reject a namespaced owner for an object in another namespace", func() {
			meta.Namespace = new("other")
			err := controllerutil.SetOwnerReferenceForApply(owner, meta, scheme.Scheme)
			Expect(err).To(MatchError(ContainSubstring("cross-namespace owner references are disallowed")))
			Expect(meta.OwnerReferences).To(BeEmpty())
		})

		It("should reject a namespaced owner for a cluster scoped object", func() {
			meta.Namespace = nil
			err := controllerutil.SetOwnerReferenceForApply(owner, meta, scheme.Scheme)
			Expect(err).To(MatchError(ContainSubstring("cluster-scoped resource must not have a namespace-scoped owner")))
		})

		It("should accept a cluster scoped owner for a namespaced object", func() {
			node := &corev1.Node{
				Kind: "Node", APIVersion: "v1",
				Name: "node-1", UID: "node-uid",
			}
			Expect(controllerutil.SetOwnerReferenceForApply(node, meta, scheme.Scheme)).To(Succeed())
			Expect(ownerRefOf(meta)).To(HaveLen(1))
		})

		It("should fail when the owner is not registered in the scheme", func() {
			owner := &unregisteredOwner{Name: "foo", Namespace: "default"}
			Expect(controllerutil.SetOwnerReferenceForApply(owner, meta, scheme.Scheme)).NotTo(Succeed())
		})

		It("should fail when the owner is not a runtime.Object", func() {
			err := controllerutil.SetOwnerReferenceForApply(&metav1.ObjectMeta{Name: "foo"}, meta, scheme.Scheme)
			Expect(err).To(MatchError(ContainSubstring("is not a runtime.Object, cannot call SetOwnerReferenceForApply")))
		})

		It("should fail on nil metadata", func() {
			Expect(controllerutil.SetOwnerReferenceForApply(owner, nil, scheme.Scheme)).NotTo(Succeed())
		})
	})

	Describe("SetControllerReferenceForApply", func() {
		It("should set the controller reference with blockOwnerDeletion", func() {
			Expect(controllerutil.SetControllerReferenceForApply(owner, meta, scheme.Scheme)).To(Succeed())
			Expect(ownerRefOf(meta)).To(ConsistOf(metav1.OwnerReference{
				APIVersion:         "apps/v1",
				Kind:               "Deployment",
				Name:               "foo",
				UID:                "foo-uid",
				Controller:         new(true),
				BlockOwnerDeletion: new(true),
			}))
		})

		It("should allow blockOwnerDeletion to be turned off", func() {
			Expect(controllerutil.SetControllerReferenceForApply(owner, meta, scheme.Scheme, controllerutil.WithBlockOwnerDeletion(false))).To(Succeed())
			Expect(meta.OwnerReferences[0].BlockOwnerDeletion).To(HaveValue(BeFalse()))
		})

		It("should be idempotent for the same controller", func() {
			Expect(controllerutil.SetControllerReferenceForApply(owner, meta, scheme.Scheme)).To(Succeed())
			Expect(controllerutil.SetControllerReferenceForApply(owner, meta, scheme.Scheme)).To(Succeed())
			Expect(meta.OwnerReferences).To(HaveLen(1))
		})

		It("should return an AlreadyOwnedError if another controller owns the object", func() {
			other := &appsv1.StatefulSet{
				Kind: "StatefulSet", APIVersion: "apps/v1",
				Name: "bar", Namespace: "default", UID: "bar-uid",
			}
			Expect(controllerutil.SetControllerReferenceForApply(other, meta, scheme.Scheme)).To(Succeed())

			err := controllerutil.SetControllerReferenceForApply(owner, meta, scheme.Scheme)
			var alreadyOwned *controllerutil.AlreadyOwnedError
			Expect(errors.As(err, &alreadyOwned)).To(BeTrue())
			Expect(alreadyOwned.Owner.Name).To(Equal("bar"))
			Expect(alreadyOwned.Object.GetName()).To(Equal("cm"))
			Expect(alreadyOwned.Object.GetNamespace()).To(Equal("default"))
			Expect(ownerRefOf(meta)).To(HaveLen(1))
		})

		It("should allow a controller next to non-controller owners", func() {
			other := &appsv1.StatefulSet{
				Kind: "StatefulSet", APIVersion: "apps/v1",
				Name: "bar", Namespace: "default", UID: "bar-uid",
			}
			Expect(controllerutil.SetOwnerReferenceForApply(other, meta, scheme.Scheme)).To(Succeed())
			Expect(controllerutil.SetControllerReferenceForApply(owner, meta, scheme.Scheme)).To(Succeed())
			Expect(ownerRefOf(meta)).To(HaveLen(2))
		})

		It("should reject cross namespace owners", func() {
			meta.Namespace = new("other")
			Expect(controllerutil.SetControllerReferenceForApply(owner, meta, scheme.Scheme)).NotTo(Succeed())
		})

		It("should fail on nil metadata", func() {
			Expect(controllerutil.SetControllerReferenceForApply(owner, nil, scheme.Scheme)).NotTo(Succeed())
		})
	})

	Describe("HasControllerReferenceForApply and HasOwnerReferenceForApply", func() {
		It("should report no references on empty metadata", func() {
			Expect(controllerutil.HasControllerReferenceForApply(meta)).To(BeFalse())
			Expect(controllerutil.HasControllerReferenceForApply(nil)).To(BeFalse())
			has, err := controllerutil.HasOwnerReferenceForApply(meta, owner, scheme.Scheme)
			Expect(err).NotTo(HaveOccurred())
			Expect(has).To(BeFalse())
		})

		It("should find a plain owner reference but not report it as a controller", func() {
			Expect(controllerutil.SetOwnerReferenceForApply(owner, meta, scheme.Scheme)).To(Succeed())
			Expect(controllerutil.HasControllerReferenceForApply(meta)).To(BeFalse())
			has, err := controllerutil.HasOwnerReferenceForApply(meta, owner, scheme.Scheme)
			Expect(err).NotTo(HaveOccurred())
			Expect(has).To(BeTrue())
		})

		It("should report a controller reference", func() {
			Expect(controllerutil.SetControllerReferenceForApply(owner, meta, scheme.Scheme)).To(Succeed())
			Expect(controllerutil.HasControllerReferenceForApply(meta)).To(BeTrue())
		})

		It("should not match an owner with a different name or kind", func() {
			Expect(controllerutil.SetOwnerReferenceForApply(owner, meta, scheme.Scheme)).To(Succeed())

			renamed := owner.DeepCopy()
			renamed.Name = "other"
			has, err := controllerutil.HasOwnerReferenceForApply(meta, renamed, scheme.Scheme)
			Expect(err).NotTo(HaveOccurred())
			Expect(has).To(BeFalse())

			sts := &appsv1.StatefulSet{
				Kind: "StatefulSet", APIVersion: "apps/v1",
				Name: "foo", Namespace: "default",
			}
			has, err = controllerutil.HasOwnerReferenceForApply(meta, sts, scheme.Scheme)
			Expect(err).NotTo(HaveOccurred())
			Expect(has).To(BeFalse())
		})

		It("should return an error for a type that is not in the scheme", func() {
			_, err := controllerutil.HasOwnerReferenceForApply(meta, &unregisteredOwner{}, scheme.Scheme)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("RemoveOwnerReferenceForApply", func() {
		It("should remove only the reference of the given owner", func() {
			other := &appsv1.StatefulSet{
				Kind: "StatefulSet", APIVersion: "apps/v1",
				Name: "bar", Namespace: "default", UID: "bar-uid",
			}
			Expect(controllerutil.SetOwnerReferenceForApply(owner, meta, scheme.Scheme)).To(Succeed())
			Expect(controllerutil.SetOwnerReferenceForApply(other, meta, scheme.Scheme)).To(Succeed())

			Expect(controllerutil.RemoveOwnerReferenceForApply(owner, meta, scheme.Scheme)).To(Succeed())
			Expect(ownerRefOf(meta)).To(ConsistOf(HaveField("Name", "bar")))
		})

		It("should fail when there are no owner references", func() {
			Expect(controllerutil.RemoveOwnerReferenceForApply(owner, meta, scheme.Scheme)).NotTo(Succeed())
		})

		It("should fail when the owner has no reference", func() {
			other := &appsv1.StatefulSet{
				Kind: "StatefulSet", APIVersion: "apps/v1",
				Name: "bar", Namespace: "default", UID: "bar-uid",
			}
			Expect(controllerutil.SetOwnerReferenceForApply(other, meta, scheme.Scheme)).To(Succeed())
			Expect(controllerutil.RemoveOwnerReferenceForApply(owner, meta, scheme.Scheme)).NotTo(Succeed())
			Expect(meta.OwnerReferences).To(HaveLen(1))
		})

		It("should fail on nil metadata", func() {
			Expect(controllerutil.RemoveOwnerReferenceForApply(owner, nil, scheme.Scheme)).NotTo(Succeed())
		})
	})

	Describe("RemoveControllerReferenceForApply", func() {
		It("should remove the controller reference", func() {
			Expect(controllerutil.SetControllerReferenceForApply(owner, meta, scheme.Scheme)).To(Succeed())
			Expect(controllerutil.RemoveControllerReferenceForApply(owner, meta, scheme.Scheme)).To(Succeed())
			Expect(meta.OwnerReferences).To(BeEmpty())
			Expect(controllerutil.HasControllerReferenceForApply(meta)).To(BeFalse())
		})

		It("should fail when nothing is a controller", func() {
			Expect(controllerutil.SetOwnerReferenceForApply(owner, meta, scheme.Scheme)).To(Succeed())
			Expect(controllerutil.RemoveControllerReferenceForApply(owner, meta, scheme.Scheme)).NotTo(Succeed())
			Expect(meta.OwnerReferences).To(HaveLen(1))
		})

		It("should fail when the given owner is not the controller", func() {
			other := &appsv1.StatefulSet{
				Kind: "StatefulSet", APIVersion: "apps/v1",
				Name: "bar", Namespace: "default", UID: "bar-uid",
			}
			Expect(controllerutil.SetControllerReferenceForApply(other, meta, scheme.Scheme)).To(Succeed())
			Expect(controllerutil.SetOwnerReferenceForApply(owner, meta, scheme.Scheme)).To(Succeed())

			err := controllerutil.RemoveControllerReferenceForApply(owner, meta, scheme.Scheme)
			Expect(err).To(MatchError(ContainSubstring("is not the controller reference")))
			Expect(meta.OwnerReferences).To(HaveLen(2))
		})

		It("should fail when the owner has no reference", func() {
			other := &appsv1.StatefulSet{
				Kind: "StatefulSet", APIVersion: "apps/v1",
				Name: "bar", Namespace: "default", UID: "bar-uid",
			}
			Expect(controllerutil.SetControllerReferenceForApply(other, meta, scheme.Scheme)).To(Succeed())
			Expect(controllerutil.RemoveControllerReferenceForApply(owner, meta, scheme.Scheme)).NotTo(Succeed())
		})

		It("should fail on nil metadata", func() {
			Expect(controllerutil.RemoveControllerReferenceForApply(owner, nil, scheme.Scheme)).NotTo(Succeed())
		})
	})

	Describe("finalizers", func() {
		It("should add a finalizer once", func() {
			Expect(controllerutil.AddFinalizerForApply(meta, "example.com/fin")).To(BeTrue())
			Expect(controllerutil.AddFinalizerForApply(meta, "example.com/fin")).To(BeFalse())
			Expect(meta.Finalizers).To(Equal([]string{"example.com/fin"}))
		})

		It("should keep other finalizers when adding", func() {
			Expect(controllerutil.AddFinalizerForApply(meta, "a")).To(BeTrue())
			Expect(controllerutil.AddFinalizerForApply(meta, "b")).To(BeTrue())
			Expect(meta.Finalizers).To(Equal([]string{"a", "b"}))
		})

		It("should report whether a finalizer is present", func() {
			Expect(controllerutil.ContainsFinalizerForApply(meta, "a")).To(BeFalse())
			Expect(controllerutil.ContainsFinalizerForApply(nil, "a")).To(BeFalse())
			controllerutil.AddFinalizerForApply(meta, "a")
			Expect(controllerutil.ContainsFinalizerForApply(meta, "a")).To(BeTrue())
			Expect(controllerutil.ContainsFinalizerForApply(meta, "b")).To(BeFalse())
		})

		It("should remove a finalizer and report the change", func() {
			controllerutil.AddFinalizerForApply(meta, "a")
			controllerutil.AddFinalizerForApply(meta, "b")
			Expect(controllerutil.RemoveFinalizerForApply(meta, "a")).To(BeTrue())
			Expect(controllerutil.RemoveFinalizerForApply(meta, "a")).To(BeFalse())
			Expect(meta.Finalizers).To(Equal([]string{"b"}))
		})

		It("should not touch the finalizers when removing one that is absent", func() {
			Expect(controllerutil.RemoveFinalizerForApply(meta, "a")).To(BeFalse())
			Expect(meta.Finalizers).To(BeEmpty())
		})
	})

	Describe("applied to a cluster", func() {
		const fieldOwner = client.FieldOwner("controllerutil-apply-test")

		var ownerCM *corev1.ConfigMap

		BeforeEach(func(ctx SpecContext) {
			ownerCM = &corev1.ConfigMap{GenerateName: "apply-owner-", Namespace: "default"}
			Expect(c.Create(ctx, ownerCM)).To(Succeed())
			ownerCM.TypeMeta = metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"}
			DeferCleanup(func(ctx SpecContext) { Expect(client.IgnoreNotFound(c.Delete(ctx, ownerCM))).To(Succeed()) })
		})

		It("should apply the owner reference and finalizer to the server", func(ctx SpecContext) {
			cm := corev1applyconfigurations.ConfigMap("apply-owned", "default").WithData(map[string]string{"k": "v"})
			Expect(controllerutil.SetControllerReferenceForApply(ownerCM, cm.ObjectMetaApplyConfiguration, scheme.Scheme)).To(Succeed())
			Expect(controllerutil.AddFinalizerForApply(cm.ObjectMetaApplyConfiguration, "example.com/fin")).To(BeTrue())
			Expect(c.Apply(ctx, cm, fieldOwner, client.ForceOwnership)).To(Succeed())
			DeferCleanup(func(ctx SpecContext) {
				got := &corev1.ConfigMap{}
				Expect(c.Get(ctx, types.NamespacedName{Namespace: "default", Name: "apply-owned"}, got)).To(Succeed())
				got.Finalizers = nil
				Expect(c.Update(ctx, got)).To(Succeed())
				Expect(client.IgnoreNotFound(c.Delete(ctx, got))).To(Succeed())
			})

			got := &corev1.ConfigMap{}
			Expect(c.Get(ctx, types.NamespacedName{Namespace: "default", Name: "apply-owned"}, got)).To(Succeed())
			Expect(got.Data).To(HaveKeyWithValue("k", "v"))
			Expect(got.Finalizers).To(ConsistOf("example.com/fin"))
			Expect(got.OwnerReferences).To(ConsistOf(metav1.OwnerReference{
				APIVersion:         "v1",
				Kind:               "ConfigMap",
				Name:               ownerCM.Name,
				UID:                ownerCM.UID,
				Controller:         new(true),
				BlockOwnerDeletion: new(true),
			}))
			Expect(metav1.IsControlledBy(got, ownerCM)).To(BeTrue())

			By("dropping the finalizer from the configuration and applying again")
			Expect(controllerutil.RemoveFinalizerForApply(cm.ObjectMetaApplyConfiguration, "example.com/fin")).To(BeTrue())
			Expect(c.Apply(ctx, cm, fieldOwner, client.ForceOwnership)).To(Succeed())
			Expect(c.Get(ctx, types.NamespacedName{Namespace: "default", Name: "apply-owned"}, got)).To(Succeed())
			Expect(got.Finalizers).To(BeEmpty())
		})

		It("should refuse a cross namespace owner before anything is sent", func() {
			cm := corev1applyconfigurations.ConfigMap("apply-cross-ns", "other")
			Expect(controllerutil.SetControllerReferenceForApply(ownerCM, cm.ObjectMetaApplyConfiguration, scheme.Scheme)).NotTo(Succeed())
			Expect(cm.OwnerReferences).To(BeEmpty())
		})
	})
})
