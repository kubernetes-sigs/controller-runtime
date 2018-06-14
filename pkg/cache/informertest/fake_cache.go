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

	"github.com/kubernetes-sigs/controller-runtime/pkg/cache"
	"github.com/kubernetes-sigs/controller-runtime/pkg/client"
	"github.com/kubernetes-sigs/controller-runtime/pkg/controller/controllertest"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	toolscache "k8s.io/client-go/tools/cache"
)

var _ cache.Cache = &FakeInformers{}

// FakeInformers is a fake implementation of Informers
type FakeInformers struct {
	InformersByGVK map[schema.GroupVersionKind]toolscache.SharedIndexInformer
	Scheme         *runtime.Scheme
	Error          error
	Synced         *bool
}

// GetInformerForKind implements Informers
func (c *FakeInformers) GetInformerForKind(gvk schema.GroupVersionKind) (toolscache.SharedIndexInformer, error) {
	if c.Scheme == nil {
		c.Scheme = scheme.Scheme
	}
	obj, _ := c.Scheme.New(gvk)
	return c.informerFor(gvk, obj)
}

// FakeInformerForKind implements Informers
func (c *FakeInformers) FakeInformerForKind(gvk schema.GroupVersionKind) (*controllertest.FakeInformer, error) {
	if c.Scheme == nil {
		c.Scheme = scheme.Scheme
	}
	obj, _ := c.Scheme.New(gvk)
	i, err := c.informerFor(gvk, obj)
	if err != nil {
		return nil, err
	}
	return i.(*controllertest.FakeInformer), nil
}

// GetInformer implements Informers
func (c *FakeInformers) GetInformer(obj runtime.Object) (toolscache.SharedIndexInformer, error) {
	if c.Scheme == nil {
		c.Scheme = scheme.Scheme
	}
	gvks, _, _ := c.Scheme.ObjectKinds(obj)
	gvk := gvks[0]
	return c.informerFor(gvk, obj)
}

// WaitForCacheSync implements Informers
func (c *FakeInformers) WaitForCacheSync(stop <-chan struct{}) bool {
	if c.Synced == nil {
		return true
	}
	return *c.Synced
}

// FakeInformerFor implements Informers
func (c *FakeInformers) FakeInformerFor(obj runtime.Object) (*controllertest.FakeInformer, error) {
	if c.Scheme == nil {
		c.Scheme = scheme.Scheme
	}
	gvks, _, _ := c.Scheme.ObjectKinds(obj)
	gvk := gvks[0]
	i, err := c.informerFor(gvk, obj)
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

	c.InformersByGVK[gvk] = &controllertest.FakeInformer{}
	return c.InformersByGVK[gvk], nil
}

// Start implements Informers
func (c *FakeInformers) Start(stopCh <-chan struct{}) error {
	return c.Error
}

// IndexField implements Cache
func (c *FakeInformers) IndexField(obj runtime.Object, field string, extractValue client.IndexerFunc) error {
	return nil
}

// Get implements Cache
func (c *FakeInformers) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	return nil
}

// List implements Cache
func (c *FakeInformers) List(ctx context.Context, opts *client.ListOptions, list runtime.Object) error {
	return nil
}
