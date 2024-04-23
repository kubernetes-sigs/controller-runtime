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
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	toolscache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ClusterNameIndex = "cluster/name"
	ClusterIndex     = "cluster"
)

var _ cache.Cache = &NamespacedCache{}

type NamespacedCache struct {
	clusterName string
	cache.Cache
}

func (c *NamespacedCache) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if key.Namespace != "default" {
		return apierrors.NewNotFound(schema.GroupResource{}, key.Name)
	}
	key.Namespace = c.clusterName
	if err := c.Cache.Get(ctx, key, obj, opts...); err != nil {
		return err
	}
	obj.SetNamespace("default")
	return nil
}

func (c *NamespacedCache) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	var listOpts client.ListOptions
	for _, o := range opts {
		o.ApplyToList(&listOpts)
	}

	switch {
	case listOpts.FieldSelector != nil:
		reqs := listOpts.FieldSelector.Requirements()
		flds := make(map[string]string, len(reqs))
		for i := range reqs {
			flds[fmt.Sprintf("cluster/%s", reqs[i].Field)] = fmt.Sprintf("%s/%s", c.clusterName, reqs[i].Value)
		}
		opts = append(opts, client.MatchingFields(flds))
	case listOpts.Namespace != "":
		opts = append(opts, client.MatchingFields(map[string]string{ClusterIndex: c.clusterName}))
		if c.clusterName == "*" {
			listOpts.Namespace = ""
		} else if listOpts.Namespace == "default" {
			listOpts.Namespace = c.clusterName
		}
	default:
		opts = append(opts, client.MatchingFields(map[string]string{ClusterIndex: c.clusterName}))
	}

	if err := c.Cache.List(ctx, list, opts...); err != nil {
		return err
	}

	return meta.EachListItem(list, func(obj runtime.Object) error {
		obj.(client.Object).SetNamespace("default")
		return nil
	})
}

func (c *NamespacedCache) GetInformer(ctx context.Context, obj client.Object, opts ...cache.InformerGetOption) (cache.Informer, error) {
	inf, err := c.Cache.GetInformer(ctx, obj, opts...)
	if err != nil {
		return nil, err
	}
	return &ScopedInformer{clusterName: c.clusterName, Informer: inf}, nil
}

func (c *NamespacedCache) GetInformerForKind(ctx context.Context, gvk schema.GroupVersionKind, opts ...cache.InformerGetOption) (cache.Informer, error) {
	inf, err := c.Cache.GetInformerForKind(ctx, gvk, opts...)
	if err != nil {
		return nil, err
	}
	return &ScopedInformer{clusterName: c.clusterName, Informer: inf}, nil
}

func (c *NamespacedCache) RemoveInformer(ctx context.Context, obj client.Object) error {
	return errors.New("informer cannot be removed from scoped cache")
}

type ScopedInformer struct {
	clusterName string
	cache.Informer
}

func (i *ScopedInformer) AddEventHandler(handler toolscache.ResourceEventHandler) (toolscache.ResourceEventHandlerRegistration, error) {
	return i.Informer.AddEventHandler(toolscache.ResourceEventHandlerDetailedFuncs{
		AddFunc: func(obj interface{}, isInInitialList bool) {
			cobj := obj.(client.Object)
			if cobj.GetNamespace() == i.clusterName {
				cobj := cobj.DeepCopyObject().(client.Object)
				cobj.SetNamespace("default")
				handler.OnAdd(cobj, isInInitialList)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			cobj := newObj.(client.Object)
			if cobj.GetNamespace() == i.clusterName {
				cobj := cobj.DeepCopyObject().(client.Object)
				cobj.SetNamespace("default")
				handler.OnUpdate(oldObj, cobj)
			}
		},
		DeleteFunc: func(obj interface{}) {
			if tombStone, ok := obj.(toolscache.DeletedFinalStateUnknown); ok {
				obj = tombStone.Obj
			}
			cobj := obj.(client.Object)
			if cobj.GetNamespace() == i.clusterName {
				cobj := cobj.DeepCopyObject().(client.Object)
				cobj.SetNamespace("default")
				handler.OnDelete(cobj)
			}
		},
	})
}

func (i *ScopedInformer) AddEventHandlerWithResyncPeriod(handler toolscache.ResourceEventHandler, resyncPeriod time.Duration) (toolscache.ResourceEventHandlerRegistration, error) {
	return i.Informer.AddEventHandlerWithResyncPeriod(toolscache.ResourceEventHandlerDetailedFuncs{
		AddFunc: func(obj interface{}, isInInitialList bool) {
			cobj := obj.(client.Object)
			if cobj.GetNamespace() == i.clusterName {
				cobj := cobj.DeepCopyObject().(client.Object)
				cobj.SetNamespace("default")
				handler.OnAdd(cobj, isInInitialList)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			cobj := newObj.(client.Object)
			if cobj.GetNamespace() == i.clusterName {
				cobj := cobj.DeepCopyObject().(client.Object)
				cobj.SetNamespace("default")
				handler.OnUpdate(oldObj, cobj)
			}
		},
		DeleteFunc: func(obj interface{}) {
			if tombStone, ok := obj.(toolscache.DeletedFinalStateUnknown); ok {
				obj = tombStone.Obj
			}
			cobj := obj.(client.Object)
			if cobj.GetNamespace() == i.clusterName {
				cobj := cobj.DeepCopyObject().(client.Object)
				cobj.SetNamespace("default")
				handler.OnDelete(cobj)
			}
		},
	}, resyncPeriod)
}

func (i *ScopedInformer) AddIndexers(indexers toolscache.Indexers) error {
	return errors.New("indexes cannot be added to scoped informers")
}

type NamespaceScopeableCache struct {
	cache.Cache
}

func (f *NamespaceScopeableCache) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	return f.IndexField(ctx, obj, "cluster/"+field, func(obj client.Object) []string {
		keys := extractValue(obj)
		withCluster := make([]string, len(keys)*2)
		for i, key := range keys {
			withCluster[i] = fmt.Sprintf("%s/%s", obj.GetNamespace(), key)
			withCluster[i+len(keys)] = fmt.Sprintf("*/%s", key)
		}
		return withCluster
	})
}

func (f *NamespaceScopeableCache) Start(ctx context.Context) error {
	return nil // no-op as this is shared
}
