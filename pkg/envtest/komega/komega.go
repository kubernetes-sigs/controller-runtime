/*
Copyright 2021 The Kubernetes Authors.

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

package komega

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// komega is a collection of utilites for writing tests involving a mocked
// Kubernetes API.
type komega struct {
	ctx    context.Context
	client client.Client
}

var _ Komega = &komega{}

// New creates a new Komega instance with the given client.
func New(c client.Client) Komega {
	return &komega{
		client: c,
		ctx:    context.Background(),
	}
}

// WithContext returns a copy that uses the given context.
func (k komega) WithContext(ctx context.Context) Komega {
	k.ctx = ctx
	return &k
}

// Get returns a function that fetches a resource and returns the occurring error.
func (k *komega) Get(obj client.Object) func() error {
	key := types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
	return func() error {
		return k.client.Get(k.ctx, key, obj)
	}
}

// List returns a function that lists resources and returns the occurring error.
func (k *komega) List(obj client.ObjectList, opts ...client.ListOption) func() error {
	return func() error {
		return k.client.List(k.ctx, obj, opts...)
	}
}

// Update returns a function that fetches a resource, applies the provided update function and then updates the resource.
func (k *komega) Update(obj client.Object, updateFunc func(), opts ...client.UpdateOption) func() error {
	key := types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
	return func() error {
		err := k.client.Get(k.ctx, key, obj)
		if err != nil {
			return err
		}
		updateFunc()
		return k.client.Update(k.ctx, obj, opts...)
	}
}

// Delete returns a function that deletes a resource and returns the occurring error.
func (k *komega) Delete(obj client.Object, opts ...client.DeleteOption) func() error {
	return func() error {
		return k.client.Delete(k.ctx, obj, opts...)
	}
}

// Patch returns a function that applies the provided patch on the resource and returns the occurring error.
func (k *komega) Patch(obj client.Object, patch client.Patch, opts ...client.PatchOption) func() error {
	return func() error {
		return k.client.Patch(k.ctx, obj, patch, opts...)
	}
}

// PatchStatus returns a function that applies the provided patch on the resource status and returns the occurring error.
func (k *komega) PatchStatus(obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) func() error {
	return func() error {
		return k.client.Status().Patch(k.ctx, obj, patch, opts...)
	}
}

// DeleteAllOf returns a function that deletes a list of resources and returns the occurring error.
func (k *komega) DeleteAllOf(obj client.Object, opts ...client.DeleteAllOfOption) func() error {
	return func() error {
		return k.client.DeleteAllOf(k.ctx, obj, opts...)
	}
}

// UpdateStatus returns a function that fetches a resource, applies the provided update function and then updates the resource's status.
func (k *komega) UpdateStatus(obj client.Object, updateFunc func(), opts ...client.SubResourceUpdateOption) func() error {
	key := types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
	return func() error {
		err := k.client.Get(k.ctx, key, obj)
		if err != nil {
			return err
		}
		updateFunc()
		return k.client.Status().Update(k.ctx, obj, opts...)
	}
}

// Object returns a function that fetches a resource and returns the object.
func (k *komega) Object(obj client.Object) func() (client.Object, error) {
	key := types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
	return func() (client.Object, error) {
		err := k.client.Get(k.ctx, key, obj)
		return obj, err
	}
}

// ObjectList returns a function that fetches a resource and returns the object.
func (k *komega) ObjectList(obj client.ObjectList, opts ...client.ListOption) func() (client.ObjectList, error) {
	return func() (client.ObjectList, error) {
		err := k.client.List(k.ctx, obj, opts...)
		return obj, err
	}
}
