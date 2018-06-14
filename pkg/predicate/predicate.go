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

import "sigs.k8s.io/controller-runtime/pkg/event"

// Predicate filters events before enqueuing the keys.
type Predicate interface {
	// Create returns true if the Create event should be processed
	Create(event.CreateEvent) bool

	// Delete returns true if the Delete event should be processed
	Delete(event.DeleteEvent) bool

	// Update returns true if the Update event should be processed
	Update(event.UpdateEvent) bool

	// Generic returns true if the Generic event should be processed
	Generic(event.GenericEvent) bool
}

var _ Predicate = Funcs{}

// Funcs is a function that implements Predicate.
type Funcs struct {
	// Create returns true if the Create event should be processed
	CreateFunc func(event.CreateEvent) bool

	// Delete returns true if the Delete event should be processed
	DeleteFunc func(event.DeleteEvent) bool

	// Update returns true if the Update event should be processed
	UpdateFunc func(event.UpdateEvent) bool

	// Generic returns true if the Generic event should be processed
	GenericFunc func(event.GenericEvent) bool
}

// Create implements Predicate
func (p Funcs) Create(e event.CreateEvent) bool {
	if p.CreateFunc != nil {
		return p.CreateFunc(e)
	}
	return true
}

// Delete implements Predicate
func (p Funcs) Delete(e event.DeleteEvent) bool {
	if p.DeleteFunc != nil {
		return p.DeleteFunc(e)
	}
	return true
}

// Update implements Predicate
func (p Funcs) Update(e event.UpdateEvent) bool {
	if p.UpdateFunc != nil {
		return p.UpdateFunc(e)
	}
	return true
}

// Generic implements Predicate
func (p Funcs) Generic(e event.GenericEvent) bool {
	if p.GenericFunc != nil {
		return p.GenericFunc(e)
	}
	return true
}
