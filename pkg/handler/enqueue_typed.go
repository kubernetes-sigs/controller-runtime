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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ EventHandler = &EnqueueRequest[metav1.Object]{}
var _ ObjectHandler[metav1.Object] = &EnqueueRequest[metav1.Object]{}

type Request interface {
	comparable
	GetName() string
	GetNamespace() string
}

// EnqueueRequest enqueues a Request containing the Name and Namespace of the object that is the source of the Event.
// (e.g. the created / deleted / updated objects Name and Namespace).  handler.EnqueueRequest is used by almost all
// Controllers that have associated Resources (e.g. CRDs) to reconcile the associated Resource.
type EnqueueRequest[T Request] struct{}

// OnCreate implements ObjectHandler.
func (e *EnqueueRequest[T]) OnCreate(ctx context.Context, obj T, q workqueue.RateLimitingInterface) {
	if obj != *new(T) {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		}})
	}
}

// OnDelete implements ObjectHandler.
func (e *EnqueueRequest[T]) OnDelete(ctx context.Context, obj T, q workqueue.RateLimitingInterface) {
	if obj != *new(T) {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		}})
	}
}

// OnGeneric implements ObjectHandler.
func (e *EnqueueRequest[T]) OnGeneric(ctx context.Context, obj T, q workqueue.RateLimitingInterface) {
	if obj != *new(T) {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		}})
	}
}

// OnUpdate implements ObjectHandler.
func (e *EnqueueRequest[T]) OnUpdate(ctx context.Context, oldObj T, newObj T, q workqueue.RateLimitingInterface) {
	if oldObj != *new(T) {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      oldObj.GetName(),
			Namespace: oldObj.GetNamespace(),
		}})
	}
}

// Create implements EventHandler.
func (e *EnqueueRequest[T]) Create(ctx context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	obj, ok := evt.Object.(T)
	if ok {
		e.OnCreate(ctx, obj, q)
	}
}

// Update implements EventHandler.
func (e *EnqueueRequest[T]) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	objOld, okOld := evt.ObjectOld.(T)
	objNew, okNew := evt.ObjectNew.(T)

	if okOld && okNew {
		e.OnUpdate(ctx, objOld, objNew, q)
	}
}

// Delete implements EventHandler.
func (e *EnqueueRequest[T]) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	obj, ok := evt.Object.(T)
	if ok {
		e.OnDelete(ctx, obj, q)
	}
}

// Generic implements EventHandler.
func (e *EnqueueRequest[T]) Generic(ctx context.Context, evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	obj, ok := evt.Object.(T)
	if ok {
		e.OnGeneric(ctx, obj, q)
	}
}
