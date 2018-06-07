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
	"os"
	"sync"

	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/event"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/inject"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/reconcile"
	logf "github.com/kubernetes-sigs/kubebuilder/pkg/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
)

var _ EventHandler = &EnqueueOwnerHandler{}

var log = logf.KBLog.WithName("eventhandler").WithName("EnqueueOwnerHandler")

// EnqueueOwnerHandler enqueues ReconcileRequests for the Owners of an object.  E.g. an object that created
// another object.
//
// If a ReplicaSet creates Pods, reconcile the ReplicaSet in response to events on Pods that it created using:
//
// - a KindSource with Type Pod.
//
// - a EnqueueOwnerHandler with OwnerType ReplicaSet.
type EnqueueOwnerHandler struct {
	// OwnerType is the type of the Owner object to look for in OwnerReferences.  Only Group and Kind are compared.
	OwnerType runtime.Object

	// IsController if set will only look at the first OwnerReference with Controller: true.
	IsController bool

	// scheme is injected by a Controller
	// scheme is used to resolve the Group and Kind for the runtime.Object.
	scheme *runtime.Scheme

	// once is used to cache the resolution of OwnerType to a Group and Kind
	once sync.Once

	// groupKind is the cached Group and Kind from OwnerType
	groupKind schema.GroupKind

	// kindOk is true if OwnerType was successfuly parsed
	kindOk bool
}

var _ inject.Scheme = &EnqueueOwnerHandler{}

// InjectScheme is called by the Controller to provide a singleton scheme to the EnqueueOwnerHandler.
func (e *EnqueueOwnerHandler) InjectScheme(s *runtime.Scheme) {
	if e.scheme == nil {
		e.scheme = s
	}
}

// Create implements EventHandler
func (e *EnqueueOwnerHandler) Create(q workqueue.RateLimitingInterface, evt event.CreateEvent) {
	for _, req := range e.getOwnerReconcileRequest(evt.Meta) {
		q.AddRateLimited(req)
	}
}

// Update implements EventHandler
func (e *EnqueueOwnerHandler) Update(q workqueue.RateLimitingInterface, evt event.UpdateEvent) {
	for _, req := range e.getOwnerReconcileRequest(evt.MetaOld) {
		q.AddRateLimited(req)
	}
	for _, req := range e.getOwnerReconcileRequest(evt.MetaNew) {
		q.AddRateLimited(req)
	}
}

// Delete implements EventHandler
func (e *EnqueueOwnerHandler) Delete(q workqueue.RateLimitingInterface, evt event.DeleteEvent) {
	for _, req := range e.getOwnerReconcileRequest(evt.Meta) {
		q.AddRateLimited(req)
	}
}

// Generic implements EventHandler
func (e *EnqueueOwnerHandler) Generic(q workqueue.RateLimitingInterface, evt event.GenericEvent) {
	for _, req := range e.getOwnerReconcileRequest(evt.Meta) {
		q.AddRateLimited(req)
	}
}

// parseOwnerTypeGroupKind parses the OwnerType into a Group and Kind and caches the result.  Returns false
// if the OwnerType could not be parsed using the scheme.
func (e *EnqueueOwnerHandler) parseOwnerTypeGroupKind() bool {
	// Parse the OwnerType group and kind once the first time an event is handled and cache the result.
	e.once.Do(func() {
		// If the scheme isn't, bail.
		if e.scheme == nil {
			log.Error(nil, "Must use inject a scheme into EnqueueOwnerHandler before using it."+
				"This is done automatically if using EnqueueOwnerHandler from a Controller Watch function.")
			os.Exit(1)
		}

		// Get the kinds of the type
		kinds, _, err := e.scheme.ObjectKinds(e.OwnerType)
		if err != nil {
			log.Error(err, "Could not get ObjectKinds for OwnerType", "OwnerType", e.OwnerType)
			return
		}
		// Expect only 1 kind.  If there is more than one kind this is probably an edge case such as ListOptions.
		if len(kinds) != 1 {
			log.Error(nil, "Expected exactly 1 kind for OwnerType",
				"OwnerType", e.OwnerType, "Kinds", kinds)
			return
		}
		// Cache the Group and Kind for the OwnerType
		e.groupKind = schema.GroupKind{Group: kinds[0].Group, Kind: kinds[0].Kind}
		e.kindOk = true
	})
	return e.kindOk
}

// getOwnerReconcileRequest looks at object and returns a slice of reconcile.ReconcileRequest to reconcile
// owners of object that match e.OwnerType.
func (e *EnqueueOwnerHandler) getOwnerReconcileRequest(object metav1.Object) []reconcile.ReconcileRequest {
	// If there was an error getting the Group and Kind for the OwnerType do nothing and log an error
	if !e.parseOwnerTypeGroupKind() {
		log.Error(nil, "Tried to get owner of unknown kind", "OwnerType", e.OwnerType)
		return nil
	}

	// Iterate through the OwnerReferences looking for a match on Group and Kind against what was requested
	// by the user
	var result []reconcile.ReconcileRequest
	for _, ref := range e.getOwnersReferences(object) {
		// Parse the Group out of the OwnerReference to compare it to what was parsed out of the requested OwnerType
		refGV, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			log.Error(err, "Could not parse OwnerReference GroupVersion",
				"OwnerReference", ref.APIVersion)
			return nil
		}

		// Compare the OwnerReference Group and Kind against the OwnerType Group and Kind specified by the user.
		// If the two match, create a ReconcileRequest for the objected referred to by
		// the OwnerReference.  Use the Name from the OwnerReference and the Namespace from the
		// object in the event.
		if ref.Kind == e.groupKind.Kind && refGV.Group == e.groupKind.Group {
			// Match found - add a ReconcileRequest for the object referred to in the OwnerReference
			result = append(result, reconcile.ReconcileRequest{NamespacedName: types.NamespacedName{
				Namespace: object.GetNamespace(),
				Name:      ref.Name,
			}})
		}
	}

	// Return the matches
	return result
}

// getOwnersReferences returns the OwnerReferences for an object as specified by the EnqueueOwnerHandler
// - if IsController is true: only take the Controller OwnerReference (if found)
// - if IsController is false: take all OwnerReferences
func (e *EnqueueOwnerHandler) getOwnersReferences(object metav1.Object) []metav1.OwnerReference {
	if object == nil {
		return nil
	}

	// If filtered to Controller use all the OwnerReferences
	if !e.IsController {
		return object.GetOwnerReferences()
	}
	// If filtered to a Controller, only take the Controller OwnerReference
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		return []metav1.OwnerReference{*ownerRef}
	}
	// No Controller OwnerReference found
	return nil
}
