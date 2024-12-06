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

func wrapHandler[request comparable](handler handler.TypedEventHandler[client.Object, request], cluster cluster.Cluster) handler.TypedEventHandler[client.Object, request] {
	return &wrappedEventHandler[request]{
		handler: handler,
		cluster: cluster,
	}
}

type wrappedEventHandler[request comparable] struct {
	handler handler.TypedEventHandler[client.Object, request]
	cluster cluster.Cluster
}

func (w *wrappedEventHandler[request]) Create(ctx context.Context, e event.TypedCreateEvent[client.Object], q workqueue.TypedRateLimitingInterface[request]) {
	annotations := e.Object.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations["cluster-name"] = w.cluster.Name()
	e.Object.SetAnnotations(annotations)
	w.handler.Create(ctx, e, q)
}

func (w *wrappedEventHandler[request]) Update(ctx context.Context, e event.TypedUpdateEvent[client.Object], q workqueue.TypedRateLimitingInterface[request]) {
	annotations := e.ObjectNew.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations["cluster-name"] = w.cluster.Name()
	e.ObjectNew.SetAnnotations(annotations)
	w.handler.Update(ctx, e, q)
}

func (w *wrappedEventHandler[request]) Delete(ctx context.Context, e event.TypedDeleteEvent[client.Object], q workqueue.TypedRateLimitingInterface[request]) {
	annotations := e.Object.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations["cluster-name"] = w.cluster.Name()
	e.Object.SetAnnotations(annotations)
	w.handler.Delete(ctx, e, q)
}

func (w *wrappedEventHandler[request]) Generic(ctx context.Context, e event.TypedGenericEvent[client.Object], q workqueue.TypedRateLimitingInterface[request]) {
	annotations := e.Object.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations["cluster-name"] = w.cluster.Name()
	e.Object.SetAnnotations(annotations)
	w.handler.Generic(ctx, e, q)
}

var _ handler.TypedEventHandler[client.Object, clusterRequest] = &EnqueueClusterRequestForObject{}

type EnqueueClusterRequestForObject = TypedEnqueueClusterRequestForObject[client.Object]

type TypedEnqueueClusterRequestForObject[object client.Object] struct{}

// Create implements EventHandler.
func (e *TypedEnqueueClusterRequestForObject[T]) Create(ctx context.Context, evt event.TypedCreateEvent[T], q workqueue.TypedRateLimitingInterface[clusterRequest]) {
	if isNil(evt.Object) {
		return
	}

	clusterName := evt.Object.GetAnnotations()["cluster-name"]

	q.Add(clusterRequest{
		ClusterName: clusterName,
		Request: reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      evt.Object.GetName(),
			Namespace: evt.Object.GetNamespace(),
		}}})
}

// Update implements EventHandler.
func (e *TypedEnqueueClusterRequestForObject[T]) Update(ctx context.Context, evt event.TypedUpdateEvent[T], q workqueue.TypedRateLimitingInterface[clusterRequest]) {
	clusterName := evt.ObjectNew.GetAnnotations()["cluster-name"]

	switch {
	case !isNil(evt.ObjectNew):
		q.Add(clusterRequest{
			ClusterName: clusterName,
			Request: reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      evt.ObjectNew.GetName(),
				Namespace: evt.ObjectNew.GetNamespace(),
			}}})
	case !isNil(evt.ObjectOld):
		q.Add(clusterRequest{
			ClusterName: clusterName,
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

	clusterName := evt.Object.GetAnnotations()["cluster-name"]

	q.Add(clusterRequest{
		ClusterName: clusterName,
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

	clusterName := evt.Object.GetAnnotations()["cluster-name"]

	q.Add(clusterRequest{
		ClusterName: clusterName,
		Request: reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      evt.Object.GetName(),
			Namespace: evt.Object.GetNamespace(),
		}}})
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
