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

package cache

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// informersMap defines an interface that allows access to the internal informers of a cache
type informersMap interface {
	// VisitInformers iterates through all informers and calls the visitor function
	VisitInformers(visitor func(gvk schema.GroupVersionKind, informer cache.SharedIndexInformer))
}

// hasInformers defines an interface that a cache must implement to use the DumpCacheResourceMetrics function
type hasInformers interface {
	// Informers returns an object that implements the informersMap interface
	Informers() interface{}
}

// DumpCacheResourceMetrics manually updates metrics for all resources
// currently in the cache. This can be useful for initialization or
// to force a refresh of the metrics.
func DumpCacheResourceMetrics(ctx context.Context, c Cache) error {
	// First check if the cache implements the hasInformers interface
	cacheWithInformers, ok := c.(hasInformers)
	if !ok {
		return fmt.Errorf("cache does not implement necessary interface to access informers")
	}

	// Get the informers
	informers := cacheWithInformers.Informers()

	// Try to convert it to the informersMap interface
	informersMap, ok := informers.(informersMap)
	if !ok {
		return fmt.Errorf("cache.Informers() does not return a valid informers map")
	}

	// Visit all informers and update metrics
	informersMap.VisitInformers(func(gvk schema.GroupVersionKind, informer cache.SharedIndexInformer) {
		count := len(informer.GetIndexer().List())
		metrics.RecordCacheResourceCount(gvk, count)
	})

	return nil
}
