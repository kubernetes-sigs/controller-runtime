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
	"sync"

	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/event"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/reconcile"
	logf "github.com/kubernetes-sigs/kubebuilder/pkg/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
)

var _ EventHandler = EnqueueOwnerHandler{}

var log = logf.KBLog.WithName("eventhandler").WithName("EnqueueOwnerHandler")

// EnqueueOwnerHandler enqueues a ReconcileRequest containing the Name and Namespace of the Owner of the object in
// the Event.  EnqueueOwnerHandler is used with Reconcile implementations that create objects to trigger a Reconcile
// for Events on the created objects (by Reconciling the parent).
type EnqueueOwnerHandler struct {
	// OwnerType is the GroupVersionKind of the Owner type
	OwnerType runtime.Object

	// IsController determines whether or not to enqueue non-controller Owners.
	IsController bool

	Scheme *runtime.Scheme

	once      sync.Once
	groupKind schema.GroupKind
	kindOk    bool
}

func (e *EnqueueOwnerHandler) InitScheme(s *runtime.Scheme) {
	if e.Scheme == nil {
		e.Scheme = s
	}
}

// Create implements EventHandler
func (e EnqueueOwnerHandler) Create(q workqueue.RateLimitingInterface, evt event.CreateEvent) {
	for _, req := range e.getOwnerReconcileRequest(evt.Meta) {
		q.AddRateLimited(req)
	}
}

// Update implements EventHandler
func (e EnqueueOwnerHandler) Update(q workqueue.RateLimitingInterface, evt event.UpdateEvent) {
	for _, req := range e.getOwnerReconcileRequest(evt.MetaOld) {
		q.AddRateLimited(req)
	}
	for _, req := range e.getOwnerReconcileRequest(evt.MetaNew) {
		q.AddRateLimited(req)
	}
}

// Delete implements EventHandler
func (e EnqueueOwnerHandler) Delete(q workqueue.RateLimitingInterface, evt event.DeleteEvent) {
	for _, req := range e.getOwnerReconcileRequest(evt.Meta) {
		q.AddRateLimited(req)
	}
}

// Generic implements EventHandler
func (e EnqueueOwnerHandler) Generic(q workqueue.RateLimitingInterface, evt event.GenericEvent) {
	for _, req := range e.getOwnerReconcileRequest(evt.Meta) {
		q.AddRateLimited(req)
	}
}

func (e EnqueueOwnerHandler) getOwnerReconcileRequest(object metav1.Object) []reconcile.ReconcileRequest {
	e.once.Do(func() {
		kinds, _, err := e.Scheme.ObjectKinds(e.OwnerType)
		if err != nil {
			log.Error(err, "Could not get ObjectKinds for OwnerType", "OwnerType", e.OwnerType)
			return
		}
		if len(kinds) != 1 {
			log.Error(nil, "Expected exactly 1 kind for OwnerType",
				"OwnerType", e.OwnerType, "Kinds", kinds)
			return
		}
		e.groupKind = schema.GroupKind{Group: kinds[0].Group, Kind: kinds[0].Kind}
		e.kindOk = true
	})
	if !e.kindOk {
		return nil
	}

	reqs := []reconcile.ReconcileRequest{}
	for _, ref := range e.getOwnersReferences(object) {
		// Parse the Group and Version from the OwnerReference
		refGV, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			log.Error(err, "Could not parse OwnerReference GroupVersion",
				"OwnerReference", ref.APIVersion)
			return nil
		}

		// Kind and Group match
		if ref.Kind == e.groupKind.Kind && refGV.Group == e.groupKind.Group {
			reqs = append(reqs, reconcile.ReconcileRequest{
				NamespacedName: types.NamespacedName{
					Namespace: object.GetNamespace(),
					Name:      ref.Name,
				},
			})
		}
	}

	// No matching owners
	return reqs
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
