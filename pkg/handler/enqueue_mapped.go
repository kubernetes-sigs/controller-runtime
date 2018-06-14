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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ EventHandler = &EnqueueMapped{}

// EnqueueMapped enqueues Requests by running a transformation function on each Event.
//
// For UpdateEvents which contain both a new and old object, the transformation function is run on both
// objects and both sets of Requests are enqueue.
type EnqueueMapped struct {
	// Mapper transforms the argument into a slice of keys to be reconciled
	ToRequests Mapper
}

// Create implements EventHandler
func (e *EnqueueMapped) Create(q workqueue.RateLimitingInterface, evt event.CreateEvent) {
	e.mapAndEnqueue(q, MapObject{Meta: evt.Meta, Object: evt.Object})
}

// Update implements EventHandler
func (e *EnqueueMapped) Update(q workqueue.RateLimitingInterface, evt event.UpdateEvent) {
	e.mapAndEnqueue(q, MapObject{Meta: evt.MetaOld, Object: evt.ObjectOld})
	e.mapAndEnqueue(q, MapObject{Meta: evt.MetaNew, Object: evt.ObjectNew})
}

// Delete implements EventHandler
func (e *EnqueueMapped) Delete(q workqueue.RateLimitingInterface, evt event.DeleteEvent) {
	e.mapAndEnqueue(q, MapObject{Meta: evt.Meta, Object: evt.Object})
}

// Generic implements EventHandler
func (e *EnqueueMapped) Generic(q workqueue.RateLimitingInterface, evt event.GenericEvent) {
	e.mapAndEnqueue(q, MapObject{Meta: evt.Meta, Object: evt.Object})
}

func (e *EnqueueMapped) mapAndEnqueue(q workqueue.RateLimitingInterface, object MapObject) {
	for _, req := range e.ToRequests.Map(object) {
		q.AddRateLimited(req)
	}
}

// Mapper maps an object to a collection of keys to be enqueued
type Mapper interface {
	// Map maps an object
	Map(MapObject) []reconcile.Request
}

// MapObject contains information from an event to be transformed into a Request.
type MapObject struct {
	// Meta is the meta data for an object from an event.
	Meta metav1.Object

	// Object is the object from an event.
	Object runtime.Object
}

var _ Mapper = ToRequestsFunc(nil)

// ToRequestsFunc implements Mapper using a function.
type ToRequestsFunc func(MapObject) []reconcile.Request

// Map implements Mapper
func (m ToRequestsFunc) Map(i MapObject) []reconcile.Request {
	return m(i)
}
