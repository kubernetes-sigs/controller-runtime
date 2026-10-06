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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// writeOptions holds a set of options per write verb, including the subresource
// verbs.
type writeOptions struct {
	create      []CreateOption
	update      []UpdateOption
	patch       []PatchOption
	apply       []ApplyOption
	delete      []DeleteOption
	deleteAllOf []DeleteAllOfOption

	subResourceCreate []SubResourceCreateOption
	subResourceUpdate []SubResourceUpdateOption
	subResourcePatch  []SubResourcePatchOption
	subResourceApply  []SubResourceApplyOption
}

// withInjectedOptions wraps a Client and injects options into every matching
// write request. The defaults are applied before the per call options, so a per
// call option takes precedence over them. The forced options are applied after
// the per call options, so they win over anything the caller passes and cannot
// be overridden.
//
// This is the shared backend for the option injecting wrappers WithFieldOwner,
// WithFieldValidation and NewDryRunClient. It gives us a single place to wrap a
// Client with write options instead of a dedicated wrapper type per option.
func withInjectedOptions(c Client, defaults, forced writeOptions) Client {
	return &clientWithOptions{
		Reader:   c,
		client:   c,
		defaults: defaults,
		forced:   forced,
	}
}

// combine returns pre, mid and post concatenated in order. The per call options
// (mid) sit between the defaults (pre) and the forced options (post), so that
// for options setting the same field the defaults lose to the caller and the
// caller loses to the forced options.
func combine[T any](pre, mid, post []T) []T {
	if len(pre) == 0 && len(post) == 0 {
		return mid
	}
	out := make([]T, 0, len(pre)+len(mid)+len(post))
	out = append(out, pre...)
	out = append(out, mid...)
	out = append(out, post...)
	return out
}

var (
	_ Client            = &clientWithOptions{}
	_ SubResourceClient = &subResourceClientWithOptions{}
)

// clientWithOptions embeds only Reader for Get and List, and keeps the full
// client in a field. Every Writer method is implemented explicitly rather than
// promoted, so adding a new write verb to the Client interface is a compile
// error here until it is handled, instead of silently passing through without
// the configured options.
type clientWithOptions struct {
	Reader
	client   Client
	defaults writeOptions
	forced   writeOptions
}

func (c *clientWithOptions) Create(ctx context.Context, obj Object, opts ...CreateOption) error {
	return c.client.Create(ctx, obj, combine(c.defaults.create, opts, c.forced.create)...)
}

func (c *clientWithOptions) Update(ctx context.Context, obj Object, opts ...UpdateOption) error {
	return c.client.Update(ctx, obj, combine(c.defaults.update, opts, c.forced.update)...)
}

func (c *clientWithOptions) Patch(ctx context.Context, obj Object, patch Patch, opts ...PatchOption) error {
	return c.client.Patch(ctx, obj, patch, combine(c.defaults.patch, opts, c.forced.patch)...)
}

func (c *clientWithOptions) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...ApplyOption) error {
	return c.client.Apply(ctx, obj, combine(c.defaults.apply, opts, c.forced.apply)...)
}

func (c *clientWithOptions) Delete(ctx context.Context, obj Object, opts ...DeleteOption) error {
	return c.client.Delete(ctx, obj, combine(c.defaults.delete, opts, c.forced.delete)...)
}

func (c *clientWithOptions) DeleteAllOf(ctx context.Context, obj Object, opts ...DeleteAllOfOption) error {
	return c.client.DeleteAllOf(ctx, obj, combine(c.defaults.deleteAllOf, opts, c.forced.deleteAllOf)...)
}

func (c *clientWithOptions) Scheme() *runtime.Scheme     { return c.client.Scheme() }
func (c *clientWithOptions) RESTMapper() meta.RESTMapper { return c.client.RESTMapper() }
func (c *clientWithOptions) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return c.client.GroupVersionKindFor(obj)
}
func (c *clientWithOptions) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return c.client.IsObjectNamespaced(obj)
}

func (c *clientWithOptions) Status() StatusWriter {
	return c.SubResource("status")
}

func (c *clientWithOptions) SubResource(subResource string) SubResourceClient {
	sr := c.client.SubResource(subResource)
	return &subResourceClientWithOptions{
		SubResourceReader: sr,
		writer:            sr,
		defaults:          c.defaults,
		forced:            c.forced,
	}
}

// subResourceClientWithOptions embeds only SubResourceReader for Get and
// implements each writer method explicitly, for the same reason as above.
type subResourceClientWithOptions struct {
	SubResourceReader
	writer   SubResourceWriter
	defaults writeOptions
	forced   writeOptions
}

func (c *subResourceClientWithOptions) Create(ctx context.Context, obj Object, subResource Object, opts ...SubResourceCreateOption) error {
	return c.writer.Create(ctx, obj, subResource, combine(c.defaults.subResourceCreate, opts, c.forced.subResourceCreate)...)
}

func (c *subResourceClientWithOptions) Update(ctx context.Context, obj Object, opts ...SubResourceUpdateOption) error {
	return c.writer.Update(ctx, obj, combine(c.defaults.subResourceUpdate, opts, c.forced.subResourceUpdate)...)
}

func (c *subResourceClientWithOptions) Patch(ctx context.Context, obj Object, patch Patch, opts ...SubResourcePatchOption) error {
	return c.writer.Patch(ctx, obj, patch, combine(c.defaults.subResourcePatch, opts, c.forced.subResourcePatch)...)
}

func (c *subResourceClientWithOptions) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...SubResourceApplyOption) error {
	return c.writer.Apply(ctx, obj, combine(c.defaults.subResourceApply, opts, c.forced.subResourceApply)...)
}
