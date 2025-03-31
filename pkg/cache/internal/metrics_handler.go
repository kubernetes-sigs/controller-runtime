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

package internal

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// NewMetricsResourceEventHandler creates a new metrics-collecting event handler for an informer.
// It counts resource additions, updates, and deletions and records them in metrics.
func NewMetricsResourceEventHandler(gvk schema.GroupVersionKind, informer cache.SharedIndexInformer) cache.ResourceEventHandler {
	handler := &metricsResourceEventHandler{
		gvk:      gvk,
		informer: informer,
	}

	// Initialize the initial count
	handler.updateCount()

	return handler
}

// metricsResourceEventHandler implements cache.ResourceEventHandler interface
// to collect metrics about resources in the cache
type metricsResourceEventHandler struct {
	gvk      schema.GroupVersionKind
	informer cache.SharedIndexInformer
}

// OnAdd is called when an object is added.
func (h *metricsResourceEventHandler) OnAdd(obj interface{}, isInInitialList bool) {
	h.updateCount()
}

// OnUpdate is called when an object is modified.
func (h *metricsResourceEventHandler) OnUpdate(oldObj, newObj interface{}) {
	// No need to update counts on update as the total count hasn't changed
}

// OnDelete is called when an object is deleted.
func (h *metricsResourceEventHandler) OnDelete(obj interface{}) {
	h.updateCount()
}

// updateCount updates the metrics with the current count of resources.
func (h *metricsResourceEventHandler) updateCount() {
	count := len(h.informer.GetIndexer().ListKeys())
	metrics.RecordCacheResourceCount(h.gvk, count)
}
