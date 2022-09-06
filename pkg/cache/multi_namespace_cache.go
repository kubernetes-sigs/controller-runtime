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
	"time"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/internal/objectutil"
)

// NewCacheFunc - Function for creating a new cache from the options and a rest config.
type NewCacheFunc func(config *rest.Config, opts Options) (Cache, error)

// a new global namespaced cache to handle cluster scoped resources.
const globalCache = "_cluster-scope"

// MultiNamespacedOption is a function that modifies a MultiNamespacedOptions.
type MultiNamespacedOption func(*MultiNamespacedOptions)

// MultiNamespacedOptions is used to configure the functions used to create caches
// on a per-namespace basis.
type MultiNamespacedOptions struct {
	NewNamespacedCaches       map[string]NewCacheFunc
	NewClusterScopedCache     NewCacheFunc
	NewDefaultNamespacedCache NewCacheFunc
}

// WithLegacyNamespacedCaches configures the MultiNamespacedCacheWithOptionsBuilder
// with standard caches in each of the namespaces provided as well as for cluster-scoped
// objects. This option enables use of the MultiNamespacedCacheWithOptionsBuilder
// to match the behavior of the deprecated MultiNamespacedCacheBuilder.
func WithLegacyNamespacedCaches(namespaces []string) MultiNamespacedOption {
	return func(options *MultiNamespacedOptions) {
		WithNamespacedCaches(namespaces, New)(options)
		WithClusterScopedCache(New)(options)
	}
}

// WithNamespacedCaches configures MultiNamespacedCacheWithOptionsBuilder
// with namespace-specific caches that are created using the provided NewCacheFunc.
func WithNamespacedCaches(namespaces []string, f NewCacheFunc) MultiNamespacedOption {
	return func(options *MultiNamespacedOptions) {
		for _, ns := range namespaces {
			WithNamespacedCache(ns, f)(options)
		}
	}
}

// WithNamespacedCache configures MultiNamespacedCacheWithOptionsBuilder
// with a namespace cache that uses the provided NewCacheFunc.
func WithNamespacedCache(namespace string, f NewCacheFunc) MultiNamespacedOption {
	return func(options *MultiNamespacedOptions) {
		options.NewNamespacedCaches[namespace] = f
	}
}

// WithClusterScopedCache configures MultiNamespacedCacheWithOptionsBuilder
// with a cache for cluster-scoped objects that uses the provided NewCacheFunc.
func WithClusterScopedCache(f NewCacheFunc) MultiNamespacedOption {
	return func(options *MultiNamespacedOptions) {
		options.NewClusterScopedCache = f
	}
}

// WithDefaultNamespacedCache configures MultiNamespacedCacheWithOptionsBuilder
// with a "catch-all" cache for namespace-scoped objects that are in namespaces
// not explicitly configured on the cache builder.
func WithDefaultNamespacedCache(f NewCacheFunc) MultiNamespacedOption {
	return func(options *MultiNamespacedOptions) {
		options.NewDefaultNamespacedCache = f
	}
}

// MultiNamespacedCacheWithOptionsBuilder builds a composite cache that delegates to per-namespace
// caches built according to the passed MultiNamespacedOption options.
//
// If the set of options passed to MultiNamespacedCacheWithOptionsBuilder results in the NewCacheFunc
// being set multiple times for the same namespace, the last such setting will be used.
//
// If a default namespaced cache is defined (e.g. with WithDefaultNamespacedCache), it will be used
// as a catch-all for objects not in the explicit namespaces configured on the cache builder. The
// default namespaced cache is automatically configured with extra field selectors to avoid duplicate
// caching of objects between namespace-specific caches and this catch-all cache.
//
// If a cluster-scoped cache is defined (e.g. with WithClusterScopedCache), it will be used for
// cluster-scoped objects. If it is undefined, the resulting cache will return an error during read
// operations, reporting that a cluster-scoped cache is not defined.
func MultiNamespacedCacheWithOptionsBuilder(opts ...MultiNamespacedOption) NewCacheFunc {
	multiNamespaceOpts := MultiNamespacedOptions{
		NewNamespacedCaches: map[string]NewCacheFunc{},
	}
	for _, opt := range opts {
		opt(&multiNamespaceOpts)
	}

	return func(config *rest.Config, opts Options) (Cache, error) {
		opts, err := defaultOpts(config, opts)
		if err != nil {
			return nil, err
		}

		var clusterCache Cache
		if multiNamespaceOpts.NewClusterScopedCache != nil {
			clusterCache, err = multiNamespaceOpts.NewClusterScopedCache(config, opts)
			if err != nil {
				return nil, err
			}
		}

		nsToCache := map[string]Cache{}
		if multiNamespaceOpts.NewDefaultNamespacedCache != nil {
			defaultNamespacedOpts := setDefaultNamespacedCacheOpts(opts, multiNamespaceOpts.NewNamespacedCaches)
			defaultNamespacedCache, err := multiNamespaceOpts.NewDefaultNamespacedCache(config, defaultNamespacedOpts)
			if err != nil {
				return nil, err
			}
			nsToCache[corev1.NamespaceAll] = defaultNamespacedCache
		}

		for ns, newCacheFunc := range multiNamespaceOpts.NewNamespacedCaches {
			opts.Namespace = ns
			nsToCache[ns], err = newCacheFunc(config, opts)
			if err != nil {
				return nil, err
			}
		}

		return &multiNamespaceCache{
			namespaceToCache: nsToCache,
			clusterCache:     clusterCache,
			RESTMapper:       opts.Mapper,
			Scheme:           opts.Scheme,
		}, nil
	}
}

func setDefaultNamespacedCacheOpts(opts Options, newObjectCaches map[string]NewCacheFunc) Options {
	fieldSelectors := []fields.Selector{}
	if opts.DefaultSelector.Field != nil {
		fieldSelectors = append(fieldSelectors, opts.DefaultSelector.Field)
	}
	for ns := range newObjectCaches {
		fieldSelectors = append(fieldSelectors, fields.OneTermNotEqualSelector("metadata.namespace", ns))
	}
	opts.DefaultSelector.Field = fields.AndSelectors(fieldSelectors...)
	return opts
}

// MultiNamespacedCacheBuilder - Builder function to create a new multi-namespaced cache.
// This will scope the cache to a list of namespaces. Listing for all namespaces
// will list for all the namespaces that this knows about. By default this will create
// a global cache for cluster scoped resource. Note that this is not intended
// to be used for excluding namespaces, this is better done via a Predicate. Also note that
// you may face performance issues when using this with a high number of namespaces.
//
// Deprecated: Use MultiNamespacedCacheWithOptionsBuilder instead:
//
//	  cache.MultiNamespacedCacheWithOptionsBuilder(
//		   WithLegacyNamespacedCaches(namespaces),
//	  )
func MultiNamespacedCacheBuilder(namespaces []string) NewCacheFunc {
	return MultiNamespacedCacheWithOptionsBuilder(
		WithLegacyNamespacedCaches(namespaces),
	)
}

// multiNamespaceCache knows how to handle multiple namespaced caches
// Use this feature when scoping permissions for your
// operator to a list of namespaces instead of watching every namespace
// in the cluster.
type multiNamespaceCache struct {
	namespaceToCache map[string]Cache
	Scheme           *runtime.Scheme
	RESTMapper       apimeta.RESTMapper
	clusterCache     Cache
}

var _ Cache = &multiNamespaceCache{}

// Methods for multiNamespaceCache to conform to the Informers interface.
func (c *multiNamespaceCache) GetInformer(ctx context.Context, obj client.Object) (Informer, error) {
	informers := map[string]Informer{}

	// If the object is clusterscoped, get the informer from clusterCache,
	// if not use the namespaced caches.
	isNamespaced, err := objectutil.IsAPINamespaced(obj, c.Scheme, c.RESTMapper)
	if err != nil {
		return nil, err
	}
	if !isNamespaced {
		if c.clusterCache != nil {
			clusterCacheInf, err := c.clusterCache.GetInformer(ctx, obj)
			if err != nil {
				return nil, err
			}
			informers[globalCache] = clusterCacheInf
		}
		return &multiNamespaceInformer{namespaceToInformer: informers}, nil
	}

	for ns, cache := range c.namespaceToCache {
		informer, err := cache.GetInformer(ctx, obj)
		if err != nil {
			return nil, err
		}
		informers[ns] = informer
	}

	return &multiNamespaceInformer{namespaceToInformer: informers}, nil
}

func (c *multiNamespaceCache) GetInformerForKind(ctx context.Context, gvk schema.GroupVersionKind) (Informer, error) {
	informers := map[string]Informer{}

	// If the object is clusterscoped, get the informer from clusterCache,
	// if not use the namespaced caches.
	isNamespaced, err := objectutil.IsAPINamespacedWithGVK(gvk, c.Scheme, c.RESTMapper)
	if err != nil {
		return nil, err
	}
	if !isNamespaced {
		if c.clusterCache != nil {
			clusterCacheInf, err := c.clusterCache.GetInformerForKind(ctx, gvk)
			if err != nil {
				return nil, err
			}
			informers[globalCache] = clusterCacheInf
		}
		return &multiNamespaceInformer{namespaceToInformer: informers}, nil
	}

	for ns, cache := range c.namespaceToCache {
		informer, err := cache.GetInformerForKind(ctx, gvk)
		if err != nil {
			return nil, err
		}
		informers[ns] = informer
	}

	return &multiNamespaceInformer{namespaceToInformer: informers}, nil
}

func (c *multiNamespaceCache) Start(ctx context.Context) error {
	// start global cache
	if c.clusterCache != nil {
		go func() {
			err := c.clusterCache.Start(ctx)
			if err != nil {
				log.Error(err, "cluster scoped cache failed to start")
			}
		}()
	}

	// start namespaced caches
	for ns, cache := range c.namespaceToCache {
		go func(ns string, cache Cache) {
			err := cache.Start(ctx)
			if err != nil {
				log.Error(err, "multinamespace cache failed to start namespaced informer", "namespace", ns)
			}
		}(ns, cache)
	}

	<-ctx.Done()
	return nil
}

func (c *multiNamespaceCache) WaitForCacheSync(ctx context.Context) bool {
	synced := true
	for _, cache := range c.namespaceToCache {
		if s := cache.WaitForCacheSync(ctx); !s {
			synced = s
		}
	}

	// check if cluster scoped cache has synced
	if c.clusterCache != nil && !c.clusterCache.WaitForCacheSync(ctx) {
		synced = false
	}
	return synced
}

func (c *multiNamespaceCache) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	isNamespaced, err := objectutil.IsAPINamespaced(obj, c.Scheme, c.RESTMapper)
	if err != nil {
		return nil //nolint:nilerr
	}

	if !isNamespaced {
		if c.clusterCache == nil {
			return nil
		}
		return c.clusterCache.IndexField(ctx, obj, field, extractValue)
	}

	for _, cache := range c.namespaceToCache {
		if err := cache.IndexField(ctx, obj, field, extractValue); err != nil {
			return err
		}
	}
	return nil
}

func (c *multiNamespaceCache) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	isNamespaced, err := objectutil.IsAPINamespaced(obj, c.Scheme, c.RESTMapper)
	if err != nil {
		return err
	}

	if !isNamespaced {
		if c.clusterCache == nil {
			return fmt.Errorf("unable to get: %v because cluster-scoped cache does not exist", key)
		}
		// Look into the global cache to fetch the object
		return c.clusterCache.Get(ctx, key, obj)
	}

	cache, ok := c.namespaceToCache[key.Namespace]
	if !ok {
		// Use the default/catch-all namespace cache if we have one.
		cache, ok = c.namespaceToCache[corev1.NamespaceAll]
	}
	if !ok {
		return fmt.Errorf("unable to get %q: neither a per-namespace nor a default namespaced cache exists", key)
	}
	return cache.Get(ctx, key, obj)
}

// List multi namespace cache will get all the objects in the namespaces that the cache is watching if asked for all namespaces.
func (c *multiNamespaceCache) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	listOpts := client.ListOptions{}
	listOpts.ApplyOptions(opts)

	isNamespaced, err := objectutil.IsAPINamespaced(list, c.Scheme, c.RESTMapper)
	if err != nil {
		return err
	}

	if !isNamespaced {
		if c.clusterCache == nil {
			return fmt.Errorf("unable to get because cluster-scoped cache does not exist")
		}
		// Look at the global cache to get the objects with the specified GVK
		return c.clusterCache.List(ctx, list, opts...)
	}

	if listOpts.Namespace != corev1.NamespaceAll {
		cache, ok := c.namespaceToCache[listOpts.Namespace]
		if !ok {
			// Use the default/catch-all namespace cache if we have one.
			cache, ok = c.namespaceToCache[corev1.NamespaceAll]
		}
		if !ok {
			return fmt.Errorf("unable to list in namespace %q: neither a per-namespace nor a default namespaced cache exists", listOpts.Namespace)
		}
		return cache.List(ctx, list, opts...)
	}

	listAccessor, err := apimeta.ListAccessor(list)
	if err != nil {
		return err
	}

	allItems, err := apimeta.ExtractList(list)
	if err != nil {
		return err
	}

	limitSet := listOpts.Limit > 0

	var resourceVersion string
	for _, cache := range c.namespaceToCache {
		listObj := list.DeepCopyObject().(client.ObjectList)
		err = cache.List(ctx, listObj, &listOpts)
		if err != nil {
			return err
		}
		items, err := apimeta.ExtractList(listObj)
		if err != nil {
			return err
		}
		accessor, err := apimeta.ListAccessor(listObj)
		if err != nil {
			return fmt.Errorf("object: %T must be a list type", list)
		}
		allItems = append(allItems, items...)
		// The last list call should have the most correct resource version.
		resourceVersion = accessor.GetResourceVersion()
		if limitSet {
			// decrement Limit by the number of items
			// fetched from the current namespace.
			listOpts.Limit -= int64(len(items))
			// if a Limit was set and the number of
			// items read has reached this set limit,
			// then stop reading.
			if listOpts.Limit == 0 {
				break
			}
		}
	}
	listAccessor.SetResourceVersion(resourceVersion)

	return apimeta.SetList(list, allItems)
}

// multiNamespaceInformer knows how to handle interacting with the underlying informer across multiple namespaces.
type multiNamespaceInformer struct {
	namespaceToInformer map[string]Informer
}

var _ Informer = &multiNamespaceInformer{}

// AddEventHandler adds the handler to each namespaced informer.
func (i *multiNamespaceInformer) AddEventHandler(handler toolscache.ResourceEventHandler) {
	for _, informer := range i.namespaceToInformer {
		informer.AddEventHandler(handler)
	}
}

// AddEventHandlerWithResyncPeriod adds the handler with a resync period to each namespaced informer.
func (i *multiNamespaceInformer) AddEventHandlerWithResyncPeriod(handler toolscache.ResourceEventHandler, resyncPeriod time.Duration) {
	for _, informer := range i.namespaceToInformer {
		informer.AddEventHandlerWithResyncPeriod(handler, resyncPeriod)
	}
}

// AddIndexers adds the indexer for each namespaced informer.
func (i *multiNamespaceInformer) AddIndexers(indexers toolscache.Indexers) error {
	for _, informer := range i.namespaceToInformer {
		err := informer.AddIndexers(indexers)
		if err != nil {
			return err
		}
	}
	return nil
}

// HasSynced checks if each namespaced informer has synced.
func (i *multiNamespaceInformer) HasSynced() bool {
	for _, informer := range i.namespaceToInformer {
		if ok := informer.HasSynced(); !ok {
			return ok
		}
	}
	return true
}
