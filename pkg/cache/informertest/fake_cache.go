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

package informertest

import (
	"context"
	"fmt"
	"strings"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	toolscache "k8s.io/client-go/tools/cache"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	cacheinternal "sigs.k8s.io/controller-runtime/pkg/cache/internal"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllertest"
)

var _ cache.Cache = &FakeInformers{}

// FakeInformers is a fake implementation of Informers.
type FakeInformers struct {
	InformersByGVK map[schema.GroupVersionKind]toolscache.SharedIndexInformer
	Scheme         *runtime.Scheme
	// RESTMapper optionally identifies whether an object is namespaced or cluster-scoped.
	RESTMapper apimeta.RESTMapper
	Error      error
	Synced     *bool
}

// GetInformerForKind implements Informers.
func (c *FakeInformers) GetInformerForKind(ctx context.Context, gvk schema.GroupVersionKind, opts ...cache.InformerGetOption) (cache.Informer, error) {
	if c.Scheme == nil {
		c.Scheme = scheme.Scheme
	}
	obj, err := c.Scheme.New(gvk)
	if err != nil {
		return nil, err
	}
	return c.informerFor(gvk, obj)
}

// FakeInformerForKind implements Informers.
func (c *FakeInformers) FakeInformerForKind(ctx context.Context, gvk schema.GroupVersionKind) (*controllertest.FakeInformer, error) {
	i, err := c.GetInformerForKind(ctx, gvk)
	if err != nil {
		return nil, err
	}
	return i.(*controllertest.FakeInformer), nil
}

// GetInformer implements Informers.
func (c *FakeInformers) GetInformer(ctx context.Context, obj client.Object, opts ...cache.InformerGetOption) (cache.Informer, error) {
	if c.Scheme == nil {
		c.Scheme = scheme.Scheme
	}
	gvks, _, err := c.Scheme.ObjectKinds(obj)
	if err != nil {
		return nil, err
	}
	gvk := gvks[0]
	return c.informerFor(gvk, obj)
}

// RemoveInformer implements Informers.
func (c *FakeInformers) RemoveInformer(ctx context.Context, obj client.Object) error {
	if c.Scheme == nil {
		c.Scheme = scheme.Scheme
	}
	gvks, _, err := c.Scheme.ObjectKinds(obj)
	if err != nil {
		return err
	}
	gvk := gvks[0]
	delete(c.InformersByGVK, gvk)
	return nil
}

// WaitForCacheSync implements Informers.
func (c *FakeInformers) WaitForCacheSync(ctx context.Context) bool {
	if c.Synced == nil {
		return true
	}
	return *c.Synced
}

// FakeInformerFor implements Informers.
func (c *FakeInformers) FakeInformerFor(ctx context.Context, obj client.Object) (*controllertest.FakeInformer, error) {
	i, err := c.GetInformer(ctx, obj)
	if err != nil {
		return nil, err
	}
	return i.(*controllertest.FakeInformer), nil
}

func (c *FakeInformers) informerFor(gvk schema.GroupVersionKind, _ runtime.Object) (toolscache.SharedIndexInformer, error) {
	if c.Error != nil {
		return nil, c.Error
	}
	if c.InformersByGVK == nil {
		c.InformersByGVK = map[schema.GroupVersionKind]toolscache.SharedIndexInformer{}
	}
	informer, ok := c.InformersByGVK[gvk]
	if ok {
		return informer, nil
	}

	// Set Synced to true by default so that WaitForCacheSync returns immediately
	c.InformersByGVK[gvk] = controllertest.NewFakeInformer(controllertest.Synced)
	return c.InformersByGVK[gvk], nil
}

// Start implements Informers.
func (c *FakeInformers) Start(ctx context.Context) error {
	return c.Error
}

// IndexField implements Cache by adding a field index to the fake informer's store.
func (c *FakeInformers) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	informer, err := c.GetInformer(ctx, obj, cache.BlockUntilSynced(false))
	if err != nil {
		return err
	}

	indexFunc := func(objRaw any) ([]string, error) {
		indexedObj, ok := objRaw.(client.Object)
		if !ok {
			return nil, fmt.Errorf("object of type %T is not an Object", objRaw)
		}
		objMeta, err := apimeta.Accessor(indexedObj)
		if err != nil {
			return nil, err
		}

		namespace := objMeta.GetNamespace()
		rawValues := extractValue(indexedObj)
		values := make([]string, len(rawValues))
		if namespace != "" {
			values = make([]string, len(rawValues)*2)
		}
		for i, value := range rawValues {
			values[i] = cacheinternal.KeyToNamespacedKey(namespace, value)
			if namespace != "" {
				values[i+len(rawValues)] = cacheinternal.KeyToNamespacedKey("", value)
			}
		}

		return values, nil
	}

	return informer.AddIndexers(toolscache.Indexers{cacheinternal.FieldIndexName(field): indexFunc})
}

// Get implements Cache by reading from the fake informer's store.
func (c *FakeInformers) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	gvk, err := c.gvkForObject(obj)
	if err != nil {
		return err
	}
	indexer, err := c.indexerFor(ctx, obj)
	if err != nil {
		return err
	}
	scopeName, err := c.scopeNameFor(obj, indexer)
	if err != nil {
		return err
	}
	return cacheinternal.NewCacheReader(
		indexer, gvk, scopeName,
	).Get(ctx, key, obj, opts...)
}

// List implements Cache by reading from the fake informer's store.
func (c *FakeInformers) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	gvk, obj, err := c.gvkForList(list)
	if err != nil {
		return err
	}
	indexer, err := c.indexerFor(ctx, obj)
	if err != nil {
		return err
	}
	scopeName, err := c.scopeNameFor(obj, indexer)
	if err != nil {
		return err
	}
	return cacheinternal.NewCacheReader(
		indexer, gvk, scopeName,
	).List(ctx, list, opts...)
}

func (c *FakeInformers) indexerFor(ctx context.Context, obj client.Object) (toolscache.Indexer, error) {
	informer, err := c.GetInformer(ctx, obj, cache.BlockUntilSynced(false))
	if err != nil {
		return nil, err
	}
	indexerProvider, ok := informer.(interface{ GetIndexer() toolscache.Indexer })
	if !ok {
		return nil, fmt.Errorf("informer for %T does not expose an indexer", obj)
	}
	return indexerProvider.GetIndexer(), nil
}

func (c *FakeInformers) scopeNameFor(obj client.Object, indexer toolscache.Indexer) (apimeta.RESTScopeName, error) {
	if c.RESTMapper != nil {
		namespaced, err := apiutil.IsObjectNamespaced(obj, c.Scheme, c.RESTMapper)
		if err != nil {
			return "", err
		}
		if !namespaced {
			return apimeta.RESTScopeNameRoot, nil
		}
		return apimeta.RESTScopeNameNamespace, nil
	}

	// Without a RESTMapper, infer the scope from the objects already in the store.
	for _, item := range indexer.List() {
		obj, ok := item.(client.Object)
		if !ok {
			return "", fmt.Errorf("indexer contains %T, which is not an Object", item)
		}
		if obj.GetNamespace() != "" {
			return apimeta.RESTScopeNameNamespace, nil
		}
	}
	return apimeta.RESTScopeNameRoot, nil
}

func (c *FakeInformers) gvkForObject(obj client.Object) (schema.GroupVersionKind, error) {
	if c.Scheme == nil {
		c.Scheme = scheme.Scheme
	}
	gvks, _, err := c.Scheme.ObjectKinds(obj)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	if len(gvks) == 0 {
		return schema.GroupVersionKind{}, fmt.Errorf("no GVK found for %T", obj)
	}
	return gvks[0], nil
}

func (c *FakeInformers) gvkForList(list client.ObjectList) (schema.GroupVersionKind, client.Object, error) {
	if c.Scheme == nil {
		c.Scheme = scheme.Scheme
	}
	gvks, _, err := c.Scheme.ObjectKinds(list)
	if err != nil {
		return schema.GroupVersionKind{}, nil, err
	}
	if len(gvks) == 0 {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("no GVK found for %T", list)
	}
	gvk := gvks[0]
	gvk.Kind = strings.TrimSuffix(gvk.Kind, "List")
	obj, err := c.Scheme.New(gvk)
	if err != nil {
		return schema.GroupVersionKind{}, nil, err
	}
	clientObj, ok := obj.(client.Object)
	if !ok {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("%T is not a client.Object", obj)
	}
	return gvk, clientObj, nil
}
