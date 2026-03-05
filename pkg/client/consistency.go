package client

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	toolscache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

type informerGetter interface {
	GetInformer(context.Context, schema.GroupVersionKind) (informer, error)
}

type informer interface {
	AddEventHandler(handler toolscache.ResourceEventHandler) (toolscache.ResourceEventHandlerRegistration, error)
	LastSyncResourceVersion() string
}

func ConsistentClient(upstream Client, ig informerGetter) Client {

}

type consistentClient struct {
	upstream       Client
	informerGetter informerGetter
	scheme         *runtime.Scheme

	trackers     map[schema.GroupVersionKind]*highestSeenRVTracker
	trackersLock sync.Mutex

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

func (c *consistentClient) Update(ctx context.Context, obj Object, opts ...UpdateOption) error {
	gvk, err := apiutil.GVKForObject(obj, c.scheme)
	if err != nil {
		return fmt.Errorf("failed to get GVK for object %v: %v", obj, err)
	}

	namespacedName := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}

	keyLock := c.lockedKeysByGVK.getOrCreate(gvk).getOrCreate(namespacedName)
	keyLock.lock()
	defer keyLock.unlock()

	if err := c.upstream.Update(ctx, obj, opts...); err != nil {
		return err
	}

	rvRaw := obj.GetResourceVersion()
	rv, err := strconv.ParseInt(rvRaw, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse resource version %s: %v", rvRaw, err)
	}

	go func() {
		_ = c.blockFor(ctx, gvk, rv)
	}()
	return nil
}

type lockedKeys struct {
	lock       sync.RWMutex
	lockedKeys map[types.NamespacedName]*keyLocker
}

type keyLocker struct {
	// mutex must be held to access any other field
	mutex sync.Mutex

	// holders counts the holders. If zero, done
	// may be nil
	holders int
	done    chan struct{}
}

func (l *keyLocker) lock() {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.holders == 0 {
		l.done = make(chan struct{})
	}
	l.holders++
}

func (l *keyLocker) unlock() {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.holders--
	if l.holders == 0 {
		close(l.done)
	}
}

func (l *keyLocker) wait(ctx context.Context) error {
	l.mutex.Lock()
	if l.holders == 0 {
		l.mutex.Unlock()
		return nil
	}
	done := l.done
	l.mutex.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

// blockForObject blocks until the context is done or the RV in obj is observed. It may end up
// creating an informer.
func (c *consistentClient) blockFor(ctx context.Context, gvk schema.GroupVersionKind, rv int64) error {
	tracker, err := c.trackerFor(ctx, gvk)
	if err != nil {
		return fmt.Errorf("failed to set up tracker for %v: %v", gvk, err)
	}

	select {
	case <-tracker.blockUntil(ctx, rv):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *consistentClient) trackerFor(ctx context.Context, obj schema.GroupVersionKind) (*highestSeenRVTracker, error) {
	c.trackersLock.Lock()
	defer c.trackersLock.Unlock()

	tracker, ok := c.trackers[obj]
	if ok {
		return tracker, nil
	}

	informer, err := c.informerGetter.GetInformer(ctx, obj)
	if err != nil {
		return nil, fmt.Errorf("Failed to get informer for %v: %v", obj, err)
	}

	stringRV := informer.LastSyncResourceVersion()
	rv, err := strconv.ParseInt(stringRV, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse resource version %s: %v", stringRV, err)
	}

	c.trackers[obj] = &highestSeenRVTracker{
		rv:   rv,
		cond: &sync.Cond{},
	}

	return c.trackers[obj], nil
}

type highestSeenRVTracker struct {
	rv   int64
	cond *sync.Cond
}

func (h *highestSeenRVTracker) blockUntil(ctx context.Context, rv int64) chan struct{} {
	c := make(chan struct{})
	go func() {
		h.cond.L.Lock()
		for h.rv < rv {
			h.cond.Wait()
		}
		h.cond.L.Unlock()
		close(c)
	}()

	return c
}

func (h *highestSeenRVTracker) update(rv string) {
	parsed, err := strconv.ParseInt(rv, 10, 64)
	if err != nil {
		fmt.Printf("Failed to parse resource version %s: %v\n", rv, err)
		return
	}

	h.cond.L.Lock()
	defer h.cond.L.Unlock()

	if parsed > h.rv {
		h.rv = parsed
		h.cond.Broadcast()
	}
}

func (h *highestSeenRVTracker) OnAdd(raw interface{}, isInInitialList bool) {
	obj, ok := raw.(Object)
	if !ok {
		// Never expected, should we log an error?
		return
	}
	go func() { h.update(obj.GetResourceVersion()) }()
}
func (h *highestSeenRVTracker) OnUpdate(oldObj, newObj interface{}) {
	obj, ok := newObj.(Object)
	if !ok {
		// Never expected, should we log an error?
		return
	}
	go func() { h.update(obj.GetResourceVersion()) }()
}
func (h *highestSeenRVTracker) OnDelete(raw interface{}) {
	obj, ok := raw.(Object)
	if !ok {
		// Could be cache.DeletedFinalStateUnknown, will we
		// get the latest RV through `OnUpdate` if that happens?
		return
	}
	go func() { h.update(obj.GetResourceVersion()) }()
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
