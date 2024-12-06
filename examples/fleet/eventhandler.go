/*
Copyright 2024 The Kubernetes Authors.

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

package main

import (
	"context"
	"reflect"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ handler.TypedDeepCopyableEventHandler[client.Object, clusterRequest] = &EnqueueClusterRequestForObject{}

type EnqueueClusterRequestForObject = TypedEnqueueClusterRequestForObject[client.Object]

type TypedEnqueueClusterRequestForObject[object client.Object] struct {
	clusterName string
}

// Create implements EventHandler.
func (e *TypedEnqueueClusterRequestForObject[T]) Create(ctx context.Context, evt event.TypedCreateEvent[T], q workqueue.TypedRateLimitingInterface[clusterRequest]) {
	if isNil(evt.Object) {
		return
	}

	q.Add(clusterRequest{
		ClusterName: e.clusterName,
		Request: reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      evt.Object.GetName(),
			Namespace: evt.Object.GetNamespace(),
		}}})
}

// Update implements EventHandler.
func (e *TypedEnqueueClusterRequestForObject[T]) Update(ctx context.Context, evt event.TypedUpdateEvent[T], q workqueue.TypedRateLimitingInterface[clusterRequest]) {

	switch {
	case !isNil(evt.ObjectNew):
		q.Add(clusterRequest{
			ClusterName: e.clusterName,
			Request: reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      evt.ObjectNew.GetName(),
				Namespace: evt.ObjectNew.GetNamespace(),
			}}})
	case !isNil(evt.ObjectOld):
		q.Add(clusterRequest{
			ClusterName: e.clusterName,
			Request: reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      evt.ObjectOld.GetName(),
				Namespace: evt.ObjectOld.GetNamespace(),
			}}})
	}
}

// Delete implements EventHandler.
func (e *TypedEnqueueClusterRequestForObject[T]) Delete(ctx context.Context, evt event.TypedDeleteEvent[T], q workqueue.TypedRateLimitingInterface[clusterRequest]) {
	if isNil(evt.Object) {
		return
	}

	q.Add(clusterRequest{
		ClusterName: e.clusterName,
		Request: reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      evt.Object.GetName(),
			Namespace: evt.Object.GetNamespace(),
		}}})
}

// Generic implements EventHandler.
func (e *TypedEnqueueClusterRequestForObject[T]) Generic(ctx context.Context, evt event.TypedGenericEvent[T], q workqueue.TypedRateLimitingInterface[clusterRequest]) {
	if isNil(evt.Object) {
		return
	}

	q.Add(clusterRequest{
		ClusterName: e.clusterName,
		Request: reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      evt.Object.GetName(),
			Namespace: evt.Object.GetNamespace(),
		}}})
}

func (e *TypedEnqueueClusterRequestForObject[T]) DeepCopyFor(c cluster.Cluster) handler.TypedDeepCopyableEventHandler[T, clusterRequest] {
	if e == nil {
		return nil
	}

	out := new(TypedEnqueueClusterRequestForObject[T])
	*out = *e
	out.clusterName = c.Name()

	return out
}

func isNil(arg any) bool {
	if v := reflect.ValueOf(arg); !v.IsValid() || ((v.Kind() == reflect.Ptr ||
		v.Kind() == reflect.Interface ||
		v.Kind() == reflect.Slice ||
		v.Kind() == reflect.Map ||
		v.Kind() == reflect.Chan ||
		v.Kind() == reflect.Func) && v.IsNil()) {
		return true
	}
	return false
}
