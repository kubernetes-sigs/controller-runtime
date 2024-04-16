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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ObjectMapFunc is the signature required for enqueueing requests from a generic function.
// This type is usually used with EnqueueRequestsFromTypeMapFunc when registering an event handler.
// Unlike MapFunc, a specific object type can be used to process and create mapping requests.
type ObjectMapFunc[T any] func(context.Context, T) []reconcile.Request

func MapFuncAdapter(m MapFunc) ObjectMapFunc[any] {
	return func(ctx context.Context, a any) (reqs []reconcile.Request) {
		obj, ok := a.(client.Object)
		if ok {
			return m(ctx, obj)
		}

		return []reconcile.Request{}
	}
}

// EnqueueRequestsFromObjectMapFunc enqueues Requests by running a transformation function that outputs a collection
// of reconcile.Requests on each Event.  The reconcile.Requests may be for an arbitrary set of objects
// defined by some user specified transformation of the source Event.  (e.g. trigger Reconciler for a set of objects
// in response to a cluster resize event caused by adding or deleting a Node)
//
// EnqueueRequestsFromObjectMapFunc is frequently used to fan-out updates from one object to one or more other
// objects of a differing type.
//
// For UpdateEvents which contain both a new and old object, the transformation function is run on both
// objects and both sets of Requests are enqueue.
func EnqueueRequestsFromObjectMapFunc[T any](fn ObjectMapFunc[T]) EventHandler {
	return &enqueueRequestsFromObjectMapFunc[T]{
		toRequests: fn,
	}
}

// EnqueueRequestsFromObjectMap enqueues Requests by running a transformation function that outputs a collection
// of reconcile.Requests on each Event.  The reconcile.Requests may be for an arbitrary set of objects
// defined by some user specified transformation of the source Event.  (e.g. trigger Reconciler for a set of objects
// in response to a cluster resize event caused by adding or deleting a Node)
//
// EnqueueRequestsFromObjectMap is frequently used to fan-out updates from one object to one or more other
// objects of a differing type.
//
// For UpdateEvents which contain both a new and old object, the transformation function is run on both
// objects and both sets of Requests are enqueue.
func EnqueueRequestsFromObjectMap[T any](fn ObjectMapFunc[T]) ObjectHandler[T] {
	return &enqueueRequestsFromObjectMapFunc[T]{
		toRequests: fn,
	}
}

var _ EventHandler = &enqueueRequestsFromObjectMapFunc[any]{}
var _ ObjectHandler[any] = &enqueueRequestsFromObjectMapFunc[any]{}

type enqueueRequestsFromObjectMapFunc[T any] struct {
	// Mapper transforms the argument into a slice of keys to be reconciled
	toRequests ObjectMapFunc[T]
}

// OnCreate implements ObjectHandler.
func (e *enqueueRequestsFromObjectMapFunc[T]) OnCreate(ctx context.Context, obj T, q workqueue.RateLimitingInterface) {
	reqs := map[reconcile.Request]empty{}
	e.mapAndEnqueue(ctx, q, obj, reqs)
}

// OnDelete implements ObjectHandler.
func (e *enqueueRequestsFromObjectMapFunc[T]) OnDelete(ctx context.Context, obj T, q workqueue.RateLimitingInterface) {
	reqs := map[reconcile.Request]empty{}
	e.mapAndEnqueue(ctx, q, obj, reqs)
}

// OnGeneric implements ObjectHandler.
func (e *enqueueRequestsFromObjectMapFunc[T]) OnGeneric(ctx context.Context, obj T, q workqueue.RateLimitingInterface) {
	reqs := map[reconcile.Request]empty{}
	e.mapAndEnqueue(ctx, q, obj, reqs)
}

// OnUpdate implements ObjectHandler.
func (e *enqueueRequestsFromObjectMapFunc[T]) OnUpdate(ctx context.Context, old T, new T, q workqueue.RateLimitingInterface) {
	reqs := map[reconcile.Request]empty{}
	e.mapAndEnqueue(ctx, q, old, reqs)
	e.mapAndEnqueue(ctx, q, new, reqs)
}

// Create implements EventHandler.
func (e *enqueueRequestsFromObjectMapFunc[T]) Create(ctx context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	obj, ok := evt.Object.(T)
	if ok {
		e.OnCreate(ctx, obj, q)
	}
}

// Update implements EventHandler.
func (e *enqueueRequestsFromObjectMapFunc[T]) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	old, okOld := evt.ObjectOld.(T)
	new, okNew := evt.ObjectNew.(T)
	if okOld && okNew {
		e.OnUpdate(ctx, old, new, q)
	}
}

// Delete implements EventHandler.
func (e *enqueueRequestsFromObjectMapFunc[T]) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	obj, ok := evt.Object.(T)
	if ok {
		e.OnDelete(ctx, obj, q)
	}
}

// Generic implements EventHandler.
func (e *enqueueRequestsFromObjectMapFunc[T]) Generic(ctx context.Context, evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	obj, ok := evt.Object.(T)
	if ok {
		e.OnGeneric(ctx, obj, q)
	}
}

func (e *enqueueRequestsFromObjectMapFunc[T]) mapAndEnqueue(ctx context.Context, q workqueue.RateLimitingInterface, object T, reqs map[reconcile.Request]empty) {
	for _, req := range e.toRequests(ctx, object) {
		_, ok := reqs[req]
		if !ok {
			q.Add(req)
			reqs[req] = empty{}
		}
	}
}
