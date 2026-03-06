package cache

import (
	"strconv"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type minimumRVStore struct {
	mu       sync.Mutex
	minimums map[schema.GroupVersionKind]map[client.ObjectKey]int64
}

func newMinimumRVStore() *minimumRVStore {
	return &minimumRVStore{
		minimums: make(map[schema.GroupVersionKind]map[client.ObjectKey]int64),
	}
}

func (s *minimumRVStore) Set(gvk schema.GroupVersionKind, key client.ObjectKey, rv int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.minimums[gvk] == nil {
		s.minimums[gvk] = make(map[client.ObjectKey]int64)
	}
	s.minimums[gvk][key] = rv
}

func (s *minimumRVStore) GetForKey(gvk schema.GroupVersionKind, key client.ObjectKey) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	keys, ok := s.minimums[gvk]
	if !ok {
		return 0, false
	}
	rv, ok := keys[key]
	return rv, ok
}

func (s *minimumRVStore) GetMaxForGVK(gvk schema.GroupVersionKind) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	keys, ok := s.minimums[gvk]
	if !ok || len(keys) == 0 {
		return 0, false
	}
	var maxRV int64
	for _, rv := range keys {
		if rv > maxRV {
			maxRV = rv
		}
	}
	return maxRV, true
}

func (s *minimumRVStore) Cleanup(gvk schema.GroupVersionKind, currentRV int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	keys, ok := s.minimums[gvk]
	if !ok {
		return
	}
	for key, rv := range keys {
		if rv <= currentRV {
			delete(keys, key)
		}
	}
}

type highestSeenRVTracker struct {
	rv   int64
	cond *sync.Cond
}

func newHighestSeenRVTracker(initialRV int64) *highestSeenRVTracker {
	return &highestSeenRVTracker{
		rv:   initialRV,
		cond: sync.NewCond(&sync.Mutex{}),
	}
}

func (h *highestSeenRVTracker) blockUntil(rv int64) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		h.cond.L.Lock()
		for h.rv < rv {
			h.cond.Wait()
		}
		h.cond.L.Unlock()
		close(ch)
	}()
	return ch
}

func (h *highestSeenRVTracker) update(rv string) {
	parsed, err := strconv.ParseInt(rv, 10, 64)
	if err != nil {
		return
	}

	h.cond.L.Lock()
	defer h.cond.L.Unlock()

	if parsed > h.rv {
		h.rv = parsed
		h.cond.Broadcast()
	}
}

func (h *highestSeenRVTracker) OnAdd(raw any, _ bool) {
	obj, ok := raw.(client.Object)
	if !ok {
		return
	}
	go func() { h.update(obj.GetResourceVersion()) }()
}

func (h *highestSeenRVTracker) OnUpdate(_, newObj any) {
	obj, ok := newObj.(client.Object)
	if !ok {
		return
	}
	go func() { h.update(obj.GetResourceVersion()) }()
}

func (h *highestSeenRVTracker) OnDelete(raw any) {
	obj, ok := raw.(client.Object)
	if !ok {
		return
	}
	go func() { h.update(obj.GetResourceVersion()) }()
}
