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
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

type GVKCachesOptions struct {
	ObjectCacheFuncs map[client.Object]NewCacheFunc
	DefaultCacheFunc NewCacheFunc
}

// GVKCachesBuilder - Builder function to create a new composite cache which contains
// one or more caches based on a mapping of GVKs. Each GVK can be configured to
// use a separate cache.
func GVKCachesBuilder(gvkCachesOpts GVKCachesOptions) NewCacheFunc {
	return func(config *rest.Config, opts Options) (Cache, error) {
		opts, err := defaultOpts(config, opts)
		if err != nil {
			return nil, err
		}

		if gvkCachesOpts.DefaultCacheFunc == nil {
			gvkCachesOpts.DefaultCacheFunc = New
		}
		defaultCache, err := gvkCachesOpts.DefaultCacheFunc(config, opts)
		if err != nil {
			return nil, err
		}

		gvkToCache := map[schema.GroupVersionKind]Cache{}
		for obj, newCacheFunc := range gvkCachesOpts.ObjectCacheFuncs {
			gvk, err := apiutil.GVKForObject(obj, opts.Scheme)
			if err != nil {
				return nil, err
			}
			gvkToCache[gvk], err = newCacheFunc(config, opts)
			if err != nil {
				return nil, err
			}
		}

		return &gvkCaches{
			gvkToCache:   gvkToCache,
			defaultCache: defaultCache,
			scheme:       opts.Scheme,
		}, nil
	}
}

type gvkCaches struct {
	gvkToCache   map[schema.GroupVersionKind]Cache
	defaultCache Cache

	scheme *runtime.Scheme
}

func (c *gvkCaches) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	gvk, err := apiutil.GVKForObject(obj, c.scheme)
	if err != nil {
		return err
	}

	if cache, ok := c.gvkToCache[gvk]; ok {
		return cache.Get(ctx, key, obj)
	}

	return c.defaultCache.Get(ctx, key, obj)
}

func (c *gvkCaches) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	gvk, err := apiutil.GVKForObject(list, c.scheme)
	if err != nil {
		return err
	}

	if !strings.HasSuffix(gvk.Kind, "List") {
		return fmt.Errorf("non-list type %T (kind %q) passed as output", list, gvk)
	}
	gvk.Kind = strings.TrimSuffix(gvk.Kind, "List")

	if cache, ok := c.gvkToCache[gvk]; ok {
		return cache.List(ctx, list, opts...)
	}
	return c.defaultCache.List(ctx, list, opts...)
}

func (c *gvkCaches) GetInformer(ctx context.Context, obj client.Object) (Informer, error) {
	gvk, err := apiutil.GVKForObject(obj, c.scheme)
	if err != nil {
		return nil, err
	}

	if cache, ok := c.gvkToCache[gvk]; ok {
		return cache.GetInformer(ctx, obj)
	}

	return c.defaultCache.GetInformer(ctx, obj)
}

func (c *gvkCaches) GetInformerForKind(ctx context.Context, gvk schema.GroupVersionKind) (Informer, error) {
	if cache, ok := c.gvkToCache[gvk]; ok {
		return cache.GetInformerForKind(ctx, gvk)
	}

	return c.defaultCache.GetInformerForKind(ctx, gvk)
}

func (c *gvkCaches) Start(ctx context.Context) error {
	for gvk, cache := range c.gvkToCache {
		go func(gvk schema.GroupVersionKind, cache Cache) {
			err := cache.Start(ctx)
			if err != nil {
				log.Error(err, "gvk cache failed to start", "gvk", gvk)
			}
		}(gvk, cache)
	}
	go func() {
		err := c.defaultCache.Start(ctx)
		if err != nil {
			log.Error(err, "default cache failed to start")
		}
	}()
	<-ctx.Done()
	return nil
}

func (c *gvkCaches) WaitForCacheSync(ctx context.Context) bool {
	synced := true
	for _, cache := range c.gvkToCache {
		if s := cache.WaitForCacheSync(ctx); !s {
			synced = synced && s
		}
	}
	return synced && c.defaultCache.WaitForCacheSync(ctx)
}

func (c *gvkCaches) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	gvk, err := apiutil.GVKForObject(obj, c.scheme)
	if err != nil {
		return err
	}

	if cache, ok := c.gvkToCache[gvk]; ok {
		return cache.IndexField(ctx, obj, field, extractValue)
	}

	return c.defaultCache.IndexField(ctx, obj, field, extractValue)
}
