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

package eventhandler

import (
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/event"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/reconcile"
	logf "github.com/kubernetes-sigs/kubebuilder/pkg/log"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
)

var enqueueLog = logf.KBLog.WithName("eventhandler").WithName("EnqueueHandler")

var _ EventHandler = &EnqueueHandler{}

// EnqueueHandler enqueues a ReconcileRequest containing the Name and Namespace of the object in the Event.
type EnqueueHandler struct{}

// Create implements EventHandler
func (e *EnqueueHandler) Create(q workqueue.RateLimitingInterface, evt event.CreateEvent) {
	enqueueLog.Info("Enqueue", "CreateEvent", evt)

	if evt.Meta == nil {
		// TODO: Log an error and increment a metric
		return
	}
	q.AddRateLimited(reconcile.ReconcileRequest{types.NamespacedName{
		Name:      evt.Meta.GetName(),
		Namespace: evt.Meta.GetNamespace(),
	}})
}

// Update implements EventHandler
func (e *EnqueueHandler) Update(q workqueue.RateLimitingInterface, evt event.UpdateEvent) {
	if evt.MetaOld != nil {
		q.AddRateLimited(reconcile.ReconcileRequest{types.NamespacedName{
			Name:      evt.MetaOld.GetName(),
			Namespace: evt.MetaOld.GetNamespace(),
		}})
	} else {
		// TODO: Log an error and increment a metric
	}

	if evt.MetaNew != nil {
		q.AddRateLimited(reconcile.ReconcileRequest{types.NamespacedName{
			Name:      evt.MetaNew.GetName(),
			Namespace: evt.MetaNew.GetNamespace(),
		}})
	} else {
		// TODO: Log an error and increment a metric
	}
}

// Delete implements EventHandler
func (e *EnqueueHandler) Delete(q workqueue.RateLimitingInterface, evt event.DeleteEvent) {
	if evt.Meta == nil {
		// TODO: Log an error and increment a metric
		return
	}
	q.AddRateLimited(reconcile.ReconcileRequest{types.NamespacedName{
		Name:      evt.Meta.GetName(),
		Namespace: evt.Meta.GetNamespace(),
	}})
}

// Generic implements EventHandler
func (e *EnqueueHandler) Generic(q workqueue.RateLimitingInterface, evt event.GenericEvent) {
	if evt.Meta == nil {
		// TODO: Log an error and increment a metric
		return
	}
	q.AddRateLimited(reconcile.ReconcileRequest{types.NamespacedName{
		Name:      evt.Meta.GetName(),
		Namespace: evt.Meta.GetNamespace(),
	}})
}
