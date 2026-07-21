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

package internal

import (
	"context"

	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ toolscache.TypedResourceEventHandler[client.Object] = &EventHandler[client.Object, any]{}

// NewEventHandler creates a new EventHandler.
func NewEventHandler[object toolscache.Object, request comparable](
	ctx context.Context,
	queue workqueue.TypedRateLimitingInterface[request],
	handler handler.TypedEventHandler[object, request],
	predicates []predicate.TypedPredicate[object]) *EventHandler[object, request] {
	return &EventHandler[object, request]{
		ctx:        ctx,
		handler:    handler,
		queue:      queue,
		predicates: predicates,
	}
}

// EventHandler adapts a handler.EventHandler interface to a cache.ResourceEventHandler interface.
type EventHandler[object toolscache.Object, request comparable] struct {
	// ctx stores the context that created the event handler
	// that is used to propagate cancellation signals to each handler function.
	ctx context.Context

	handler    handler.TypedEventHandler[object, request]
	queue      workqueue.TypedRateLimitingInterface[request]
	predicates []predicate.TypedPredicate[object]
}

// OnAdd creates CreateEvent and calls Create on EventHandler.
func (e *EventHandler[object, request]) OnAdd(obj object, isInInitialList bool) {
	c := event.TypedCreateEvent[object]{
		Object:          obj,
		IsInInitialList: isInInitialList,
	}

	for _, p := range e.predicates {
		if !p.Create(c) {
			return
		}
	}

	// Invoke create handler
	ctx, cancel := context.WithCancel(e.ctx)
	defer cancel()
	e.handler.Create(ctx, c, e.queue)
}

// OnUpdate creates UpdateEvent and calls Update on EventHandler.
func (e *EventHandler[object, request]) OnUpdate(oldObj, newObj object) {
	u := event.TypedUpdateEvent[object]{ObjectOld: oldObj, ObjectNew: newObj}

	for _, p := range e.predicates {
		if !p.Update(u) {
			return
		}
	}

	// Invoke update handler
	ctx, cancel := context.WithCancel(e.ctx)
	defer cancel()
	e.handler.Update(ctx, u, e.queue)
}

// OnDelete creates DeleteEvent and calls Delete on EventHandler.
func (e *EventHandler[object, request]) OnDelete(obj toolscache.DeletedObject[object]) {
	d := event.TypedDeleteEvent[object]{
		Object:             obj.OptionalObj,
		DeleteStateUnknown: obj.FinalStateUnknown != nil,
	}

	for _, p := range e.predicates {
		if !p.Delete(d) {
			return
		}
	}

	// Invoke delete handler
	ctx, cancel := context.WithCancel(e.ctx)
	defer cancel()
	e.handler.Delete(ctx, d, e.queue)
}
