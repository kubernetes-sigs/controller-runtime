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
	"sync"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	toolscache "k8s.io/client-go/tools/cache"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllertest"
)

var _ cache.Cache = &FakeInformers{}

// FakeInformers uses a fake client for reads and field indexes.
// Write through Client to change stored objects. Fake informer methods only send
// events; they do not update Client. Index functions run during List.
// Set the fields before use.
type FakeInformers struct {
	InformersByGVK map[schema.GroupVersionKind]toolscache.SharedIndexInformer
	Scheme         *runtime.Scheme
	// Client must come from fake.NewClientBuilder; interceptors are supported.
	// If nil, first use creates an empty fake client with Scheme and RESTMapper.
	// Supply a client to seed or change stored objects.
	Client client.Client
	// RESTMapper lets Get ignore key.Namespace for cluster-scoped objects.
	// If nil, Get uses the key unchanged.
	RESTMapper apimeta.RESTMapper
	Error      error
	Synced     *bool

	clientOnce sync.Once
	backend    client.Client
}

// GetInformerForKind implements Informers.
func (c *FakeInformers) GetInformerForKind(ctx context.Context, gvk schema.GroupVersionKind, opts ...cache.InformerGetOption) (cache.Informer, error) {
	if _, err := c.objectScheme().New(gvk); err != nil {
		return nil, err
	}
	return c.informerFor(gvk)
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
	gvk, err := apiutil.GVKForObject(obj, c.objectScheme())
	if err != nil {
		return nil, err
	}
	return c.informerFor(gvk)
}

// RemoveInformer implements Informers.
func (c *FakeInformers) RemoveInformer(ctx context.Context, obj client.Object) error {
	gvk, err := apiutil.GVKForObject(obj, c.objectScheme())
	if err != nil {
		return err
	}
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

func (c *FakeInformers) informerFor(gvk schema.GroupVersionKind) (toolscache.SharedIndexInformer, error) {
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

	// Fake informers start synced.
	c.InformersByGVK[gvk] = controllertest.NewFakeInformer(controllertest.Synced)
	return c.InformersByGVK[gvk], nil
}

// Start implements Informers.
func (c *FakeInformers) Start(ctx context.Context) error {
	return c.Error
}

// IndexField registers an index on the backing fake client.
func (c *FakeInformers) IndexField(_ context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	if c.Error != nil {
		return c.Error
	}
	return fake.AddIndex(c.backingClient(), obj, field, extractValue)
}

// Get delegates to the backing fake client.
func (c *FakeInformers) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if c.Error != nil {
		return c.Error
	}
	if c.RESTMapper != nil {
		namespaced, err := apiutil.IsObjectNamespaced(obj, c.objectScheme(), c.RESTMapper)
		if err != nil {
			return err
		}
		if !namespaced {
			key.Namespace = ""
		}
	}
	return c.backingClient().Get(ctx, key, obj, opts...)
}

// List delegates to the backing fake client.
func (c *FakeInformers) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if c.Error != nil {
		return c.Error
	}
	return c.backingClient().List(ctx, list, opts...)
}

func (c *FakeInformers) backingClient() client.Client {
	if c.Client != nil {
		return c.Client
	}
	c.clientOnce.Do(func() {
		c.backend = fake.NewClientBuilder().WithScheme(c.objectScheme()).WithRESTMapper(c.RESTMapper).Build()
	})
	return c.backend
}

func (c *FakeInformers) objectScheme() *runtime.Scheme {
	if c.Scheme != nil {
		return c.Scheme
	}
	if c.Client != nil {
		return c.Client.Scheme()
	}
	return scheme.Scheme
}
