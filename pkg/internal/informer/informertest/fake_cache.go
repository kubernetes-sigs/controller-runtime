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
	"github.com/kubernetes-sigs/controller-runtime/pkg/controller/controllertest"
	"github.com/kubernetes-sigs/controller-runtime/pkg/internal/informer"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
)

var _ informer.Informers = &FakeInformers{}

// FakeInformers is a fake implementation of Informers
type FakeInformers struct {
	InformersByGVK map[schema.GroupVersionKind]cache.SharedIndexInformer
	Scheme         *runtime.Scheme
	Error          error
}

// InformerForKind implements Informers
func (c *FakeInformers) InformerForKind(gvk schema.GroupVersionKind) (cache.SharedIndexInformer, error) {
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

// InformerFor implements Informers
func (c *FakeInformers) InformerFor(obj runtime.Object) (cache.SharedIndexInformer, error) {
	if c.Scheme == nil {
		c.Scheme = scheme.Scheme
	}
	gvks, _, _ := c.Scheme.ObjectKinds(obj)
	gvk := gvks[0]
	return c.informerFor(gvk, obj)
}

// KnownInformersByType implements Informers
func (c *FakeInformers) KnownInformersByType() map[schema.GroupVersionKind]cache.SharedIndexInformer {
	return c.InformersByGVK
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

func (c *FakeInformers) informerFor(gvk schema.GroupVersionKind, obj runtime.Object) (cache.SharedIndexInformer, error) {
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

	c.InformersByGVK[gvk] = &controllertest.FakeInformer{}
	return c.InformersByGVK[gvk], nil
}

// Start implements Informers
func (c *FakeInformers) Start(stopCh <-chan struct{}) error {
	return c.Error
}
