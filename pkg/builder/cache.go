/*
Copyright 2023 The Kubernetes Authors.

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

package builder

import (
	"errors"
	"time"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Cache is a builder for a cache.Cache.
func Cache() *CacheBuilder {
	return &CacheBuilder{}
}

// RawCacheFactory adapts a cache.NewCacheFunc to a CacheFactory.
func RawCacheFactory(fn cache.NewCacheFunc) CacheFactory {
	return &rawCacheFactory{factory: fn}
}

type rawCacheFactory struct {
	factory cache.NewCacheFunc
}

func (r *rawCacheFactory) Factory() cache.NewCacheFunc {
	return r.factory
}

// CacheFactory has a Factory method that builds a cache.NewCacheFunc.
type CacheFactory interface {
	// Factory builds a cache.NewCacheFunc from the CacheBuilder.
	Factory() cache.NewCacheFunc
}

// CacheBuilder builds a cache.Cache.
type CacheBuilder struct {
	opts []cache.SetOptions
}

// Factory builds a cache.NewCacheFunc from the CacheBuilder.
func (c *CacheBuilder) Factory() cache.NewCacheFunc {
	return func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
		for _, opt := range c.opts {
			if err := opt.ApplyToCache(&opts); err != nil {
				return nil, err
			}
		}
		return cache.New(config, opts)
	}
}

// Build builds a cache.Cache from the CacheBuilder.
func (c *CacheBuilder) Build(config *rest.Config) (cache.Cache, error) {
	return c.Factory()(config, cache.Options{})
}

// Scheme sets the scheme for the cache.
func (c *CacheBuilder) Scheme(scheme *runtime.Scheme) *CacheBuilder {
	c.opts = append(c.opts, cache.SetOptionsFunc(func(o *cache.Options) error {
		if o.Scheme == nil {
			o.Scheme = scheme
		}
		return nil
	}))
	return c
}

// RESTMapper sets the RESTMapper for the cache.
func (c *CacheBuilder) RESTMapper(mapper apimeta.RESTMapper) *CacheBuilder {
	c.opts = append(c.opts, cache.SetOptionsFunc(func(o *cache.Options) error {
		if o.Mapper == nil {
			o.Mapper = mapper
		}
		return nil
	}))
	return c
}

// SyncEvery sets the resync period for the cache.
func (c *CacheBuilder) SyncEvery(dur *time.Duration) *CacheBuilder {
	c.opts = append(c.opts, cache.SetOptionsFunc(func(o *cache.Options) error {
		o.Resync = dur
		return nil
	}))
	return c
}

// SetGlobalSelector sets the default selector for the cache.
func (c *CacheBuilder) SetGlobalSelector(selector cache.ObjectSelector) *CacheBuilder {
	c.opts = append(c.opts, cache.SetOptionsFunc(func(o *cache.Options) error {
		o.DefaultSelector = selector
		return nil
	}))
	return c
}

// SetGlobalTransform sets the default transform for the cache.
func (c *CacheBuilder) SetGlobalTransform(transform toolscache.TransformFunc) *CacheBuilder {
	c.opts = append(c.opts, cache.SetOptionsFunc(func(o *cache.Options) error {
		o.DefaultTransform = transform
		return nil
	}))
	return c
}

// UnsafeDisableDeepCopyFor disables deep copy for the given objects.
// THIS IS UNSAFE in general, and should only be used for performance reasons.
// It is only safe to use this if you know that the objects are not mutated
// after being read from the cache.
func (c *CacheBuilder) UnsafeDisableDeepCopyFor(objs ...client.Object) *CacheBuilder {
	c.opts = append(c.opts, cache.SetOptionsFunc(func(o *cache.Options) error {
		if o.UnsafeDisableDeepCopyByObject == nil {
			o.UnsafeDisableDeepCopyByObject = make(map[client.Object]bool)
		}
		for _, obj := range objs {
			o.UnsafeDisableDeepCopyByObject[obj] = true
		}
		return nil
	}))
	return c
}

// Transform sets the transform function for the given objects.
func (c *CacheBuilder) Transform(obj client.Object, transform toolscache.TransformFunc) *CacheBuilder {
	c.opts = append(c.opts, cache.SetOptionsFunc(func(o *cache.Options) error {
		if o.TransformByObject == nil {
			o.TransformByObject = make(map[client.Object]toolscache.TransformFunc)
		}
		o.TransformByObject[obj] = transform
		return nil
	}))
	return c
}

// Select sets the selector for the given objects.
func (c *CacheBuilder) Select(obj client.Object, selector cache.ObjectSelector) *CacheBuilder {
	c.opts = append(c.opts, cache.SetOptionsFunc(func(o *cache.Options) error {
		if o.SelectorsByObject == nil {
			o.SelectorsByObject = make(map[client.Object]cache.ObjectSelector)
		}
		o.SelectorsByObject[obj] = selector
		return nil
	}))
	return c
}

// RestrictedView allows to build restricted cache views.
//
// A restricted cache view is a cache that is restricted to only specific namespaces
// and/or specific objects. This is useful for controllers that only need to watch
// a subset of the cluster.
func (c *CacheBuilder) RestrictedView() *RestrictedViewCacheBuilder {
	return &RestrictedViewCacheBuilder{
		global: c,
	}
}

// RestrictedViewCacheBuilder is a builder for restricted cache views.
type RestrictedViewCacheBuilder struct {
	global     *CacheBuilder
	namespaces []string
}

// With sets the namespace to restrict the cache to.
// This can be called multiple times to restrict the cache to multiple namespaces.
//
// TODO(vincepri): The restricted view should be able to set its own transformers, selectors,
// and other options on a namespace-by-namespace basis. Under the hood, the MultiNamespacedCacheBuilder
// is building a cache for each namespace, and we should be able to set options on each of them.
// This will require a change to the exported functions.
func (r *RestrictedViewCacheBuilder) With(namespace string) *RestrictedViewCacheBuilder {
	r.namespaces = append(r.namespaces, namespace)
	return r
}

// Factory builds a cache.NewCacheFunc from the RestrictedCacheBuilder.
func (r *RestrictedViewCacheBuilder) Factory() cache.NewCacheFunc {
	return func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
		for _, opt := range r.global.opts {
			if err := opt.ApplyToCache(&opts); err != nil {
				return nil, err
			}
		}
		if opts.Namespace != "" {
			return nil, errors.New("cache.Options.Namespace cannot be set for the cache's RestrictedView")
		}

		// Build the set of namespaces to limit the watch to.
		ns := sets.New[string]()
		ns.Insert(r.namespaces...)
		ns.Delete("")
		if ns.Len() == 0 {
			return nil, errors.New("no namespaces specified for the cache's RestrictedView")
		}

		if ns.Len() == 1 {
			// If there's only one namespace, use the regular cache builder.
			opts.Namespace = ns.UnsortedList()[0]
			return r.global.Factory()(config, opts)
		}

		// We're using the deprecated function here because we're building a restricted view of the cache.
		// And we need to retain backwards compatibility with the older exported function. This will be removed
		// in favor of a new function that allows to set options on each namespace.
		return cache.MultiNamespacedCacheBuilder(ns.UnsortedList())(config, opts) //nolint:staticcheck
	}
}

// Build builds a cache.Cache from the RestrictedCacheBuilder.
func (r *RestrictedViewCacheBuilder) Build(config *rest.Config) (cache.Cache, error) {
	return r.Factory()(config, cache.Options{})
}
