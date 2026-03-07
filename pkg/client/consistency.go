package client

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

type cache interface {
	SetMinimumRVForGVKAndKey(gvk schema.GroupVersionKind, key ObjectKey, rv int64)
	AddRequiredDeleteForObject(Object) error
}

type consistentClient struct {
	upstream *client
	cache    cache
	scheme   *runtime.Scheme

	// lockedKeysByGVK maps gvk -> key -> keyLocker
	lockedKeysByGVK threadSafeMap[schema.GroupVersionKind, *threadSafeMap[types.NamespacedName, *keyLocker]]
}

func (c *consistentClient) Get(ctx context.Context, key ObjectKey, obj Object, opts ...GetOption) error {
	gvk, err := apiutil.GVKForObject(obj, c.scheme)
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
	gvk, err := apiutil.GVKForObject(list, c.scheme)
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
	gvk, err := apiutil.GVKForObject(obj, c.scheme)
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
	gvk, err := apiutil.GVKForObject(obj, c.scheme)
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
	gvk, err := apiutil.GVKForObject(obj, c.scheme)
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

func (c *consistentClient) Delete(ctx context.Context, obj Object, opts ...DeleteOption) error {
	gvk, err := apiutil.GVKForObject(obj, c.scheme)
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
