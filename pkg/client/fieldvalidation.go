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

package client

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// WithStrictFieldValidation wraps a Client and configures strict field
// validation, by default, for all write requests from this client. Users
// can override the field validation for individual requests.
func WithStrictFieldValidation(c Client) Client {
	return &clientWithFieldValidation{
		validation: metav1.FieldValidationStrict,
		c:          c,
		Reader:     c,
	}
}

type clientWithFieldValidation struct {
	validation string
	c          Client
	Reader
}

func (f *clientWithFieldValidation) Create(ctx context.Context, obj Object, opts ...CreateOption) error {
	return f.c.Create(ctx, obj, append([]CreateOption{FieldValidation(f.validation)}, opts...)...)
}

func (f *clientWithFieldValidation) Update(ctx context.Context, obj Object, opts ...UpdateOption) error {
	return f.c.Update(ctx, obj, append([]UpdateOption{FieldValidation(f.validation)}, opts...)...)
}

func (f *clientWithFieldValidation) Patch(ctx context.Context, obj Object, patch Patch, opts ...PatchOption) error {
	return f.c.Patch(ctx, obj, patch, append([]PatchOption{FieldValidation(f.validation)}, opts...)...)
}

func (f *clientWithFieldValidation) Delete(ctx context.Context, obj Object, opts ...DeleteOption) error {
	return f.c.Delete(ctx, obj, opts...)
}

func (f *clientWithFieldValidation) DeleteAllOf(ctx context.Context, obj Object, opts ...DeleteAllOfOption) error {
	return f.c.DeleteAllOf(ctx, obj, opts...)
}

func (f *clientWithFieldValidation) Scheme() *runtime.Scheme     { return f.c.Scheme() }
func (f *clientWithFieldValidation) RESTMapper() meta.RESTMapper { return f.c.RESTMapper() }
func (f *clientWithFieldValidation) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return f.c.GroupVersionKindFor(obj)
}

func (f *clientWithFieldValidation) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return f.c.IsObjectNamespaced(obj)
}

func (f *clientWithFieldValidation) Status() StatusWriter {
	return &subresourceClientWithFieldValidation{
		validation:        f.validation,
		subresourceWriter: f.c.Status(),
	}
}

func (f *clientWithFieldValidation) SubResource(subresource string) SubResourceClient {
	c := f.c.SubResource(subresource)
	return &subresourceClientWithFieldValidation{
		validation:        f.validation,
		subresourceWriter: c,
		SubResourceReader: c,
	}
}

type subresourceClientWithFieldValidation struct {
	validation        string
	subresourceWriter SubResourceWriter
	SubResourceReader
}

func (f *subresourceClientWithFieldValidation) Create(ctx context.Context, obj Object, subresource Object, opts ...SubResourceCreateOption) error {
	return f.subresourceWriter.Create(ctx, obj, subresource, append([]SubResourceCreateOption{FieldValidation(f.validation)}, opts...)...)
}

func (f *subresourceClientWithFieldValidation) Update(ctx context.Context, obj Object, opts ...SubResourceUpdateOption) error {
	return f.subresourceWriter.Update(ctx, obj, append([]SubResourceUpdateOption{FieldValidation(f.validation)}, opts...)...)
}

func (f *subresourceClientWithFieldValidation) Patch(ctx context.Context, obj Object, patch Patch, opts ...SubResourcePatchOption) error {
	return f.subresourceWriter.Patch(ctx, obj, patch, append([]SubResourcePatchOption{FieldValidation(f.validation)}, opts...)...)
}
