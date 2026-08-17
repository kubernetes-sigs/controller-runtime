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
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

type cache interface {
	SetMinimumRVForGVKAndKey(gvk schema.GroupVersionKind, key ObjectKey, rv int64)
	AddRequiredDeleteForObject(Object) error
	RemoveRequiredDeleteForObject(Object) error
}

type consistentClientUpstream interface {
	Client

	delete(ctx context.Context, obj Object, opts ...DeleteOption) (*unstructured.Unstructured, error)
}

type keyLock interface {
	Lock(ctx context.Context) error
	Unlock()
	Wait(ctx context.Context) error
}

var _ Client = (*consistentClient)(nil)

func newConsistentClient(upstream consistentClientUpstream, c cache, newKeyLock func() keyLock) *consistentClient {
	if newKeyLock == nil {
		newKeyLock = func() keyLock { return &keyLocker{} }
	}

	return &consistentClient{
		upstream: upstream,
		cache:    c,
		lockedKeysByGVK: newThreadSafeMap[schema.GroupVersionKind](func() *threadSafeMap[types.NamespacedName, keyLock] {
			return newThreadSafeMap[types.NamespacedName](newKeyLock)
		}),
	}
}

type consistentClient struct {
	upstream consistentClientUpstream
	cache    cache

	// lockedKeysByGVK maps gvk -> key -> keyLock
	lockedKeysByGVK *threadSafeMap[schema.GroupVersionKind, *threadSafeMap[types.NamespacedName, keyLock]]
}

func (c *consistentClient) Get(ctx context.Context, key ObjectKey, obj Object, opts ...GetOption) error {
	gvk, err := apiutil.GVKForObject(obj, c.upstream.Scheme())
	if err != nil {
		return fmt.Errorf("failed to get GVK for object %T: %w", obj, err)
	}

	keyLock := c.lockedKeysByGVK.getOrCreate(gvk).getOrCreate(key)
	if err := keyLock.Wait(ctx); err != nil {
		return err
	}

	return c.upstream.Get(ctx, key, obj, opts...)
}

func (c *consistentClient) List(ctx context.Context, list ObjectList, opts ...ListOption) error {
	gvk, err := apiutil.GVKForObject(list, c.upstream.Scheme())
	if err != nil {
		return fmt.Errorf("failed to get GVK for list %T: %w", list, err)
	}
	gvk.Kind = strings.TrimSuffix(gvk.Kind, "List")

	keys := c.lockedKeysByGVK.getOrCreate(gvk).allValues()
	for _, keyLock := range keys {
		if err := keyLock.Wait(ctx); err != nil {
			return err
		}
	}

	return c.upstream.List(ctx, list, opts...)
}

func (c *consistentClient) Create(ctx context.Context, obj Object, opts ...CreateOption) error {
	return c.writeAndRecordRV(ctx, obj, func() error {
		return c.upstream.Create(ctx, obj, opts...)
	})
}

func (c *consistentClient) Update(ctx context.Context, obj Object, opts ...UpdateOption) error {
	return c.writeAndRecordRV(ctx, obj, func() error {
		return c.upstream.Update(ctx, obj, opts...)
	})
}

func (c *consistentClient) Patch(ctx context.Context, obj Object, patch Patch, opts ...PatchOption) error {
	return c.writeAndRecordRV(ctx, obj, func() error {
		return c.upstream.Patch(ctx, obj, patch, opts...)
	})
}

func (c *consistentClient) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...ApplyOption) error {
	return c.writeAndRecordRV(ctx, obj, func() error {
		return c.upstream.Apply(ctx, obj, opts...)
	})
}

func writeTargetFor(obj any, scheme *runtime.Scheme) (schema.GroupVersionKind, types.NamespacedName, func() (string, error), error) {
	switch t := obj.(type) {
	case *unstructuredApplyConfiguration:
		return t.Unstructured.GroupVersionKind(),
			types.NamespacedName{Namespace: t.Unstructured.GetNamespace(), Name: t.Unstructured.GetName()},
			func() (string, error) { return t.Unstructured.GetResourceVersion(), nil },
			nil
	case applyConfiguration:
		gvk, err := gvkFromApplyConfiguration(t)
		if err != nil {
			return schema.GroupVersionKind{}, types.NamespacedName{}, nil, fmt.Errorf("failed to get GVK for apply configuration %T: %w", obj, err)
		}
		return gvk,
			types.NamespacedName{Namespace: ptr.Deref(t.GetNamespace(), ""), Name: ptr.Deref(t.GetName(), "")},
			func() (string, error) { return resourceVersionFromApplyConfiguration(t) },
			nil
	case Object:
		gvk, err := apiutil.GVKForObject(t, scheme)
		if err != nil {
			return schema.GroupVersionKind{}, types.NamespacedName{}, nil, fmt.Errorf("failed to get GVK for object %T: %w", obj, err)
		}
		return gvk,
			types.NamespacedName{Namespace: t.GetNamespace(), Name: t.GetName()},
			func() (string, error) { return t.GetResourceVersion(), nil },
			nil
	default:
		return schema.GroupVersionKind{}, types.NamespacedName{}, nil, fmt.Errorf("unsupported type %T, must be either %T, %T or %T", obj, Object(nil), &unstructuredApplyConfiguration{}, applyConfiguration(nil))
	}
}

func (c *consistentClient) writeAndRecordRV(ctx context.Context, obj any, write func() error) error {
	gvk, namespacedName, getResourceVersion, err := writeTargetFor(obj, c.upstream.Scheme())
	if err != nil {
		return err
	}

	keyLock := c.lockedKeysByGVK.getOrCreate(gvk).getOrCreate(namespacedName)
	if err := keyLock.Lock(ctx); err != nil {
		return fmt.Errorf("failed to acquire lock for %s/%s: %w", namespacedName.Namespace, namespacedName.Name, err)
	}
	defer keyLock.Unlock()

	if err := write(); err != nil {
		return err
	}

	rvRaw, err := getResourceVersion()
	if err != nil {
		return fmt.Errorf("failed to get resource version from %T: %w", obj, err)
	}
	rv, err := strconv.ParseInt(rvRaw, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse resource version %s: %w", rvRaw, err)
	}
	c.cache.SetMinimumRVForGVKAndKey(gvk, namespacedName, rv)

	return nil
}

func resourceVersionFromApplyConfiguration(obj applyConfiguration) (string, error) {
	v := reflect.ValueOf(obj)
	for v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return "", fmt.Errorf("expected struct, got %s", v.Kind())
	}
	rv := v.FieldByName("ResourceVersion")
	if !rv.IsValid() {
		return "", fmt.Errorf("type %T has no ResourceVersion field", obj)
	}
	if rv.Kind() != reflect.Pointer || rv.Type().Elem().Kind() != reflect.String {
		return "", fmt.Errorf("ResourceVersion field in %T is not *string", obj)
	}
	if rv.IsNil() {
		return "", fmt.Errorf("ResourceVersion field in %T is nil", obj)
	}
	return rv.Elem().String(), nil
}

func (c *consistentClient) Delete(ctx context.Context, obj Object, opts ...DeleteOption) error {
	gvk, err := apiutil.GVKForObject(obj, c.upstream.Scheme())
	if err != nil {
		return fmt.Errorf("failed to get GVK for object %v: %w", obj, err)
	}

	namespacedName := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}

	keyLock := c.lockedKeysByGVK.getOrCreate(gvk).getOrCreate(namespacedName)
	if err := keyLock.Lock(ctx); err != nil {
		return fmt.Errorf("failed to acquire lock for %s/%s: %w", namespacedName.Namespace, namespacedName.Name, err)
	}
	defer keyLock.Unlock()

	// Register the delete before we execute it, otherwise it may be in the cache
	// before we register it, causing a deadlock.
	if err := c.cache.AddRequiredDeleteForObject(obj); err != nil {
		return fmt.Errorf("failed to add required delete for object: %w", err)
	}

	response, err := c.upstream.delete(ctx, obj, opts...)
	if err != nil {
		if removeErr := c.cache.RemoveRequiredDeleteForObject(obj); removeErr != nil {
			return errors.Join(err, fmt.Errorf("failed to remove required delete for object after delete error: %w", removeErr))
		}
		return err
	}

	if rvRaw := response.GetResourceVersion(); rvRaw != "" {
		if err := c.cache.RemoveRequiredDeleteForObject(obj); err != nil {
			return fmt.Errorf("failed to remove required delete for object after successful delete: %w", err)
		}
		rv, err := strconv.ParseInt(rvRaw, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse resource version %s: %w", rvRaw, err)
		}
		c.cache.SetMinimumRVForGVKAndKey(gvk, namespacedName, rv)
	}

	return nil
}

func (c *consistentClient) DeleteAllOf(ctx context.Context, obj Object, opts ...DeleteAllOfOption) error {
	return errors.New("DeleteAllOf is not supported by consistentClient, please use List and Delete instead")
}

func (c *consistentClient) Status() SubResourceWriter {
	return c.SubResource("status")
}

func (c *consistentClient) Scheme() *runtime.Scheme {
	return c.upstream.Scheme()
}

func (c *consistentClient) RESTMapper() meta.RESTMapper {
	return c.upstream.RESTMapper()
}

func (c *consistentClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return c.upstream.GroupVersionKindFor(obj)
}

func (c *consistentClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return c.upstream.IsObjectNamespaced(obj)
}

func (c *consistentClient) SubResource(subResource string) SubResourceClient {
	return &consistentSubResourceClient{
		writeAndRecordRV: c.writeAndRecordRV,
		upstream:         c.upstream.SubResource(subResource),
	}
}

type consistentSubResourceClient struct {
	writeAndRecordRV func(context.Context, any, func() error) error
	upstream         SubResourceClient
}

func (c *consistentSubResourceClient) Get(ctx context.Context, obj, subResource Object, opts ...SubResourceGetOption) error {
	return c.upstream.Get(ctx, obj, subResource, opts...)
}

func (c *consistentSubResourceClient) Create(ctx context.Context, obj, subResource Object, opts ...SubResourceCreateOption) error {
	return c.writeAndRecordRV(ctx, obj, func() error {
		return c.upstream.Create(ctx, obj, subResource, opts...)
	})
}

func (c *consistentSubResourceClient) Update(ctx context.Context, obj Object, opts ...SubResourceUpdateOption) error {
	return c.writeAndRecordRV(ctx, obj, func() error {
		return c.upstream.Update(ctx, obj, opts...)
	})
}

func (c *consistentSubResourceClient) Patch(ctx context.Context, obj Object, patch Patch, opts ...SubResourcePatchOption) error {
	return c.writeAndRecordRV(ctx, obj, func() error {
		return c.upstream.Patch(ctx, obj, patch, opts...)
	})
}

func (c *consistentSubResourceClient) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...SubResourceApplyOption) error {
	return c.writeAndRecordRV(ctx, obj, func() error {
		return c.upstream.Apply(ctx, obj, opts...)
	})
}

// keyLocker implements a mutex with context support
// that also allows to wait for the current lock to
// be released.
// TODO: find a better name
type keyLocker struct {
	// mutex must be held to access done
	mutex sync.Mutex
	// done is nil when no one is holding the lock
	done chan struct{}
}

func (l *keyLocker) Lock(ctx context.Context) error {
	for {
		l.mutex.Lock()
		if l.done == nil {
			l.done = make(chan struct{})
			l.mutex.Unlock()
			return nil
		}

		done := l.done
		l.mutex.Unlock()
		select {
		case <-done: // released, try acquire
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (l *keyLocker) Unlock() {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.done == nil {
		panic("unlock of unlocked mutex")
	}
	close(l.done)
	l.done = nil
}

// Wait waits for the current lock holder if any to
// release the lock.
func (l *keyLocker) Wait(ctx context.Context) error {
	l.mutex.Lock()
	done := l.done
	l.mutex.Unlock()

	if done == nil {
		return nil
	}

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func newThreadSafeMap[k comparable, v any](newValue func() v) *threadSafeMap[k, v] {
	return &threadSafeMap[k, v]{
		data:     map[k]v{},
		newValue: newValue,
	}
}

type threadSafeMap[k comparable, v any] struct {
	lock     sync.Mutex
	data     map[k]v
	newValue func() v
}

func (t *threadSafeMap[k, v]) getOrCreate(key k) v {
	t.lock.Lock()
	defer t.lock.Unlock()

	val, exists := t.data[key]
	if !exists {
		val = t.newValue()
		t.data[key] = val
	}

	return val
}

func (t *threadSafeMap[k, v]) allValues() []v {
	t.lock.Lock()
	defer t.lock.Unlock()

	result := make([]v, 0, len(t.data))
	for _, val := range t.data {
		result = append(result, val)
	}

	return result
}
