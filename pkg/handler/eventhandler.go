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

package handler

import (
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// EventHandler enqueues reconcile.Requests in response to events (e.g. Pod Create).  EventHandlers map an Event
// for one object to trigger Reconciles for either the same object or different objects - e.g. if there is an
// Event for object with type Foo (using source.KindSource) then reconcile one or more object(s) with type Bar.
//
// Identical reconcile.Requests will be batched together through the queuing mechanism before reconcile is called.
//
// * Use Enqueue to reconcile the object the event is for
// - do this for events for the type the Controller Reconciles. (e.g. Deployment for a Deployment Controller)
//
// * Use EnqueueOwner to reconcile the owner of the object the event is for
// - do this for events for the types the Controller creates.  (e.g. ReplicaSets created by a Deployment Controller)
//
// * Use EnqueueMappendHandler to transform an event for an object to a reconcile of an object
// of a different type - do this for events for types the Controller may be interested in, but doesn't create.
// (e.g. If Foo responds to cluster size events, map Node events to Foo objects.)
//
// Unless you are implementing your own EventHandler, you can ignore the functions on the EventHandler interface.
// Most users shouldn't need to implement their own EventHandler.
type EventHandler interface {
	// Create is called in response to an create event - e.g. Pod Creation.
	Create(workqueue.RateLimitingInterface, event.CreateEvent)

	// Update is called in response to an update event -  e.g. Pod Updated.
	Update(workqueue.RateLimitingInterface, event.UpdateEvent)

	// Delete is called in response to a delete event - e.g. Pod Deleted.
	Delete(workqueue.RateLimitingInterface, event.DeleteEvent)

	// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
	// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
	Generic(workqueue.RateLimitingInterface, event.GenericEvent)
}

var _ EventHandler = Funcs{}

// Funcs allows specifying a subset of EventHandler functions are fields.
type Funcs struct {
	// Create is called in response to an add event.  Defaults to no-op.
	// RateLimitingInterface is used to enqueue reconcile.Requests.
	CreateFunc func(workqueue.RateLimitingInterface, event.CreateEvent)

	// Update is called in response to an update event.  Defaults to no-op.
	// RateLimitingInterface is used to enqueue reconcile.Requests.
	UpdateFunc func(workqueue.RateLimitingInterface, event.UpdateEvent)

	// Delete is called in response to a delete event.  Defaults to no-op.
	// RateLimitingInterface is used to enqueue reconcile.Requests.
	DeleteFunc func(workqueue.RateLimitingInterface, event.DeleteEvent)

	// GenericFunc is called in response to a generic event.  Defaults to no-op.
	// RateLimitingInterface is used to enqueue reconcile.Requests.
	GenericFunc func(workqueue.RateLimitingInterface, event.GenericEvent)
}

// Create implements EventHandler
func (h Funcs) Create(q workqueue.RateLimitingInterface, e event.CreateEvent) {
	if h.CreateFunc != nil {
		h.CreateFunc(q, e)
	}
}

// Delete implements EventHandler
func (h Funcs) Delete(q workqueue.RateLimitingInterface, e event.DeleteEvent) {
	if h.DeleteFunc != nil {
		h.DeleteFunc(q, e)
	}
}

// Update implements EventHandler
func (h Funcs) Update(q workqueue.RateLimitingInterface, e event.UpdateEvent) {
	if h.UpdateFunc != nil {
		h.UpdateFunc(q, e)
	}
}

// Generic implements EventHandler
func (h Funcs) Generic(q workqueue.RateLimitingInterface, e event.GenericEvent) {
	if h.GenericFunc != nil {
		h.GenericFunc(q, e)
	}
}
