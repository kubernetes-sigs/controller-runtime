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

package predicate

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Predicate filters events before enqueuing the keys.
type Predicate interface {
	// Create returns true if the Create event should be processed
	Create(context.Context, event.CreateEvent) bool

	// Delete returns true if the Delete event should be processed
	Delete(context.Context, event.DeleteEvent) bool

	// Update returns true if the Update event should be processed
	Update(context.Context, event.UpdateEvent) bool

	// Generic returns true if the Generic event should be processed
	Generic(context.Context, event.GenericEvent) bool
}

var _ Predicate = Funcs{}
var _ Predicate = ResourceVersionChangedPredicate{}
var _ Predicate = GenerationChangedPredicate{}
var _ Predicate = or{}
var _ Predicate = and{}

// Funcs is a function that implements Predicate.
type Funcs struct {
	// Create returns true if the Create event should be processed
	CreateFunc func(context.Context, event.CreateEvent) bool

	// Delete returns true if the Delete event should be processed
	DeleteFunc func(context.Context, event.DeleteEvent) bool

	// Update returns true if the Update event should be processed
	UpdateFunc func(context.Context, event.UpdateEvent) bool

	// Generic returns true if the Generic event should be processed
	GenericFunc func(context.Context, event.GenericEvent) bool
}

// Create implements Predicate
func (p Funcs) Create(ctx context.Context, e event.CreateEvent) bool {
	if p.CreateFunc != nil {
		return p.CreateFunc(ctx, e)
	}
	return true
}

// Delete implements Predicate
func (p Funcs) Delete(ctx context.Context, e event.DeleteEvent) bool {
	if p.DeleteFunc != nil {
		return p.DeleteFunc(ctx, e)
	}
	return true
}

// Update implements Predicate
func (p Funcs) Update(ctx context.Context, e event.UpdateEvent) bool {
	if p.UpdateFunc != nil {
		return p.UpdateFunc(ctx, e)
	}
	return true
}

// Generic implements Predicate
func (p Funcs) Generic(ctx context.Context, e event.GenericEvent) bool {
	if p.GenericFunc != nil {
		return p.GenericFunc(ctx, e)
	}
	return true
}

// NewPredicateFuncs returns a predicate funcs that applies the given filter function
// on CREATE, UPDATE, DELETE and GENERIC events. For UPDATE events, the filter is applied
// to the new object.
func NewPredicateFuncs(filter func(ctx context.Context, object client.Object) bool) Funcs {
	return Funcs{
		CreateFunc: func(ctx context.Context, e event.CreateEvent) bool {
			return filter(ctx, e.Object)
		},
		UpdateFunc: func(ctx context.Context, e event.UpdateEvent) bool {
			return filter(ctx, e.ObjectNew)
		},
		DeleteFunc: func(ctx context.Context, e event.DeleteEvent) bool {
			return filter(ctx, e.Object)
		},
		GenericFunc: func(ctx context.Context, e event.GenericEvent) bool {
			return filter(ctx, e.Object)
		},
	}
}

// ResourceVersionChangedPredicate implements a default update predicate function on resource version change
type ResourceVersionChangedPredicate struct {
	Funcs
}

// Update implements default UpdateEvent filter for validating resource version change
func (ResourceVersionChangedPredicate) Update(ctx context.Context, e event.UpdateEvent) bool {
	log := log.FromContext(ctx)

	if e.ObjectOld == nil {
		log.Error(nil, "UpdateEvent has no old metadata", "event", e)
		return false
	}
	if e.ObjectOld == nil {
		log.Error(nil, "GenericEvent has no old runtime object to update", "event", e)
		return false
	}
	if e.ObjectNew == nil {
		log.Error(nil, "GenericEvent has no new runtime object for update", "event", e)
		return false
	}
	if e.ObjectNew == nil {
		log.Error(nil, "UpdateEvent has no new metadata", "event", e)
		return false
	}
	return e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion()
}

// GenerationChangedPredicate implements a default update predicate function on Generation change.
//
// This predicate will skip update events that have no change in the object's metadata.generation field.
// The metadata.generation field of an object is incremented by the API server when writes are made to the spec field of an object.
// This allows a controller to ignore update events where the spec is unchanged, and only the metadata and/or status fields are changed.
//
// For CustomResource objects the Generation is only incremented when the status subresource is enabled.
//
// Caveats:
//
// * The assumption that the Generation is incremented only on writing to the spec does not hold for all APIs.
// E.g For Deployment objects the Generation is also incremented on writes to the metadata.annotations field.
// For object types other than CustomResources be sure to verify which fields will trigger a Generation increment when they are written to.
//
// * With this predicate, any update events with writes only to the status field will not be reconciled.
// So in the event that the status block is overwritten or wiped by someone else the controller will not self-correct to restore the correct status.
type GenerationChangedPredicate struct {
	Funcs
}

// Update implements default UpdateEvent filter for validating generation change
func (GenerationChangedPredicate) Update(ctx context.Context, e event.UpdateEvent) bool {
	log := log.FromContext(ctx)

	if e.ObjectOld == nil {
		log.Error(nil, "Update event has no old metadata", "event", e)
		return false
	}
	if e.ObjectOld == nil {
		log.Error(nil, "Update event has no old runtime object to update", "event", e)
		return false
	}
	if e.ObjectNew == nil {
		log.Error(nil, "Update event has no new runtime object for update", "event", e)
		return false
	}
	if e.ObjectNew == nil {
		log.Error(nil, "Update event has no new metadata", "event", e)
		return false
	}
	return e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
}

// And returns a composite predicate that implements a logical AND of the predicates passed to it.
func And(predicates ...Predicate) Predicate {
	return and{predicates}
}

type and struct {
	predicates []Predicate
}

func (a and) Create(ctx context.Context, e event.CreateEvent) bool {
	for _, p := range a.predicates {
		if !p.Create(ctx, e) {
			return false
		}
	}
	return true
}

func (a and) Update(ctx context.Context, e event.UpdateEvent) bool {
	for _, p := range a.predicates {
		if !p.Update(ctx, e) {
			return false
		}
	}
	return true
}

func (a and) Delete(ctx context.Context, e event.DeleteEvent) bool {
	for _, p := range a.predicates {
		if !p.Delete(ctx, e) {
			return false
		}
	}
	return true
}

func (a and) Generic(ctx context.Context, e event.GenericEvent) bool {
	for _, p := range a.predicates {
		if !p.Generic(ctx, e) {
			return false
		}
	}
	return true
}

// Or returns a composite predicate that implements a logical OR of the predicates passed to it.
func Or(predicates ...Predicate) Predicate {
	return or{predicates}
}

type or struct {
	predicates []Predicate
}

func (o or) Create(ctx context.Context, e event.CreateEvent) bool {
	for _, p := range o.predicates {
		if p.Create(ctx, e) {
			return true
		}
	}
	return false
}

func (o or) Update(ctx context.Context, e event.UpdateEvent) bool {
	for _, p := range o.predicates {
		if p.Update(ctx, e) {
			return true
		}
	}
	return false
}

func (o or) Delete(ctx context.Context, e event.DeleteEvent) bool {
	for _, p := range o.predicates {
		if p.Delete(ctx, e) {
			return true
		}
	}
	return false
}

func (o or) Generic(ctx context.Context, e event.GenericEvent) bool {
	for _, p := range o.predicates {
		if p.Generic(ctx, e) {
			return true
		}
	}
	return false
}

// LabelSelectorPredicate constructs a Predicate from a LabelSelector.
// Only objects matching the LabelSelector will be admitted.
func LabelSelectorPredicate(s metav1.LabelSelector) (Predicate, error) {
	selector, err := metav1.LabelSelectorAsSelector(&s)
	if err != nil {
		return Funcs{}, err
	}
	return NewPredicateFuncs(func(ctx context.Context, o client.Object) bool {
		return selector.Matches(labels.Set(o.GetLabels()))
	}), nil
}
