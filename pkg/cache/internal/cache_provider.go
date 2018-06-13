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
	"reflect"
	"sync"

	"fmt"

	"context"

	"github.com/kubernetes-sigs/controller-runtime/pkg/client"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

func NewCacheProvider() *CacheProvider {
	return &CacheProvider{
		cachesByType: make(map[reflect.Type]*ObjectCache),
	}
}

// CacheProvider instantiates individual object caches and delegates to them.  Individual object
// caches are backed by informers / indexers.
type CacheProvider struct {
	// cachesByType is the cache of object caches keyed by type
	cachesByType map[reflect.Type]*ObjectCache

	// informerProvider is used to get the informers
	informerProvider *InformerProvider

	// mu guards access to the map
	mu sync.Mutex
}

// SetInformerProvider sets the InformerProvider used to initialize new ObjectCaches
func (cp *CacheProvider) SetInformerProvider(ip *InformerProvider) {
	cp.informerProvider = ip
}

// addInformer adds a new cache to cachesByType if none is found.
func (cp *CacheProvider) addInformer(obj runtime.Object, gvk schema.GroupVersionKind, c cache.SharedIndexInformer) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if cp.contains(obj) {
		return
	}

	objType := reflect.TypeOf(obj)
	cp.cachesByType[objType] = &ObjectCache{indexer: c.GetIndexer(), groupVersionKind: gvk}
}

// contains returns true if there is already an object cache for the obj type
func (cp *CacheProvider) contains(obj runtime.Object) bool {
	objType := reflect.TypeOf(obj)
	_, found := cp.cachesByType[objType]
	return found
}

// getCache fetches the cache for the object from the map, or creates a new cache if none is found.
func (cp *CacheProvider) getCache(obj runtime.Object) (*ObjectCache, error) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	// initializes an informer if it isn't already in the cache
	if !cp.contains(obj) {
		_, err := cp.informerProvider.GetInformer(obj)
		if err != nil {
			return nil, err
		}
	}

	if !cp.informerProvider.started {
		return nil, fmt.Errorf("must start Cache before calling Get or List %s %s",
			"Object", fmt.Sprintf("%T", obj))
	}
	objType := reflect.TypeOf(obj)

	cache, isKnown := cp.cachesByType[objType]
	if !isKnown {
		// This should never happen.  The cache should be initialized above.
		return nil, fmt.Errorf("no Cache found for %T - must call GetInformer for %T", obj, obj)
	}
	return cache, nil
}

var _ client.ReadInterface = &CacheProvider{}

// Get implements ReadInterface
func (cp *CacheProvider) Get(ctx context.Context, key client.ObjectKey, out runtime.Object) error {
	cache, err := cp.getCache(out)
	if err != nil {
		return err
	}
	return cache.Get(ctx, key, out)
}

// List implements ReadInterface
func (cp *CacheProvider) List(ctx context.Context, opts *client.ListOptions, out runtime.Object) error {
	itemsPtr, err := apimeta.GetItemsPtr(out)
	if err != nil {
		return nil
	}

	// Initialize the cache if it doesn't exist yet
	ro, ok := itemsPtr.(runtime.Object)
	if ok {
		if _, err := cp.getCache(ro); err != nil {
			return err
		}
	}

	// http://knowyourmeme.com/memes/this-is-fine
	outType := reflect.Indirect(reflect.ValueOf(itemsPtr)).Type().Elem()
	cache, isKnown := cp.cachesByType[outType]
	if !isKnown {
		// This should never happen.  The cache should be initialized above.
		return fmt.Errorf("no Cache found for %T - must call GetInformer for %T", out, out)
	}
	return cache.List(ctx, opts, out)
}
