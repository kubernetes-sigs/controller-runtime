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

package test

import (
	"github.com/kubernetes-sigs/kubebuilder/pkg/controller/test"
	"github.com/kubernetes-sigs/kubebuilder/pkg/informer"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
)

var _ informer.IndexInformerCache = &FakeIndexCache{}

type FakeIndexCache struct {
	InformersByGVK map[schema.GroupVersionKind]cache.SharedIndexInformer
	Scheme         *runtime.Scheme
	Error          error
}

func (c *FakeIndexCache) InformerForKind(gvk schema.GroupVersionKind) (cache.SharedIndexInformer, error) {
	if c.Scheme == nil {
		c.Scheme = scheme.Scheme
	}
	obj, _ := c.Scheme.New(gvk)
	return c.informerFor(gvk, obj)
}

func (c *FakeIndexCache) FakeInformerForKind(gvk schema.GroupVersionKind) (*test.FakeInformer, error) {
	if c.Scheme == nil {
		c.Scheme = scheme.Scheme
	}
	obj, _ := c.Scheme.New(gvk)
	i, err := c.informerFor(gvk, obj)
	if err != nil {
		return nil, err
	}
	return i.(*test.FakeInformer), nil
}

func (c *FakeIndexCache) InformerFor(obj runtime.Object) (cache.SharedIndexInformer, error) {
	if c.Scheme == nil {
		c.Scheme = scheme.Scheme
	}
	gvks, _, _ := c.Scheme.ObjectKinds(obj)
	gvk := gvks[0]
	return c.informerFor(gvk, obj)
}

func (c *FakeIndexCache) FakeInformerFor(obj runtime.Object) (*test.FakeInformer, error) {
	if c.Scheme == nil {
		c.Scheme = scheme.Scheme
	}
	gvks, _, _ := c.Scheme.ObjectKinds(obj)
	gvk := gvks[0]
	i, err := c.informerFor(gvk, obj)
	if err != nil {
		return nil, err
	}
	return i.(*test.FakeInformer), nil
}

func (c *FakeIndexCache) informerFor(gvk schema.GroupVersionKind, obj runtime.Object) (cache.SharedIndexInformer, error) {
	if c.Error != nil {
		return nil, c.Error
	}
	if c.InformersByGVK == nil {
		c.InformersByGVK = map[schema.GroupVersionKind]cache.SharedIndexInformer{}
	}
	informer, ok := c.InformersByGVK[gvk]
	if ok {
		return informer, nil
	}

	c.InformersByGVK[gvk] = &test.FakeInformer{}
	return c.InformersByGVK[gvk], nil
}

func (c *FakeIndexCache) Start(stopCh <-chan struct{}) error {
	return c.Error
}
