package client

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

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
	informerGetter informerGetter
	scheme         *runtime.Scheme

	trackers     map[schema.GroupVersionKind]*highestSeenRVTracker
	trackersLock sync.Mutex

	lockedKeysByGVK map[schema.GroupVersionKind]*lockedKeys
	lockedKeysLock  sync.Mutex
}

type lockedKeys struct {
	lock       sync.RWMutex
	lockedKeys map[types.NamespacedName]rvWaiter
}

type rvWaiter struct {
	waitingForRV int64
	done         chan struct{}
}

func (c *consistentClient) waitForObject(ctx context.Context, obj Object) error {
	gvk, err := apiutil.GVKForObject(obj, c.scheme)
	if err != nil {
		return fmt.Errorf("failed to get GVK for object %v: %v", obj, err)
	}
	key := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}

	c.lockedKeysLock.Lock()
	keys := c.lockedKeysByGVK[gvk]
	c.lockedKeysLock.Unlock()
	if keys == nil {
		return nil
	}

	done := func() chan struct{} {
		keys.lock.RLock()
		defer keys.lock.RUnlock()

		waiter, found := keys.lockedKeys[key]
		if !found {
			return nil
		}

		return waiter.done
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.NewTimer(5 * time.Second).C:
		return errors.New("timed out waiting for cache to have latest changes to object")
	case <-done:
		return nil
	}
}

func (c *consistentClient) blockForObject(ctx context.Context, obj Object) error {
	gvk, err := apiutil.GVKForObject(obj, c.scheme)
	if err != nil {
		return fmt.Errorf("failed to get GVK for object %v: %v", obj, err)
	}

	rvRaw := obj.GetResourceVersion()
	rv, err := strconv.ParseInt(rvRaw, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse resource version %s: %v", rvRaw, err)
	}

	tracker, err := c.trackerFor(ctx, gvk)
	if err != nil {
		return fmt.Errorf("failed to set up tracker for %v: %v", obj, err)
	}

	key := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}
	c.lockedKeysLock.Lock()
	if _, exists := c.lockedKeysByGVK[gvk]; !exists {
		c.lockedKeysByGVK[gvk] = &lockedKeys{lockedKeys: make(map[types.NamespacedName]rvWaiter)}
	}
	keys := c.lockedKeysByGVK[gvk]
	c.lockedKeysLock.Unlock()

	keys.lock.Lock()
	defer keys.lock.Unlock()
	if existing, alreadyWaiting := keys.lockedKeys[key]; alreadyWaiting && existing.waitingForRV >= rv {
		return nil
	}

	keys.lockedKeys[key] = rvWaiter{
		waitingForRV: rv,
		done:         tracker.blockUntil(ctx, rv),
	}

	return nil
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
