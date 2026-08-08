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

package readerconsistency

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"sync"
	"sync/atomic"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewHandler(rvCleanup func(int64)) *ConsistencyHandler {
	return &ConsistencyHandler{
		rvCond:             &broadcaster{},
		pendingDeletesCond: &broadcaster{},
		pendingDeletesLock: sync.RWMutex{},
		pendingDeletes:     make(map[client.ObjectKey]sets.Set[types.UID]),
		rvCleanup:          rvCleanup,
	}
}

type ConsistencyHandler struct {
	rvCond     *broadcaster
	observedRV atomic.Int64

	pendingDeletesCond *broadcaster
	pendingDeletesLock sync.RWMutex
	// pendingDeletes holds pending deletes. Must only be acquired when holding pendingDeletesLock
	pendingDeletes map[client.ObjectKey]sets.Set[types.UID]

	rvCleanup func(int64)
}

func (h *ConsistencyHandler) AddPendingDelete(key client.ObjectKey, uid types.UID) {
	h.pendingDeletesLock.Lock()
	defer h.pendingDeletesLock.Unlock()

	if h.pendingDeletes[key] == nil {
		h.pendingDeletes[key] = sets.New(uid)
		return
	}
	h.pendingDeletes[key].Insert(uid)
}

func (h *ConsistencyHandler) RemovePendingDelete(key client.ObjectKey, uid types.UID) {
	h.pendingDeletesLock.Lock()
	defer h.pendingDeletesLock.Unlock()

	if h.pendingDeletes[key] != nil {
		h.pendingDeletes[key].Delete(uid)
		if len(h.pendingDeletes[key]) == 0 {
			delete(h.pendingDeletes, key)
		}
		h.pendingDeletesCond.broadcast()
	}
}

func (h *ConsistencyHandler) WaitForList(ctx context.Context, minRV int64) error {
	if err := h.waitForRV(ctx, minRV); err != nil {
		return err
	}

	return h.waitAllDeletes(ctx)
}

func (h *ConsistencyHandler) WaitForGet(ctx context.Context, key client.ObjectKey, minRV int64) error {
	if err := h.waitForRV(ctx, minRV); err != nil {
		return err
	}

	return h.waitDeletesForKey(ctx, key)
}

// waitDeletesForKey blocks until all pending deletes at the time of calling it were observed or context times out
func (h *ConsistencyHandler) waitDeletesForKey(ctx context.Context, key client.ObjectKey) error {
	h.pendingDeletesLock.RLock()
	pendingDeletes := maps.Clone(h.pendingDeletes[key])
	h.pendingDeletesLock.RUnlock()

	return h.waitDeletes(ctx, pendingDeletes)
}

// waitDeletesForGVK blocks until all pending deletes at the time of calling it were observed or context times out
func (h *ConsistencyHandler) waitAllDeletes(ctx context.Context) error {
	h.pendingDeletesLock.RLock()
	pendingDeletes := sets.Set[types.UID]{}
	for _, uids := range h.pendingDeletes {
		maps.Copy(pendingDeletes, uids)
	}
	h.pendingDeletesLock.RUnlock()

	return h.waitDeletes(ctx, pendingDeletes)
}

func (h *ConsistencyHandler) waitDeletes(ctx context.Context, uids sets.Set[types.UID]) error {
	if len(uids) == 0 {
		return nil
	}

	for {
		// must store the chan before checking the deletes to guarantee that even if the deletes
		// get  updated after our check and before the select, we still get an event.
		updatedChan := h.pendingDeletesCond.wait()
		h.pendingDeletesLock.RLock()
		done := h.allDeletedLocked(uids)
		h.pendingDeletesLock.RUnlock()
		if done {
			return nil
		}

		select {
		case <-updatedChan:
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (h *ConsistencyHandler) allDeletedLocked(uids sets.Set[types.UID]) bool {
	for wantDeleted := range uids {
		for _, notDeletedUIDs := range h.pendingDeletes {
			if notDeletedUIDs.Has(wantDeleted) {
				return false
			}
		}
	}

	return true
}

func (h *ConsistencyHandler) observeDeletion(obj client.Object) {
	key := client.ObjectKey{Namespace: obj.GetNamespace(), Name: obj.GetName()}
	h.pendingDeletesLock.Lock()
	defer h.pendingDeletesLock.Unlock()

	if h.pendingDeletes[key].Has(obj.GetUID()) {
		h.pendingDeletes[key].Delete(obj.GetUID())
		h.pendingDeletesCond.broadcast()
	}
	if len(h.pendingDeletes[key]) == 0 {
		delete(h.pendingDeletes, key)
	}
}

func (h *ConsistencyHandler) waitForRV(ctx context.Context, rv int64) error {
	for {
		// must store the chan before checking the RV to guarantee that even if the RV
		// gets updated after our check and before the select, we still get an event.
		updatedChan := h.rvCond.wait()
		if h.observedRV.Load() >= rv {
			return nil
		}
		select {
		case <-updatedChan:
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (h *ConsistencyHandler) observeResourceVersion(rv string) {
	parsed, err := strconv.ParseInt(rv, 10, 64)
	if err != nil {
		fmt.Printf("Failed to parse resource version %s: %v\n", rv, err)
		return
	}

	for {
		current := h.observedRV.Load()
		if parsed <= current {
			return
		}

		if h.observedRV.CompareAndSwap(current, parsed) {
			break
		}
	}

	h.rvCond.broadcast()

	go h.rvCleanup(parsed)
}

func (h *ConsistencyHandler) OnAdd(raw any, _ bool) {
	obj, ok := raw.(client.Object)
	if !ok {
		// TODO: Should never happen, log an error?
		return
	}
	go func() { h.observeResourceVersion(obj.GetResourceVersion()) }()
}

func (h *ConsistencyHandler) OnUpdate(_, newObj any) {
	obj, ok := newObj.(client.Object)
	if !ok {
		// TODO: Should never happen, log an error?
		return
	}
	go func() { h.observeResourceVersion(obj.GetResourceVersion()) }()
}

func (h *ConsistencyHandler) OnDelete(raw any) {
	var obj client.Object
	switch t := raw.(type) {
	case client.Object:
		obj = t
	case cache.DeletedFinalStateUnknown:
		obj = t.Obj.(client.Object)
	default:
		// TODO: Should never happen, log an error?
		return
	}
	go func() { h.observeResourceVersion(obj.GetResourceVersion()) }()
	go func() { h.observeDeletion(obj) }()
}

type broadcaster struct {
	lock sync.Mutex
	ch   chan struct{}
}

func (b *broadcaster) broadcast() {
	b.lock.Lock()
	defer b.lock.Unlock()
	// Lazily create the channel in wait to avoid creating a channel per event.
	if b.ch != nil {
		close(b.ch)
		b.ch = nil
	}
}

func (b *broadcaster) wait() <-chan struct{} {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.ch == nil {
		b.ch = make(chan struct{})
	}

	return b.ch
}
