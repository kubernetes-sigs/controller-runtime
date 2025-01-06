/*
Copyright 2025 The Kubernetes Authors.

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

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ TypedEventHandler[client.Object, reconcile.ClusterAwareRequest] = &enqueueClusterAwareRequestForOwner[client.Object]{}

// EnqueueClusterAwareRequestForOwner enqueues ClusterAwareRequests for the Owners of an object.  E.g. the object that created
// the object that was the source of the Event.
//
// If a ReplicaSet creates Pods, users may reconcile the ReplicaSet in response to Pod Events using:
//
// - a source.Kind Source with Type of Pod.
//
// - a handler.enqueueClusterAwareRequestForOwner EventHandler with an OwnerType of ReplicaSet and OnlyControllerOwner set to true.
func EnqueueClusterAwareRequestForOwner(scheme *runtime.Scheme, mapper meta.RESTMapper, ownerType client.Object, cl cluster.Cluster, opts ...OwnerOption) TypedEventHandler[client.Object, reconcile.ClusterAwareRequest] {
	return TypedEnqueueClusterAwareRequestForOwner[client.Object](scheme, mapper, ownerType, cl, opts...)
}

// TypedEnqueueClusterAwareRequestForOwner enqueues ClusterAwareRequests for the Owners of an object.  E.g. the object that created
// the object that was the source of the Event.
//
// If a ReplicaSet creates Pods, users may reconcile the ReplicaSet in response to Pod Events using:
//
// - a source.Kind Source with Type of Pod.
//
// - a handler.typedEnqueueRequestForOwner EventHandler with an OwnerType of ReplicaSet and OnlyControllerOwner set to true.
//
// TypedEnqueueClusterAwareRequestForOwner is experimental and subject to future change.
func TypedEnqueueClusterAwareRequestForOwner[object client.Object](scheme *runtime.Scheme, mapper meta.RESTMapper, ownerType client.Object, cl cluster.Cluster, opts ...OwnerOption) TypedEventHandler[object, reconcile.ClusterAwareRequest] {
	e := &enqueueClusterAwareRequestForOwner[object]{
		enqueueRequestForOwner: enqueueRequestForOwner[object]{
			ownerType: ownerType,
			mapper:    mapper,
		},
		clusterName: cl.Name(),
	}
	if err := e.parseOwnerTypeGroupKind(scheme); err != nil {
		panic(err)
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

type enqueueClusterAwareRequestForOwner[object client.Object] struct {
	enqueueRequestForOwner[object]
	clusterName string
}

// Create implements TypedEventHandler.
func (e *enqueueClusterAwareRequestForOwner[object]) Create(ctx context.Context, evt event.TypedCreateEvent[object], q workqueue.TypedRateLimitingInterface[reconcile.ClusterAwareRequest]) {
	reqs := map[reconcile.Request]empty{}
	e.getOwnerReconcileRequest(evt.Object, reqs)
	for req := range reqs {
		q.Add(reconcile.ClusterAwareRequest{Request: req, ClusterName: e.clusterName})
	}
}

// Update implements TypedEventHandler.
func (e *enqueueClusterAwareRequestForOwner[object]) Update(ctx context.Context, evt event.TypedUpdateEvent[object], q workqueue.TypedRateLimitingInterface[reconcile.ClusterAwareRequest]) {
	reqs := map[reconcile.Request]empty{}
	e.getOwnerReconcileRequest(evt.ObjectOld, reqs)
	e.getOwnerReconcileRequest(evt.ObjectNew, reqs)
	for req := range reqs {
		q.Add(reconcile.ClusterAwareRequest{Request: req, ClusterName: e.clusterName})
	}
}

// Delete implements TypedEventHandler.
func (e *enqueueClusterAwareRequestForOwner[object]) Delete(ctx context.Context, evt event.TypedDeleteEvent[object], q workqueue.TypedRateLimitingInterface[reconcile.ClusterAwareRequest]) {
	reqs := map[reconcile.Request]empty{}
	e.getOwnerReconcileRequest(evt.Object, reqs)
	for req := range reqs {
		q.Add(reconcile.ClusterAwareRequest{Request: req, ClusterName: e.clusterName})
	}
}

// Generic implements TypedEventHandler.
func (e *enqueueClusterAwareRequestForOwner[object]) Generic(ctx context.Context, evt event.TypedGenericEvent[object], q workqueue.TypedRateLimitingInterface[reconcile.ClusterAwareRequest]) {
	reqs := map[reconcile.Request]empty{}
	e.getOwnerReconcileRequest(evt.Object, reqs)
	for req := range reqs {
		q.Add(reconcile.ClusterAwareRequest{Request: req, ClusterName: e.clusterName})
	}
}
