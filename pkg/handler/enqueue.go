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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var enqueueLog = logf.KBLog.WithName("eventhandler").WithName("Enqueue")

var _ EventHandler = &Enqueue{}

// Enqueue enqueues a Request containing the Name and Namespace of the object for each event.
type Enqueue struct{}

// Create implements EventHandler
func (e *Enqueue) Create(q workqueue.RateLimitingInterface, evt event.CreateEvent) {
	if evt.Meta == nil {
		enqueueLog.Error(nil, "CreateEvent received with no metadata", "CreateEvent", evt)
		return
	}
	q.AddRateLimited(reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      evt.Meta.GetName(),
		Namespace: evt.Meta.GetNamespace(),
	}})
}

// Update implements EventHandler
func (e *Enqueue) Update(q workqueue.RateLimitingInterface, evt event.UpdateEvent) {
	if evt.MetaOld != nil {
		q.AddRateLimited(reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      evt.MetaOld.GetName(),
			Namespace: evt.MetaOld.GetNamespace(),
		}})
	} else {
		enqueueLog.Error(nil, "UpdateEvent received with no old metadata", "UpdateEvent", evt)
	}

	if evt.MetaNew != nil {
		q.AddRateLimited(reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      evt.MetaNew.GetName(),
			Namespace: evt.MetaNew.GetNamespace(),
		}})
	} else {
		enqueueLog.Error(nil, "UpdateEvent received with no new metadata", "UpdateEvent", evt)
	}
}

// Delete implements EventHandler
func (e *Enqueue) Delete(q workqueue.RateLimitingInterface, evt event.DeleteEvent) {
	if evt.Meta == nil {
		enqueueLog.Error(nil, "DeleteEvent received with no metadata", "DeleteEvent", evt)
		return
	}
	q.AddRateLimited(reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      evt.Meta.GetName(),
		Namespace: evt.Meta.GetNamespace(),
	}})
}

// Generic implements EventHandler
func (e *Enqueue) Generic(q workqueue.RateLimitingInterface, evt event.GenericEvent) {
	if evt.Meta == nil {
		enqueueLog.Error(nil, "GenericEvent received with no metadata", "GenericEvent", evt)
		return
	}
	q.AddRateLimited(reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      evt.Meta.GetName(),
		Namespace: evt.Meta.GetNamespace(),
	}})
}
