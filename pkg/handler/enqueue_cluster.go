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

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/internal/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var clusterenqueueLog = logf.RuntimeLog.WithName("eventhandler").WithName("EnqueueClusterAwareRequestForObject")

var _ TypedEventHandler[client.Object, reconcile.ClusterAwareRequest] = &EnqueueClusterAwareRequestForObject{}
var _ TypedDeepCopyableEventHandler[client.Object, reconcile.ClusterAwareRequest] = &EnqueueClusterAwareRequestForObject{}

// EnqueueClusterAwareRequestForObject enqueues a ClusterAwareRequest containing the Name, Namespace and ClusterName of the object that is the source of the Event.
// (e.g. the created / deleted / updated objects Name and Namespace). handler.EnqueueClusterAwareRequestForObject should be used by multi-cluster
// Controllers that have associated Resources (e.g. CRDs) to reconcile the associated Resource.
type EnqueueClusterAwareRequestForObject = TypedEnqueueClusterAwareRequestForObject[client.Object]

// TypedEnqueueClusterAwareRequestForObject enqueues a ClusterAwareRequest containing the Name, Namespace and ClusterName of the object that is the source of the Event.
// (e.g. the created / deleted / updated objects Name and Namespace).  handler.TypedEnqueueClusterAwareRequestForObject should be used by multi-cluster
// Controllers that have associated Resources (e.g. CRDs) to reconcile the associated Resource.
//
// TypedEnqueueClusterAwareRequestForObject is experimental and subject to future change.
type TypedEnqueueClusterAwareRequestForObject[object client.Object] struct {
	ClusterName string
}

// Create implements EventHandler.
func (e *TypedEnqueueClusterAwareRequestForObject[T]) Create(ctx context.Context, evt event.TypedCreateEvent[T], q workqueue.TypedRateLimitingInterface[reconcile.ClusterAwareRequest]) {
	if isNil(evt.Object) {
		clusterenqueueLog.Error(nil, "CreateEvent received with no metadata", "event", evt)
		return
	}
	q.Add(reconcile.ClusterAwareRequest{
		Request: reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      evt.Object.GetName(),
			Namespace: evt.Object.GetNamespace(),
		}},
		ClusterName: e.ClusterName,
	})
}

// Update implements EventHandler.
func (e *TypedEnqueueClusterAwareRequestForObject[T]) Update(ctx context.Context, evt event.TypedUpdateEvent[T], q workqueue.TypedRateLimitingInterface[reconcile.ClusterAwareRequest]) {
	switch {
	case !isNil(evt.ObjectNew):
		q.Add(reconcile.ClusterAwareRequest{
			Request: reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      evt.ObjectNew.GetName(),
				Namespace: evt.ObjectNew.GetNamespace(),
			}},
			ClusterName: e.ClusterName,
		})
	case !isNil(evt.ObjectOld):
		q.Add(
			reconcile.ClusterAwareRequest{
				Request: reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      evt.ObjectOld.GetName(),
					Namespace: evt.ObjectOld.GetNamespace(),
				}},
				ClusterName: e.ClusterName,
			})
	default:
		clusterenqueueLog.Error(nil, "UpdateEvent received with no metadata", "event", evt)
	}
}

// Delete implements EventHandler.
func (e *TypedEnqueueClusterAwareRequestForObject[T]) Delete(ctx context.Context, evt event.TypedDeleteEvent[T], q workqueue.TypedRateLimitingInterface[reconcile.ClusterAwareRequest]) {
	if isNil(evt.Object) {
		clusterenqueueLog.Error(nil, "DeleteEvent received with no metadata", "event", evt)
		return
	}
	q.Add(reconcile.ClusterAwareRequest{
		Request: reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      evt.Object.GetName(),
			Namespace: evt.Object.GetNamespace(),
		}},
		ClusterName: e.ClusterName,
	})
}

// Generic implements EventHandler.
func (e *TypedEnqueueClusterAwareRequestForObject[T]) Generic(ctx context.Context, evt event.TypedGenericEvent[T], q workqueue.TypedRateLimitingInterface[reconcile.ClusterAwareRequest]) {
	if isNil(evt.Object) {
		clusterenqueueLog.Error(nil, "GenericEvent received with no metadata", "event", evt)
		return
	}
	q.Add(reconcile.ClusterAwareRequest{
		Request: reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      evt.Object.GetName(),
			Namespace: evt.Object.GetNamespace(),
		}},
		ClusterName: e.ClusterName,
	})
}

// DeepCopyFor implements TypedDeepCopyableEventHandler.
func (e *TypedEnqueueClusterAwareRequestForObject[T]) DeepCopyFor(c cluster.Cluster) TypedDeepCopyableEventHandler[T, reconcile.ClusterAwareRequest] {
	if e == nil {
		return nil
	}

	out := new(TypedEnqueueClusterAwareRequestForObject[T])
	*out = *e
	out.ClusterName = c.Name()

	return out
}
