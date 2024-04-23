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

package handler

import (
	"context"
	"time"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ForCluster wraps an EventHandler and adds the cluster name to the reconcile.Requests.
func ForCluster(clusterName string, h EventHandler) EventHandler {
	return &clusterAwareHandler{
		clusterName: clusterName,
		handler:     h,
	}
}

type clusterAwareHandler struct {
	handler     EventHandler
	clusterName string
}

var _ EventHandler = &clusterAwareHandler{}

func (c *clusterAwareHandler) Create(ctx context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	c.handler.Create(ctx, evt, &clusterWorkqueue{RateLimitingInterface: q, clusterName: c.clusterName})
}

func (c *clusterAwareHandler) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	c.handler.Update(ctx, evt, &clusterWorkqueue{RateLimitingInterface: q, clusterName: c.clusterName})
}

func (c *clusterAwareHandler) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	c.handler.Delete(ctx, evt, &clusterWorkqueue{RateLimitingInterface: q, clusterName: c.clusterName})
}

func (c *clusterAwareHandler) Generic(ctx context.Context, evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	c.handler.Generic(ctx, evt, &clusterWorkqueue{RateLimitingInterface: q, clusterName: c.clusterName})
}

// clusterWorkqueue is a wrapper around a RateLimitingInterface that adds the
// cluster name to the reconcile.Requests
type clusterWorkqueue struct {
	workqueue.RateLimitingInterface
	clusterName string
}

func (q *clusterWorkqueue) AddAfter(item interface{}, duration time.Duration) {
	req := item.(reconcile.Request)
	req.ClusterName = q.clusterName
	q.RateLimitingInterface.AddAfter(req, duration)
}

func (q *clusterWorkqueue) Add(item interface{}) {
	req := item.(reconcile.Request)
	req.ClusterName = q.clusterName
	q.RateLimitingInterface.Add(req)
}

func (q *clusterWorkqueue) AddRateLimited(item interface{}) {
	req := item.(reconcile.Request)
	req.ClusterName = q.clusterName
	q.RateLimitingInterface.AddRateLimited(req)
}
