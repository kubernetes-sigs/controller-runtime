package client

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

type cache interface {
	SetMinimumRVForGVKAndKey(gvk schema.GroupVersionKind, key ObjectKey, rv int64)
	AddRequiredDeleteForObject(Object) error
}

var _ Client = (*consistentClient)(nil)

type consistentClient struct {
	upstream *client
	cache    cache

	// lockedKeysByGVK maps gvk -> key -> keyLocker
	lockedKeysByGVK threadSafeMap[schema.GroupVersionKind, *threadSafeMap[types.NamespacedName, *keyLocker]]
}

func (c *consistentClient) Get(ctx context.Context, key ObjectKey, obj Object, opts ...GetOption) error {
	gvk, err := apiutil.GVKForObject(obj, c.upstream.Scheme())
	if err != nil {
		return fmt.Errorf("failed to get GVK for object %T: %v", obj, err)
	}

	keyLock := c.lockedKeysByGVK.getOrCreate(gvk).getOrCreate(key)
	if err := keyLock.wait(ctx); err != nil {
		return err
	}

	return c.upstream.Get(ctx, key, obj, opts...)
}

func (c *consistentClient) List(ctx context.Context, list ObjectList, opts ...ListOption) error {
	gvk, err := apiutil.GVKForObject(list, c.upstream.Scheme())
	if err != nil {
		return fmt.Errorf("failed to get GVK for list %T: %v", list, err)
	}

	keys := c.lockedKeysByGVK.getOrCreate(gvk).allValues()
	for _, keyLock := range keys {
		if err := keyLock.wait(ctx); err != nil {
			return err
		}
	}

	return c.upstream.List(ctx, list, opts...)
}

func (c *consistentClient) Create(ctx context.Context, obj Object, opts ...CreateOption) error {
	gvk, err := apiutil.GVKForObject(obj, c.upstream.Scheme())
	if err != nil {
		return fmt.Errorf("failed to get GVK for object %v: %v", obj, err)
	}

	namespacedName := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}

	keyLock := c.lockedKeysByGVK.getOrCreate(gvk).getOrCreate(namespacedName)
	if err := keyLock.lock(ctx); err != nil {
		return fmt.Errorf("failed to acquire lock for %s/%s: %v", namespacedName.Namespace, namespacedName.Name, err)
	}
	defer keyLock.unlock()

	if err := c.upstream.Create(ctx, obj, opts...); err != nil {
		return err
	}

	rvRaw := obj.GetResourceVersion()
	rv, err := strconv.ParseInt(rvRaw, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse resource version %s: %v", rvRaw, err)
	}
	c.cache.SetMinimumRVForGVKAndKey(gvk, namespacedName, rv)

	return nil
}

func (c *consistentClient) Update(ctx context.Context, obj Object, opts ...UpdateOption) error {
	gvk, err := apiutil.GVKForObject(obj, c.upstream.Scheme())
	if err != nil {
		return fmt.Errorf("failed to get GVK for object %v: %v", obj, err)
	}

	namespacedName := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}

	keyLock := c.lockedKeysByGVK.getOrCreate(gvk).getOrCreate(namespacedName)
	if err := keyLock.lock(ctx); err != nil {
		return fmt.Errorf("failed to acquire lock for %s/%s: %v", namespacedName.Namespace, namespacedName.Name, err)
	}
	defer keyLock.unlock()

	if err := c.upstream.Update(ctx, obj, opts...); err != nil {
		return err
	}

	rvRaw := obj.GetResourceVersion()
	rv, err := strconv.ParseInt(rvRaw, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse resource version %s: %v", rvRaw, err)
	}
	c.cache.SetMinimumRVForGVKAndKey(gvk, namespacedName, rv)

	return nil
}

func (c *consistentClient) Patch(ctx context.Context, obj Object, patch Patch, opts ...PatchOption) error {
	gvk, err := apiutil.GVKForObject(obj, c.upstream.Scheme())
	if err != nil {
		return fmt.Errorf("failed to get GVK for object %v: %v", obj, err)
	}

	namespacedName := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}

	keyLock := c.lockedKeysByGVK.getOrCreate(gvk).getOrCreate(namespacedName)
	if err := keyLock.lock(ctx); err != nil {
		return fmt.Errorf("failed to acquire lock for %s/%s: %v", namespacedName.Namespace, namespacedName.Name, err)
	}
	defer keyLock.unlock()

	if err := c.upstream.Patch(ctx, obj, patch, opts...); err != nil {
		return err
	}

	rvRaw := obj.GetResourceVersion()
	rv, err := strconv.ParseInt(rvRaw, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse resource version %s: %v", rvRaw, err)
	}
	c.cache.SetMinimumRVForGVKAndKey(gvk, namespacedName, rv)

	return nil
}

func (c *consistentClient) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...ApplyOption) error {
	var gvk schema.GroupVersionKind
	var namespacedName types.NamespacedName
	var getResourceVersion func() (string, error)
	switch t := obj.(type) {
	case *unstructuredApplyConfiguration:
		gvk = t.Unstructured.GroupVersionKind()
		namespacedName.Namespace = t.Unstructured.GetNamespace()
		namespacedName.Name = t.Unstructured.GetName()
		getResourceVersion = func() (string, error) {
			return t.Unstructured.GetResourceVersion(), nil
		}
	case applyConfiguration:
		gv, err := schema.ParseGroupVersion(*t.GetAPIVersion())
		if err != nil {
			return fmt.Errorf("failed to parse group version %s: %v", *t.GetAPIVersion(), err)
		}
		gvk.Group = gv.Group
		gvk.Version = gv.Version
		gvk.Kind = *t.GetKind()
		namespacedName.Namespace = *t.GetNamespace()
		namespacedName.Name = *t.GetName()
		getResourceVersion = func() (string, error) {
			return resourceVersionFromApplyConfiguration(t)
		}
	default:
		return fmt.Errorf("unsupported type for Apply: %T, must be either %T or %T", obj, &unstructuredApplyConfiguration{}, applyConfiguration(nil))
	}

	keyLock := c.lockedKeysByGVK.getOrCreate(gvk).getOrCreate(namespacedName)
	if err := keyLock.lock(ctx); err != nil {
		return fmt.Errorf("failed to acquire lock for %s/%s: %v", namespacedName.Namespace, namespacedName.Name, err)
	}
	defer keyLock.unlock()

	if err := c.upstream.Apply(ctx, obj, opts...); err != nil {
		return err
	}

	rvRaw, err := getResourceVersion()
	if err != nil {
		return fmt.Errorf("failed to get resource version from apply configuration: %v", err)
	}
	rv, err := strconv.ParseInt(rvRaw, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse resource version %s: %v", rvRaw, err)
	}
	c.cache.SetMinimumRVForGVKAndKey(gvk, namespacedName, rv)

	return nil
}

func resourceVersionFromApplyConfiguration(obj applyConfiguration) (string, error) {
	v := reflect.ValueOf(obj)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return "", fmt.Errorf("expected struct, got %s", v.Kind())
	}
	rv := v.FieldByName("ResourceVersion")
	if !rv.IsValid() {
		return "", fmt.Errorf("type %T has no ResourceVersion field", obj)
	}
	if rv.Kind() != reflect.Ptr || rv.Type().Elem().Kind() != reflect.String {
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
		return fmt.Errorf("failed to get GVK for object %v: %v", obj, err)
	}

	namespacedName := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}

	keyLock := c.lockedKeysByGVK.getOrCreate(gvk).getOrCreate(namespacedName)
	if err := keyLock.lock(ctx); err != nil {
		return fmt.Errorf("failed to acquire lock for %s/%s: %v", namespacedName.Namespace, namespacedName.Name, err)
	}
	defer keyLock.unlock()

	response, err := c.upstream.delete(ctx, obj, opts...)
	if err != nil {
		return err
	}

	if rvRaw := response.GetResourceVersion(); rvRaw != "" {
		rv, err := strconv.ParseInt(rvRaw, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse resource version %s: %v", rvRaw, err)
		}
		c.cache.SetMinimumRVForGVKAndKey(gvk, namespacedName, rv)
	} else {
		if err := c.cache.AddRequiredDeleteForObject(obj); err != nil {
			return fmt.Errorf("failed to add required delete for object: %v", err)
		}
	}

	return nil
}

func (c *consistentClient) DeleteAllOf(ctx context.Context, obj Object, opts ...DeleteAllOfOption) error {
	return errors.New("DeleteAllOf is not supported by consistentClient, please use List and Delete instead")
}

func (c *consistentClient) Status() SubResourceWriter {
	return c.SubResource("status")
}

func (c *consistentClient) SubResource(subResource string) SubResourceClient {
	panic("not implemented")
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

func (l *keyLocker) lock(ctx context.Context) error {
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

func (l *keyLocker) unlock() {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.done == nil {
		panic("unlock of unlocked mutex")
	}
	close(l.done)
	l.done = nil
}

// wait waits for the current lock holder if any to
// release the lock.
func (l *keyLocker) wait(ctx context.Context) error {
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

type threadSafeMap[k comparable, v any] struct {
	lock sync.Mutex
	data map[k]v
}

func (t *threadSafeMap[k, v]) getOrCreate(key k) v {
	t.lock.Lock()
	defer t.lock.Unlock()

	val, exists := t.data[key]
	if !exists {
		if t.data == nil {
			t.data = make(map[k]v)
		}
		t.data[key] = *new(v)
		val = t.data[key]
	}

	return val
}

func (t *threadSafeMap[k, v]) get(key k) (v, bool) {
	t.lock.Lock()
	defer t.lock.Unlock()

	val, ok := t.data[key]
	return val, ok
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
