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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
)

var _ EventHandler = EnqueueMappedHandler{}

// EnqueueMappedHandler enqueues ReconcileRequests resulting from running a user provided transformation
// function on the Event.
type EnqueueMappedHandler struct {
	// ToRequests transforms the argument into a slice of keys to be reconciled
	ToRequests ToRequests
}

// Create implements EventHandler
func (e EnqueueMappedHandler) Create(q workqueue.RateLimitingInterface, evt event.CreateEvent) {
	reqs := e.ToRequests.Map(ToRequestArg{
		Meta:   evt.Meta,
		Object: evt.Object,
	})
	for _, req := range reqs {
		q.AddRateLimited(req)
	}
}

// Update implements EventHandler
func (e EnqueueMappedHandler) Update(q workqueue.RateLimitingInterface, evt event.UpdateEvent) {
	reqs := e.ToRequests.Map(ToRequestArg{
		Meta:   evt.MetaOld,
		Object: evt.ObjectOld,
	})
	for _, req := range reqs {
		q.AddRateLimited(req)
	}

	reqs = e.ToRequests.Map(ToRequestArg{
		Meta:   evt.MetaNew,
		Object: evt.ObjectNew,
	})
	for _, req := range reqs {
		q.AddRateLimited(req)
	}
}

// Delete implements EventHandler
func (e EnqueueMappedHandler) Delete(q workqueue.RateLimitingInterface, evt event.DeleteEvent) {
	reqs := e.ToRequests.Map(ToRequestArg{
		Meta:   evt.Meta,
		Object: evt.Object,
	})
	for _, req := range reqs {
		q.AddRateLimited(req)
	}
}

// Generic implements EventHandler
func (e EnqueueMappedHandler) Generic(q workqueue.RateLimitingInterface, evt event.GenericEvent) {
	reqs := e.ToRequests.Map(ToRequestArg{
		Meta:   evt.Meta,
		Object: evt.Object,
	})
	for _, req := range reqs {
		q.AddRateLimited(req)
	}

}

// ToRequests maps an object to a collection of keys to be enqueued
type ToRequests interface {
	Map(ToRequestArg) []reconcile.ReconcileRequest
}

type ToRequestArg struct {
	Meta   metav1.Object
	Object runtime.Object
}

var _ ToRequests = ToRequestsFunc(nil)

// ToRequestsFunc implements ToRequests using a function.
type ToRequestsFunc func(ToRequestArg) []reconcile.ReconcileRequest

func (m ToRequestsFunc) Map(i ToRequestArg) []reconcile.ReconcileRequest {
	return m(i)
}
