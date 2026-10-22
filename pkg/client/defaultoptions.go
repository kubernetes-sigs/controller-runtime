/*
Copyright The Kubernetes Authors.

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
	"slices"

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
//
// If c is already a *clientWithOptions, its options are extended in place
// rather than wrapping it again. The new defaults are applied after the
// existing ones (so the existing defaults still lose to the caller), and the
// new forced options are applied before the existing ones (so the existing
// forced options still win), preserving the same precedence as nested wrapping.
func withInjectedOptions(c Client, defaults, forced writeOptions) Client {
	if existing, ok := c.(*clientWithOptions); ok {
		return &clientWithOptions{
			Reader:   existing.Reader,
			client:   existing.client,
			defaults: concatWriteOptions(existing.defaults, defaults),
			forced:   concatWriteOptions(forced, existing.forced),
		}
	}
	return &clientWithOptions{
		Reader:   c,
		client:   c,
		defaults: defaults,
		forced:   forced,
	}
}

// concatWriteOptions returns a writeOptions with each verb's options from a
// followed by b. It allocates fresh slices so neither input is mutated.
func concatWriteOptions(a, b writeOptions) writeOptions {
	return writeOptions{
		create:      slices.Concat(a.create, b.create),
		update:      slices.Concat(a.update, b.update),
		patch:       slices.Concat(a.patch, b.patch),
		apply:       slices.Concat(a.apply, b.apply),
		delete:      slices.Concat(a.delete, b.delete),
		deleteAllOf: slices.Concat(a.deleteAllOf, b.deleteAllOf),

		subResourceCreate: slices.Concat(a.subResourceCreate, b.subResourceCreate),
		subResourceUpdate: slices.Concat(a.subResourceUpdate, b.subResourceUpdate),
		subResourcePatch:  slices.Concat(a.subResourcePatch, b.subResourcePatch),
		subResourceApply:  slices.Concat(a.subResourceApply, b.subResourceApply),
	}
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
	return c.client.Create(ctx, obj, slices.Concat(c.defaults.create, opts, c.forced.create)...)
}

func (c *clientWithOptions) Update(ctx context.Context, obj Object, opts ...UpdateOption) error {
	return c.client.Update(ctx, obj, slices.Concat(c.defaults.update, opts, c.forced.update)...)
}

func (c *clientWithOptions) Patch(ctx context.Context, obj Object, patch Patch, opts ...PatchOption) error {
	return c.client.Patch(ctx, obj, patch, slices.Concat(c.defaults.patch, opts, c.forced.patch)...)
}

func (c *clientWithOptions) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...ApplyOption) error {
	return c.client.Apply(ctx, obj, slices.Concat(c.defaults.apply, opts, c.forced.apply)...)
}

func (c *clientWithOptions) Delete(ctx context.Context, obj Object, opts ...DeleteOption) error {
	return c.client.Delete(ctx, obj, slices.Concat(c.defaults.delete, opts, c.forced.delete)...)
}

func (c *clientWithOptions) DeleteAllOf(ctx context.Context, obj Object, opts ...DeleteAllOfOption) error {
	return c.client.DeleteAllOf(ctx, obj, slices.Concat(c.defaults.deleteAllOf, opts, c.forced.deleteAllOf)...)
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
	return c.writer.Create(ctx, obj, subResource, slices.Concat(c.defaults.subResourceCreate, opts, c.forced.subResourceCreate)...)
}

func (c *subResourceClientWithOptions) Update(ctx context.Context, obj Object, opts ...SubResourceUpdateOption) error {
	return c.writer.Update(ctx, obj, slices.Concat(c.defaults.subResourceUpdate, opts, c.forced.subResourceUpdate)...)
}

func (c *subResourceClientWithOptions) Patch(ctx context.Context, obj Object, patch Patch, opts ...SubResourcePatchOption) error {
	return c.writer.Patch(ctx, obj, patch, slices.Concat(c.defaults.subResourcePatch, opts, c.forced.subResourcePatch)...)
}

func (c *subResourceClientWithOptions) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...SubResourceApplyOption) error {
	return c.writer.Apply(ctx, obj, slices.Concat(c.defaults.subResourceApply, opts, c.forced.subResourceApply)...)
}
