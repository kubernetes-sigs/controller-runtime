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

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewHandler(rvCleanup func(int64)) *ConsistencyHandler {
	return &ConsistencyHandler{
		rvCond:             sync.NewCond(&sync.Mutex{}),
		pendingDeletesCond: sync.NewCond(&sync.Mutex{}),
		pendingDeletes:     make(map[client.ObjectKey]sets.Set[types.UID]),
		rvCleanup:          rvCleanup,
	}
}

type ConsistencyHandler struct {
	rvCond     *sync.Cond
	observedRV int64

	pendingDeletesCond *sync.Cond
	// pendingDeletes holds pending deletes. Must only be acquired when holding pendingDeletesCond.L
	pendingDeletes map[client.ObjectKey]sets.Set[types.UID]

	rvCleanup func(int64)
}

func (h *ConsistencyHandler) AddPendingDelete(key client.ObjectKey, uid types.UID) {
	h.pendingDeletesCond.L.Lock()
	defer h.pendingDeletesCond.L.Unlock()

	if h.pendingDeletes[key] == nil {
		h.pendingDeletes[key] = sets.New(uid)
		return
	}
	h.pendingDeletes[key].Insert(uid)
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
	h.pendingDeletesCond.L.Lock()
	pendingDeletes := maps.Clone(h.pendingDeletes[key])
	h.pendingDeletesCond.L.Unlock()

	return h.waitDeletes(ctx, pendingDeletes)
}

// waitDeletesForGVK blocks until all pending deletes at the time of calling it were observed or context times out
func (h *ConsistencyHandler) waitAllDeletes(ctx context.Context) error {
	h.pendingDeletesCond.L.Lock()
	pendingDeletes := sets.Set[types.UID]{}
	for _, uids := range h.pendingDeletes {
		maps.Copy(pendingDeletes, uids)
	}
	h.pendingDeletesCond.L.Unlock()

	return h.waitDeletes(ctx, pendingDeletes)
}

func (h *ConsistencyHandler) waitDeletes(ctx context.Context, uids sets.Set[types.UID]) error {
	if len(uids) == 0 {
		return nil
	}

	allDeleted := make(chan struct{})
	go func() {
		h.pendingDeletesCond.L.Lock()
		for !h.allDeletedLocked(uids) {
			if ctx.Err() != nil {
				break
			}
			h.pendingDeletesCond.Wait()
		}
		h.pendingDeletesCond.L.Unlock()
		close(allDeleted)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-allDeleted:
		return nil
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
	h.pendingDeletesCond.L.Lock()
	defer h.pendingDeletesCond.L.Unlock()

	if h.pendingDeletes[key].Has(obj.GetUID()) {
		h.pendingDeletes[key].Delete(obj.GetUID())
		h.pendingDeletesCond.Broadcast()
	}
	if len(h.pendingDeletes[key]) == 0 {
		delete(h.pendingDeletes, key)
	}
}

func (h *ConsistencyHandler) waitForRV(ctx context.Context, rv int64) error {
	observed := make(chan struct{})
	go func() {
		h.rvCond.L.Lock()
		for h.observedRV <= rv {
			if ctx.Err() != nil {
				break
			}
			h.rvCond.Wait()
		}
		h.rvCond.L.Unlock()
		close(observed)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-observed:
		return nil
	}
}

func (h *ConsistencyHandler) observeResourceVersion(rv string) {
	parsed, err := strconv.ParseInt(rv, 10, 64)
	if err != nil {
		fmt.Printf("Failed to parse resource version %s: %v\n", rv, err)
		return
	}

	h.rvCond.L.Lock()
	defer h.rvCond.L.Unlock()

	if parsed > h.observedRV {
		h.observedRV = parsed
		h.rvCond.Broadcast()
	}

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
