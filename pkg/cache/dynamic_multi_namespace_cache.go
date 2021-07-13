/*
Copyright 2019 The Kubernetes Authors.

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
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DynamicMultiNamespacedCacheBuilder - Builder function to create a new dynamic
// multi-namespaced cache. Listing for all namespaces from the namespaces that
// were previously used in gets and namespace-scoped lists.
//
// This is useful if you expect your controller a) to have limited list/watch
// permissions (i.e. not cluster-scoped) and b) to dynamically handle receiving
// permissions on a new namespace.
//
// Note that you may face performance issues when using this with a high number
// of namespaces.
func DynamicMultiNamespacedCacheBuilder() NewCacheFunc {
	return func(config *rest.Config, opts Options) (Cache, error) {
		opts, err := defaultOpts(config, opts)
		if err != nil {
			return nil, err
		}
		return &dynamicMultiNamespaceCache{
			restConfig: config,
			opts:       opts,
			cache: &multiNamespaceCache{
				namespaceToCache: map[string]Cache{},
				Scheme:           opts.Scheme,
			},
		}, nil
	}
}

// dynamicMultiNamespaceCache knows how to handle multiple namespaced caches
// Use this feature when scoping permissions for your
// operator to a list of namespaces instead of watching every namespace
// in the cluster.
type dynamicMultiNamespaceCache struct {
	restConfig *rest.Config
	opts       Options

	cache *multiNamespaceCache
	m     sync.RWMutex

	started bool
	stopCtx context.Context
}

var _ Cache = &dynamicMultiNamespaceCache{}

func (c *dynamicMultiNamespaceCache) getOrCreateCache(ctx context.Context, namespace string) (Cache, error) {
	if cache, ok := c.caches()[namespace]; ok {
		return cache, nil
	}

	opts := c.opts
	opts.Namespace = namespace
	nsCache, err := New(c.restConfig, opts)
	if err != nil {
		return nil, err
	}

	c.m.Lock()
	defer c.m.Unlock()
	if c.started {
		go nsCache.Start(c.stopCtx)
		if !nsCache.WaitForCacheSync(ctx) {
			return nil, fmt.Errorf("failed to sync cache for namespace %q\n", namespace)
		}
	}
	c.cache.namespaceToCache[namespace] = nsCache
	return nsCache, nil
}

func (c *dynamicMultiNamespaceCache) caches() map[string]Cache {
	c.m.RLock()
	defer c.m.RUnlock()
	caches := make(map[string]Cache, len(c.cache.namespaceToCache))
	for k, v := range c.cache.namespaceToCache {
		caches[k] = v
	}
	return caches
}

// Methods for dynamicMultiNamespaceCache to conform to the Informers interface
func (c *dynamicMultiNamespaceCache) GetInformer(ctx context.Context, obj client.Object) (Informer, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	return c.cache.GetInformer(ctx, obj)
}

func (c *dynamicMultiNamespaceCache) GetInformerForKind(ctx context.Context, gvk schema.GroupVersionKind) (Informer, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	return c.cache.GetInformerForKind(ctx, gvk)
}

func (c *dynamicMultiNamespaceCache) Start(ctx context.Context) error {
	func() {
		c.m.Lock()
		defer c.m.Unlock()

		// Set the stop channel so it can be passed to informers that are added later
		c.stopCtx = ctx

		go c.cache.Start(ctx)

		// Set started to true so we immediately start any informers added later.
		c.started = true
	}()
	<-ctx.Done()
	return nil
}

func (c *dynamicMultiNamespaceCache) WaitForCacheSync(ctx context.Context) bool {
	c.m.RLock()
	defer c.m.RUnlock()
	return c.cache.WaitForCacheSync(ctx)
}

func (c *dynamicMultiNamespaceCache) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	c.m.RLock()
	defer c.m.RUnlock()
	return c.cache.IndexField(ctx, obj, field, extractValue)
}

func (c *dynamicMultiNamespaceCache) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	cache, err := c.getOrCreateCache(ctx, key.Namespace)
	if err != nil {
		return err
	}
	return cache.Get(ctx, key, obj)
}

// List dynamic multi namespace cache will get all the objects in the namespaces that the cache is watching if asked for all namespaces.
func (c *dynamicMultiNamespaceCache) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	listOpts := client.ListOptions{}
	listOpts.ApplyOptions(opts)
	if listOpts.Namespace != corev1.NamespaceAll {
		nsCache, err := c.getOrCreateCache(ctx, listOpts.Namespace)
		if err != nil {
			return err
		}
		nsCache.List(ctx, list, opts...)
	}
	return c.cache.List(ctx, list, opts...)
}
