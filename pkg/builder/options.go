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

package builder

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// {{{ "Functional" Option Interfaces

// ForOption is some configuration that modifies options for a For request.
type ForOption[T client.ObjectConstraint] interface {
	// ApplyToFor applies this configuration to the given for input.
	ApplyToFor(*ForInput[T])
}

// OwnsOption is some configuration that modifies options for a owns request.
type OwnsOption[T client.ObjectConstraint] interface {
	// ApplyToOwns applies this configuration to the given owns input.
	ApplyToOwns(*OwnsInput[T])
}

// WatchesOption is some configuration that modifies options for a watches request.
type WatchesOption[T client.ObjectConstraint] interface {
	// ApplyToWatches applies this configuration to the given watches options.
	ApplyToWatches(*WatchesInput[T])
}

type ApplyAll[T client.ObjectConstraint] interface {
	ForOption[T]
	OwnsOption[T]
	WatchesOption[T]
}

// }}}

// {{{ Multi-Type Options

// WithPredicates sets the given predicates list.
func WithPredicates[T client.ObjectConstraint](predicates ...predicate.Predicate[T]) Predicates[T] {
	return Predicates[T]{
		predicates: predicates,
	}
}

// Predicates filters events before enqueuing the keys.
type Predicates[T client.ObjectConstraint] struct {
	predicates []predicate.Predicate[T]
}

// ApplyToFor applies this configuration to the given ForInput options.
func (w Predicates[T]) ApplyToFor(opts *ForInput[T]) {
	opts.predicates = w.predicates
}

// ApplyToOwns applies this configuration to the given OwnsInput options.
func (w Predicates[T]) ApplyToOwns(opts *OwnsInput[T]) {
	opts.predicates = w.predicates
}

// ApplyToWatches applies this configuration to the given WatchesInput options.
func (w Predicates[T]) ApplyToWatches(opts *WatchesInput[T]) {
	opts.predicates = w.predicates
}

// TODO(nikola-jokic)
// var (
// 	_ ForOption     = &Predicates{}
// 	_ OwnsOption    = &Predicates{}
// 	_ WatchesOption = &Predicates{}
// )

// }}}

// {{{ For & Owns Dual-Type options

// asProjection configures the projection (currently only metadata) on the input.
// Currently only metadata is supported.  We might want to expand
// this to arbitrary non-special local projections in the future.
type projectAs[T client.ObjectConstraint] objectProjection

// ApplyToFor applies this configuration to the given ForInput options.
func (p projectAs[T]) ApplyToFor(opts *ForInput[T]) {
	opts.objectProjection = objectProjection(p)
}

// ApplyToOwns applies this configuration to the given OwnsInput options.
func (p projectAs[T]) ApplyToOwns(opts *OwnsInput[T]) {
	opts.objectProjection = objectProjection(p)
}

// ApplyToWatches applies this configuration to the given WatchesInput options.
func (p projectAs[T]) ApplyToWatches(opts *WatchesInput[T]) {
	opts.objectProjection = objectProjection(p)
}

// OnlyMetadata tells the controller to *only* cache metadata, and to watch
// the API server in metadata-only form.  This is useful when watching
// lots of objects, really big objects, or objects for which you only know
// the GVK, but not the structure.  You'll need to pass
// metav1.PartialObjectMetadata to the client when fetching objects in your
// reconciler, otherwise you'll end up with a duplicate structured or
// unstructured cache.
//
// When watching a resource with OnlyMetadata, for example the v1.Pod, you
// should not Get and List using the v1.Pod type. Instead, you should use
// the special metav1.PartialObjectMetadata type.
//
// ❌ Incorrect:
//
//	pod := &v1.Pod{}
//	mgr.GetClient().Get(ctx, nsAndName, pod)
//
// ✅ Correct:
//
//	pod := &metav1.PartialObjectMetadata{}
//	pod.SetGroupVersionKind(schema.GroupVersionKind{
//	    Group:   "",
//	    Version: "v1",
//	    Kind:    "Pod",
//	})
//	mgr.GetClient().Get(ctx, nsAndName, pod)
//
// In the first case, controller-runtime will create another cache for the
// concrete type on top of the metadata cache; this increases memory
// consumption and leads to race conditions as caches are not in sync.
func OnlyMetadata[T client.ObjectConstraint]() ApplyAll[T] {
	return projectAs[T](projectAsMetadata)
}

// }}}

// MatchEveryOwner determines whether the watch should be filtered based on
// controller ownership. As in, when the OwnerReference.Controller field is set.
//
// If passed as an option,
// the handler receives notification for every owner of the object with the given type.
// If unset (default), the handler receives notification only for the first
// OwnerReference with `Controller: true`.
func MatchEveryOwner[T client.ObjectConstraint]() *matchEveryOwner[T] {
	return &matchEveryOwner[T]{}
}

type matchEveryOwner[T client.ObjectConstraint] struct{}

// ApplyToOwns applies this configuration to the given OwnsInput options.
func (o matchEveryOwner[T]) ApplyToOwns(opts *OwnsInput[T]) {
	opts.matchEveryOwner = true
}
