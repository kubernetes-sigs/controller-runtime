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
	"context"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// EventHandler enqueues reconcile.Requests in response to events (e.g. Pod Create).  EventHandlers map an Event
// for one object to trigger Reconciles for either the same object or different objects - e.g. if there is an
// Event for object with type Foo (using source.KindSource) then reconcile one or more object(s) with type Bar.
//
// Identical reconcile.Requests will be batched together through the queuing mechanism before reconcile is called.
//
// * Use EnqueueRequestForObject to reconcile the object the event is for
// - do this for events for the type the Controller Reconciles. (e.g. Deployment for a Deployment Controller)
//
// * Use EnqueueRequestForOwner to reconcile the owner of the object the event is for
// - do this for events for the types the Controller creates.  (e.g. ReplicaSets created by a Deployment Controller)
//
// * Use EnqueueRequestsFromMapFunc to transform an event for an object to a reconcile of an object
// of a different type - do this for events for types the Controller may be interested in, but doesn't create.
// (e.g. If Foo responds to cluster size events, map Node events to Foo objects.)
//
// Unless you are implementing your own EventHandler, you can ignore the functions on the EventHandler interface.
// Most users shouldn't need to implement their own EventHandler.
type EventHandler interface {
	// Create is called in response to a create event - e.g. Pod Creation.
	Create(context.Context, event.CreateEvent, workqueue.RateLimitingInterface)

	// Update is called in response to an update event -  e.g. Pod Updated.
	Update(context.Context, event.UpdateEvent, workqueue.RateLimitingInterface)

	// Delete is called in response to a delete event - e.g. Pod Deleted.
	Delete(context.Context, event.DeleteEvent, workqueue.RateLimitingInterface)

	// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
	// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
	Generic(context.Context, event.GenericEvent, workqueue.RateLimitingInterface)
}

// ObjectHandler filters events for type before enqueuing the keys.
type ObjectHandler[T any] interface {
	// Create returns true if the Create event should be processed
	OnCreate(ctx context.Context, obj T, queue workqueue.RateLimitingInterface)

	// Delete returns true if the Delete event should be processed
	OnDelete(ctx context.Context, obj T, queue workqueue.RateLimitingInterface)

	// Update returns true if the Update event should be processed
	OnUpdate(ctx context.Context, old, new T, queue workqueue.RateLimitingInterface)

	// Generic returns true if the Generic event should be processed
	OnGeneric(ctx context.Context, obj T, queue workqueue.RateLimitingInterface)
}

var _ EventHandler = Funcs{}

// Funcs implements EventHandler.
type Funcs struct {
	// Create is called in response to an add event.  Defaults to no-op.
	// RateLimitingInterface is used to enqueue reconcile.Requests.
	CreateFunc func(context.Context, event.CreateEvent, workqueue.RateLimitingInterface)

	// Update is called in response to an update event.  Defaults to no-op.
	// RateLimitingInterface is used to enqueue reconcile.Requests.
	UpdateFunc func(context.Context, event.UpdateEvent, workqueue.RateLimitingInterface)

	// Delete is called in response to a delete event.  Defaults to no-op.
	// RateLimitingInterface is used to enqueue reconcile.Requests.
	DeleteFunc func(context.Context, event.DeleteEvent, workqueue.RateLimitingInterface)

	// GenericFunc is called in response to a generic event.  Defaults to no-op.
	// RateLimitingInterface is used to enqueue reconcile.Requests.
	GenericFunc func(context.Context, event.GenericEvent, workqueue.RateLimitingInterface)
}

// Create implements EventHandler.
func (h Funcs) Create(ctx context.Context, e event.CreateEvent, q workqueue.RateLimitingInterface) {
	if h.CreateFunc != nil {
		h.CreateFunc(ctx, e, q)
	}
}

// Delete implements EventHandler.
func (h Funcs) Delete(ctx context.Context, e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	if h.DeleteFunc != nil {
		h.DeleteFunc(ctx, e, q)
	}
}

// Update implements EventHandler.
func (h Funcs) Update(ctx context.Context, e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	if h.UpdateFunc != nil {
		h.UpdateFunc(ctx, e, q)
	}
}

// Generic implements EventHandler.
func (h Funcs) Generic(ctx context.Context, e event.GenericEvent, q workqueue.RateLimitingInterface) {
	if h.GenericFunc != nil {
		h.GenericFunc(ctx, e, q)
	}
}

var _ EventHandler = Funcs{}
var _ EventHandler = ObjectFuncs[any]{}
var _ ObjectHandler[any] = ObjectFuncs[any]{}

// ObjectFuncs is a function that implements ObjectPredicate.
type ObjectFuncs[T any] struct {
	// Create is called in response to an add event.  Defaults to no-op.
	// RateLimitingInterface is used to enqueue reconcile.Requests.
	CreateFunc func(ctx context.Context, obj T, queue workqueue.RateLimitingInterface)

	// Update is called in response to an update event.  Defaults to no-op.
	// RateLimitingInterface is used to enqueue reconcile.Requests.
	UpdateFunc func(ctx context.Context, old, new T, queue workqueue.RateLimitingInterface)

	// Delete is called in response to a delete event.  Defaults to no-op.
	// RateLimitingInterface is used to enqueue reconcile.Requests.
	DeleteFunc func(ctx context.Context, obj T, queue workqueue.RateLimitingInterface)

	// GenericFunc is called in response to a generic event.  Defaults to no-op.
	// RateLimitingInterface is used to enqueue reconcile.Requests.
	GenericFunc func(ctx context.Context, obj T, queue workqueue.RateLimitingInterface)
}

// Update implements Predicate.
func (p ObjectFuncs[T]) Update(ctx context.Context, e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	objNew, newOk := e.ObjectNew.(T)
	objOld, oldOk := e.ObjectOld.(T)
	if newOk && oldOk {
		p.OnUpdate(ctx, objOld, objNew, q)
	}
}

// Generic implements Predicate.
func (p ObjectFuncs[T]) Generic(ctx context.Context, e event.GenericEvent, q workqueue.RateLimitingInterface) {
	obj, ok := e.Object.(T)
	if ok {
		p.OnGeneric(ctx, obj, q)
	}
}

// Create implements Predicate.
func (p ObjectFuncs[T]) Create(ctx context.Context, e event.CreateEvent, q workqueue.RateLimitingInterface) {
	obj, ok := e.Object.(T)
	if ok {
		p.OnCreate(ctx, obj, q)
	}
}

// Delete implements Predicate.
func (p ObjectFuncs[T]) Delete(ctx context.Context, e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	obj, ok := e.Object.(T)
	if ok {
		p.OnDelete(ctx, obj, q)
	}
}

// OnUpdate implements ObjectPredicate.
func (p ObjectFuncs[T]) OnUpdate(ctx context.Context, old, new T, q workqueue.RateLimitingInterface) {
	if p.UpdateFunc != nil {
		p.UpdateFunc(ctx, old, new, q)
	}
}

// OnGeneric implements ObjectPredicate.
func (p ObjectFuncs[T]) OnGeneric(ctx context.Context, obj T, q workqueue.RateLimitingInterface) {
	if p.GenericFunc != nil {
		p.GenericFunc(ctx, obj, q)
	}
}

// OnCreate implements ObjectPredicate.
func (p ObjectFuncs[T]) OnCreate(ctx context.Context, obj T, q workqueue.RateLimitingInterface) {
	if p.CreateFunc != nil {
		p.CreateFunc(ctx, obj, q)
	}
}

// OnDelete implements ObjectPredicate.
func (p ObjectFuncs[T]) OnDelete(ctx context.Context, obj T, q workqueue.RateLimitingInterface) {
	if p.DeleteFunc != nil {
		p.DeleteFunc(ctx, obj, q)
	}
}

// ObjectFuncAdapter allows to reuse existing EventHandler for a typed ObjectHandler
func ObjectFuncAdapter[T client.Object](h EventHandler) ObjectHandler[T] {
	return ObjectFuncs[T]{
		CreateFunc: func(ctx context.Context, obj T, queue workqueue.RateLimitingInterface) {
			h.Create(ctx, event.CreateEvent{Object: obj}, queue)
		},
		DeleteFunc: func(ctx context.Context, obj T, queue workqueue.RateLimitingInterface) {
			h.Delete(ctx, event.DeleteEvent{Object: obj}, queue)
		},
		GenericFunc: func(ctx context.Context, obj T, queue workqueue.RateLimitingInterface) {
			h.Generic(ctx, event.GenericEvent{Object: obj}, queue)
		},
		UpdateFunc: func(ctx context.Context, old, new T, queue workqueue.RateLimitingInterface) {
			h.Update(ctx, event.UpdateEvent{ObjectOld: old, ObjectNew: new}, queue)
		},
	}
}

// EventHandlerAdapter allows to reuse existing typed event handler as EventHandler
func EventHandlerAdapter[T client.Object](h ObjectHandler[T]) EventHandler {
	return Funcs{
		CreateFunc: func(ctx context.Context, e event.CreateEvent, queue workqueue.RateLimitingInterface) {
			obj, ok := e.Object.(T)
			if ok {
				h.OnCreate(ctx, obj, queue)
			}
		},
		DeleteFunc: func(ctx context.Context, e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
			obj, ok := e.Object.(T)
			if ok {
				h.OnDelete(ctx, obj, queue)
			}
		},
		GenericFunc: func(ctx context.Context, e event.GenericEvent, queue workqueue.RateLimitingInterface) {
			obj, ok := e.Object.(T)
			if ok {
				h.OnGeneric(ctx, obj, queue)
			}
		},
		UpdateFunc: func(ctx context.Context, e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
			objNew, newOk := e.ObjectNew.(T)
			objOld, oldOk := e.ObjectOld.(T)
			if newOk && oldOk {
				h.OnUpdate(ctx, objOld, objNew, queue)
			}
		},
	}
}
