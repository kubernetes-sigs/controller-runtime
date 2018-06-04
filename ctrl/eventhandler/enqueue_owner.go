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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
)

var _ EventHandler = EnqueueOwnerHandler{}

// EnqueueOwnerHandler enqueues a ReconcileRequest containing the Name and Namespace of the Owner of the object in
// the Event.  EnqueueOwnerHandler is used with Reconcile implementations that create objects to trigger a Reconcile
// for Events on the created objects (by Reconciling the parent).
type EnqueueOwnerHandler struct {
	// OwnerType is the GroupVersionKind of the Owner type
	OwnerType runtime.Object

	// IsController determines whether or not to enqueue non-controller Owners.
	IsController bool

	Scheme *runtime.Scheme
}

func (e *EnqueueOwnerHandler) InitScheme(s *runtime.Scheme) {
	if e.Scheme == nil {
		e.Scheme = s
	}
}

// Create implements EventHandler
func (e EnqueueOwnerHandler) Create(q workqueue.RateLimitingInterface, evt event.CreateEvent) {
	if req, found := e.getOwnerReconcileRequest(evt.Meta); found {
		q.AddRateLimited(req)
	}
}

// Update implements EventHandler
func (e EnqueueOwnerHandler) Update(q workqueue.RateLimitingInterface, evt event.UpdateEvent) {
	if req, found := e.getOwnerReconcileRequest(evt.MetaOld); found {
		q.AddRateLimited(req)
	}
	if req, found := e.getOwnerReconcileRequest(evt.MetaNew); found {
		q.AddRateLimited(req)
	}
}

// Delete implements EventHandler
func (e EnqueueOwnerHandler) Delete(q workqueue.RateLimitingInterface, evt event.DeleteEvent) {
	if req, found := e.getOwnerReconcileRequest(evt.Meta); found {
		q.AddRateLimited(req)
	}
}

// Generic implements EventHandler
func (e EnqueueOwnerHandler) Generic(q workqueue.RateLimitingInterface, evt event.GenericEvent) {
	if req, found := e.getOwnerReconcileRequest(evt.Meta); found {
		q.AddRateLimited(req)
	}
}

// lookupObjectFromCache looks up an object from the cache by its GroupVersionKind and name, and returns it
func (e EnqueueOwnerHandler) lookupObjectFromCache(kind runtime.Object, namespace, name string) metav1.Object {
	return nil
}

func (e EnqueueOwnerHandler) getOwnerReconcileRequest(object metav1.Object) (reconcile.ReconcileRequest, bool) {
	// Iterate through OwnerReferences to find one whose resource matches the OwnerType resource
	// The only way to figure out if 2 different GroupVersionKinds
	//kinds, _, err := e.Scheme.ObjectKinds(e.OwnerType)
	//if err != nil {
	//
	//}
	for _, ref := range e.getOwnersReferences(object) {

		// Check if this OwnerReference has the correct type
		// Compare the owner UID of the reference against the UID of the OwnerType object with the same name
		l := e.lookupObjectFromCache(e.OwnerType, object.GetNamespace(), ref.Name)
		if l.GetUID() == ref.UID {
			return reconcile.ReconcileRequest{types.NamespacedName{
				Name:      ref.Name,
				Namespace: object.GetNamespace(),
			}}, true
		}
	}

	return reconcile.ReconcileRequest{}, false
}

func (e EnqueueOwnerHandler) getOwnersReferences(object metav1.Object) []metav1.OwnerReference {
	if object == nil {
		return nil
	}

	if !e.IsController {
		return object.GetOwnerReferences()
	}

	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		return []metav1.OwnerReference{*ownerRef}
	}

	return nil
}
