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

package internal

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// InformersMap create and caches Informers for (runtime.Object, schema.GroupVersionKind) pairs.
// It uses a standard parameter codec constructed based on the given generated Scheme.
type InformersMap struct {
	// we abstract over the details of structured/unstructured/metadata with the specificInformerMaps
	// TODO(directxman12): genericize this over different projections now that we have 3 different maps
	// TODO(vincepri): A different structure of the specific informer map is possible, although it requires
	// a large refactoring that takes into account that there may be different kind of informers, in this case
	// 3 of those: structured, unstructured, and metadata.

	structured   *specificInformersMap
	unstructured *specificInformersMap
	metadata     *specificInformersMap
}

// InformersMapOptions configures an InformerMap.
type InformersMapOptions struct {
	Scheme       *runtime.Scheme
	Mapper       meta.RESTMapper
	ResyncPeriod time.Duration
	Namespace    string
	ByGVK        InformersMapOptionsByGVK
}

// InformersMapOptionsByGVK configured additional by group version kind (or object)
// in an InformerMap.
type InformersMapOptionsByGVK struct {
	Selectors       SelectorsByGVK
	Transformers    TransformFuncByObject // TODO(vincepri): Why is this by object and not GVK?
	DisableDeepCopy DisableDeepCopyByGVK
}

// NewInformersMap creates a new InformersMap that can create informers for
// both structured and unstructured objects.
func NewInformersMap(config *rest.Config, options *InformersMapOptions) *InformersMap {
	return &InformersMap{
		structured: newSpecificInformersMap(config, &specificInformerMapOptions{
			InformersMapOptions: options,
			ListWatcherFunc:     structuredListWatch,
		}),
		unstructured: newSpecificInformersMap(config, &specificInformerMapOptions{
			InformersMapOptions: options,
			ListWatcherFunc:     unstructuredListWatch,
		}),
		metadata: newSpecificInformersMap(config, &specificInformerMapOptions{
			InformersMapOptions: options,
			ListWatcherFunc:     metadataListWatch,
		}),
	}
}

// Start calls Run on each of the informers and sets started to true.  Blocks on the context.
func (m *InformersMap) Start(ctx context.Context) error {
	go m.structured.Start(ctx)
	go m.unstructured.Start(ctx)
	go m.metadata.Start(ctx)
	<-ctx.Done()
	return nil
}

// WaitForCacheSync waits until all the caches have been started and synced.
func (m *InformersMap) WaitForCacheSync(ctx context.Context) bool {
	syncedFuncs := append([]cache.InformerSynced(nil), m.structured.HasSyncedFuncs()...)
	syncedFuncs = append(syncedFuncs, m.unstructured.HasSyncedFuncs()...)
	syncedFuncs = append(syncedFuncs, m.metadata.HasSyncedFuncs()...)

	if !m.structured.waitForStarted(ctx) {
		return false
	}
	if !m.unstructured.waitForStarted(ctx) {
		return false
	}
	if !m.metadata.waitForStarted(ctx) {
		return false
	}
	return cache.WaitForCacheSync(ctx.Done(), syncedFuncs...)
}

// Get will create a new Informer and add it to the map of InformersMap if none exists.  Returns
// the Informer from the map.
func (m *InformersMap) Get(ctx context.Context, gvk schema.GroupVersionKind, obj runtime.Object) (bool, *MapEntry, error) {
	switch obj.(type) {
	case *unstructured.Unstructured:
		return m.unstructured.Get(ctx, gvk, obj)
	case *unstructured.UnstructuredList:
		return m.unstructured.Get(ctx, gvk, obj)
	case *metav1.PartialObjectMetadata:
		return m.metadata.Get(ctx, gvk, obj)
	case *metav1.PartialObjectMetadataList:
		return m.metadata.Get(ctx, gvk, obj)
	default:
		return m.structured.Get(ctx, gvk, obj)
	}
}
