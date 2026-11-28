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

package controllerutil

import (
	"errors"
	"fmt"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	metav1applyconfigurations "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// The helpers in this file mirror the owner reference and finalizer helpers in
// controllerutil.go for objects that are built as apply configurations and sent
// with Client.Apply. They work on the metadata of the apply configuration, which
// every generated typed apply configuration exposes as its embedded
// ObjectMetaApplyConfiguration field:
//
//	cm := corev1ac.ConfigMap("foo", "default")
//	err := controllerutil.SetControllerReferenceForApply(owner, cm.ObjectMetaApplyConfiguration, scheme)
//
// An apply configuration only holds the fields the caller wants to own, so there
// is nothing to read back from the server. Removing an owner reference or a
// finalizer from the configuration makes the next apply drop the field manager's
// ownership of it.

var errNilApplyMeta = errors.New("the ObjectMetaApplyConfiguration must not be nil, build the apply configuration with its constructor")

// SetControllerReferenceForApply sets owner as a Controller OwnerReference in the
// metadata of an apply configuration. The metadata must carry the name and, for a
// namespaced object, the namespace of the object that is going to be applied.
// Since only one OwnerReference can be a controller, it returns an
// AlreadyOwnedError if the configuration already has a different controller
// reference.
func SetControllerReferenceForApply(owner metav1.Object, controlled *metav1applyconfigurations.ObjectMetaApplyConfiguration, scheme *runtime.Scheme, opts ...OwnerReferenceOption) error {
	if controlled == nil {
		return errNilApplyMeta
	}
	if err := validateOwnerForApply(owner, controlled); err != nil {
		return err
	}
	gvk, err := ownerGVK(owner, scheme, "SetControllerReferenceForApply")
	if err != nil {
		return err
	}

	ref := metav1.OwnerReference{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               owner.GetName(),
		UID:                owner.GetUID(),
		BlockOwnerDeletion: new(true),
		Controller:         new(true),
	}
	for _, opt := range opts {
		opt(&ref)
	}

	refs := ownerReferencesFromApply(controlled)
	if idx := controllerRefIndex(refs); idx != -1 && !referSameObject(refs[idx], ref) {
		return newAlreadyOwnedError(applyMetaObject(controlled), refs[idx])
	}

	upsertOwnerReferenceForApply(controlled, ref)
	return nil
}

// SetOwnerReferenceForApply makes sure the metadata of an apply configuration
// contains an owner reference to the given owner, without making it the
// controller. If a reference to the same object already exists, it is
// overwritten with the newly provided version.
func SetOwnerReferenceForApply(owner metav1.Object, object *metav1applyconfigurations.ObjectMetaApplyConfiguration, scheme *runtime.Scheme, opts ...OwnerReferenceOption) error {
	if object == nil {
		return errNilApplyMeta
	}
	if err := validateOwnerForApply(owner, object); err != nil {
		return err
	}
	gvk, err := ownerGVK(owner, scheme, "SetOwnerReferenceForApply")
	if err != nil {
		return err
	}

	ref := metav1.OwnerReference{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
		Name:       owner.GetName(),
		UID:        owner.GetUID(),
	}
	for _, opt := range opts {
		opt(&ref)
	}

	upsertOwnerReferenceForApply(object, ref)
	return nil
}

// RemoveOwnerReferenceForApply removes the owner reference to the given owner from
// the metadata of an apply configuration.
func RemoveOwnerReferenceForApply(owner metav1.Object, object *metav1applyconfigurations.ObjectMetaApplyConfiguration, scheme *runtime.Scheme) error {
	if object == nil {
		return errNilApplyMeta
	}
	if len(object.OwnerReferences) == 0 {
		return fmt.Errorf("%T does not have any owner references", object)
	}
	gvk, err := ownerGVK(owner, scheme, "RemoveOwnerReferenceForApply")
	if err != nil {
		return err
	}

	refs := ownerReferencesFromApply(object)
	index := indexOwnerRef(refs, metav1.OwnerReference{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
		Name:       owner.GetName(),
	})
	if index == -1 {
		return fmt.Errorf("%T does not have an owner reference for %T", object, owner)
	}

	object.OwnerReferences = slices.Delete(object.OwnerReferences, index, index+1)
	return nil
}

// RemoveControllerReferenceForApply removes the owner reference to the given
// owner from the metadata of an apply configuration, if that reference is the
// controller reference.
func RemoveControllerReferenceForApply(owner metav1.Object, object *metav1applyconfigurations.ObjectMetaApplyConfiguration, scheme *runtime.Scheme) error {
	if object == nil {
		return errNilApplyMeta
	}
	if !HasControllerReferenceForApply(object) {
		return fmt.Errorf("%T does not have a owner reference with controller equals true", object)
	}
	gvk, err := ownerGVK(owner, scheme, "RemoveControllerReferenceForApply")
	if err != nil {
		return err
	}

	refs := ownerReferencesFromApply(object)
	index := indexOwnerRef(refs, metav1.OwnerReference{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
		Name:       owner.GetName(),
	})
	if index == -1 {
		return fmt.Errorf("%T does not have an controller reference for %T", object, owner)
	}
	if !ptr.Deref(refs[index].Controller, false) {
		return fmt.Errorf("%T owner is not the controller reference for %T", owner, object)
	}

	object.OwnerReferences = slices.Delete(object.OwnerReferences, index, index+1)
	return nil
}

// HasControllerReferenceForApply returns true if the metadata of an apply
// configuration has an owner reference with controller equal to true.
func HasControllerReferenceForApply(object *metav1applyconfigurations.ObjectMetaApplyConfiguration) bool {
	if object == nil {
		return false
	}
	return controllerRefIndex(ownerReferencesFromApply(object)) != -1
}

// HasOwnerReferenceForApply returns true if the metadata of an apply configuration
// contains an owner reference that matches the group, kind and name of owner.
func HasOwnerReferenceForApply(object *metav1applyconfigurations.ObjectMetaApplyConfiguration, owner client.Object, scheme *runtime.Scheme) (bool, error) {
	gvk, err := apiutil.GVKForObject(owner, scheme)
	if err != nil {
		return false, err
	}
	if object == nil {
		return false, nil
	}
	idx := indexOwnerRef(ownerReferencesFromApply(object), metav1.OwnerReference{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
		Name:       owner.GetName(),
	})
	return idx != -1, nil
}

// AddFinalizerForApply adds the provided finalizer to the metadata of an apply
// configuration if it is not present. It returns an indication of whether it
// updated the list of finalizers. The metadata must not be nil.
func AddFinalizerForApply(object *metav1applyconfigurations.ObjectMetaApplyConfiguration, finalizer string) (finalizersUpdated bool) {
	if slices.Contains(object.Finalizers, finalizer) {
		return false
	}
	object.WithFinalizers(finalizer)
	return true
}

// RemoveFinalizerForApply removes the provided finalizer from the metadata of an
// apply configuration if it is present. It returns an indication of whether it
// updated the list of finalizers. The metadata must not be nil.
func RemoveFinalizerForApply(object *metav1applyconfigurations.ObjectMetaApplyConfiguration, finalizer string) (finalizersUpdated bool) {
	before := len(object.Finalizers)
	object.Finalizers = slices.DeleteFunc(object.Finalizers, func(f string) bool { return f == finalizer })
	return len(object.Finalizers) != before
}

// ContainsFinalizerForApply checks that the provided finalizer is present in the
// metadata of an apply configuration.
func ContainsFinalizerForApply(object *metav1applyconfigurations.ObjectMetaApplyConfiguration, finalizer string) bool {
	if object == nil {
		return false
	}
	return slices.Contains(object.Finalizers, finalizer)
}

func ownerGVK(owner metav1.Object, scheme *runtime.Scheme, caller string) (gvk schema.GroupVersionKind, err error) {
	ro, ok := owner.(runtime.Object)
	if !ok {
		return gvk, fmt.Errorf("%T is not a runtime.Object, cannot call %s", owner, caller)
	}
	return apiutil.GVKForObject(ro, scheme)
}

// validateOwnerForApply applies the same namespace rules as validateOwner to an
// apply configuration, which has no metav1.Object accessors of its own.
func validateOwnerForApply(owner metav1.Object, object *metav1applyconfigurations.ObjectMetaApplyConfiguration) error {
	return validateOwner(owner, applyMetaObject(object))
}

// applyMetaObject returns a metav1.Object carrying the name and namespace of an
// apply configuration, for use in validation and error messages.
func applyMetaObject(object *metav1applyconfigurations.ObjectMetaApplyConfiguration) metav1.Object {
	return &metav1.ObjectMeta{
		Name:      ptr.Deref(object.Name, ""),
		Namespace: ptr.Deref(object.Namespace, ""),
	}
}

func ownerReferencesFromApply(object *metav1applyconfigurations.ObjectMetaApplyConfiguration) []metav1.OwnerReference {
	refs := make([]metav1.OwnerReference, 0, len(object.OwnerReferences))
	for _, r := range object.OwnerReferences {
		refs = append(refs, metav1.OwnerReference{
			APIVersion:         ptr.Deref(r.APIVersion, ""),
			Kind:               ptr.Deref(r.Kind, ""),
			Name:               ptr.Deref(r.Name, ""),
			UID:                ptr.Deref(r.UID, ""),
			Controller:         r.Controller,
			BlockOwnerDeletion: r.BlockOwnerDeletion,
		})
	}
	return refs
}

func controllerRefIndex(refs []metav1.OwnerReference) int {
	return slices.IndexFunc(refs, func(r metav1.OwnerReference) bool {
		return ptr.Deref(r.Controller, false)
	})
}

func upsertOwnerReferenceForApply(object *metav1applyconfigurations.ObjectMetaApplyConfiguration, ref metav1.OwnerReference) {
	ac := metav1applyconfigurations.OwnerReference().
		WithAPIVersion(ref.APIVersion).
		WithKind(ref.Kind).
		WithName(ref.Name).
		WithUID(ref.UID)
	if ref.Controller != nil {
		ac.WithController(*ref.Controller)
	}
	if ref.BlockOwnerDeletion != nil {
		ac.WithBlockOwnerDeletion(*ref.BlockOwnerDeletion)
	}

	if idx := indexOwnerRef(ownerReferencesFromApply(object), ref); idx != -1 {
		object.OwnerReferences[idx] = *ac
		return
	}
	object.WithOwnerReferences(ac)
}
