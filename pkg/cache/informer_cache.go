/*
Copyright 2018 The Kubernetes Authors.

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

package cache

import (
	"context"
	"fmt"
	"strings"
	"sync"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"sigs.k8s.io/controller-runtime/pkg/cache/internal"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

var (
	_ Informers     = &informerCache{}
	_ client.Reader = &informerCache{}
	_ Cache         = &informerCache{}
)

// ErrCacheNotStarted is returned when trying to read from the cache that wasn't started.
type ErrCacheNotStarted struct{}

func (*ErrCacheNotStarted) Error() string {
	return "the cache is not started, can not read objects"
}

var _ error = (*ErrCacheNotStarted)(nil)

// ErrResourceNotCached indicates that the resource type
// the client asked the cache for is not cached, i.e. the
// corresponding informer does not exist yet.
type ErrResourceNotCached = internal.ErrResourceNotCached

// informerCache is a Kubernetes Object cache populated from internal.Informers.
// informerCache wraps internal.Informers.
type informerCache struct {
	scheme *runtime.Scheme
	*internal.Informers
	readerFailOnMissingInformer bool

	// minimumRVs stores the minimum RVs we must have seen before returning reads. Due to RV
	// being just a version, we can store this before we have an informer. This is
	// different from deletes, where we can only observe deletes once we do have an informer,
	// so we only allow storing required deletes as part of the informer.
	minimumRVs *minimumRVStore
}

// Get implements Reader.
func (ic *informerCache) Get(ctx context.Context, key client.ObjectKey, out client.Object, opts ...client.GetOption) error {
	gvk, err := apiutil.GVKForObject(out, ic.scheme)
	if err != nil {
		return err
	}

	started, cache, err := ic.getInformerForKind(ctx, gvk, out)
	if err != nil {
		return err
	}

	if !started {
		return &ErrCacheNotStarted{}
	}

	return cache.Reader.Get(ctx, key, out, ic.minimumRVs.GetForKey(gvk, key), opts...)
}

// List implements Reader.
func (ic *informerCache) List(ctx context.Context, out client.ObjectList, opts ...client.ListOption) error {
	gvk, cacheTypeObj, err := ic.objectTypeForListObject(out)
	if err != nil {
		return err
	}

	started, cache, err := ic.getInformerForKind(ctx, *gvk, cacheTypeObj)
	if err != nil {
		return err
	}

	if !started {
		return &ErrCacheNotStarted{}
	}

	return cache.Reader.List(ctx, out, ic.minimumRVs.GetMaxForGVK(*gvk), opts...)
}

// objectTypeForListObject tries to find the runtime.Object and associated GVK
// for a single object corresponding to the passed-in list type. We need them
// because they are used as cache map key.
func (ic *informerCache) objectTypeForListObject(list client.ObjectList) (*schema.GroupVersionKind, runtime.Object, error) {
	gvk, err := apiutil.GVKForObject(list, ic.scheme)
	if err != nil {
		return nil, nil, err
	}

	// We need the non-list GVK, so chop off the "List" from the end of the kind.
	gvk.Kind = strings.TrimSuffix(gvk.Kind, "List")

	// Handle unstructured.UnstructuredList.
	if _, isUnstructured := list.(runtime.Unstructured); isUnstructured {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(gvk)
		return &gvk, u, nil
	}
	// Handle metav1.PartialObjectMetadataList.
	if _, isPartialObjectMetadata := list.(*metav1.PartialObjectMetadataList); isPartialObjectMetadata {
		pom := &metav1.PartialObjectMetadata{}
		pom.SetGroupVersionKind(gvk)
		return &gvk, pom, nil
	}

	// Any other list type should have a corresponding non-list type registered
	// in the scheme. Use that to create a new instance of the non-list type.
	cacheTypeObj, err := ic.scheme.New(gvk)
	if err != nil {
		return nil, nil, err
	}
	return &gvk, cacheTypeObj, nil
}

func applyGetOptions(opts ...InformerGetOption) *internal.GetOptions {
	cfg := &InformerGetOptions{}
	for _, opt := range opts {
		opt(cfg)
	}
	return (*internal.GetOptions)(cfg)
}

// GetInformerForKind returns the informer for the GroupVersionKind. If no informer exists, one will be started.
func (ic *informerCache) GetInformerForKind(ctx context.Context, gvk schema.GroupVersionKind, opts ...InformerGetOption) (Informer, error) {
	// Map the gvk to an object
	obj, err := ic.scheme.New(gvk)
	if err != nil {
		return nil, err
	}

	_, i, err := ic.Informers.Get(ctx, gvk, obj, false, applyGetOptions(opts...))
	if err != nil {
		return nil, err
	}
	return i.Informer, nil
}

// GetInformer returns the informer for the obj. If no informer exists, one will be started.
func (ic *informerCache) GetInformer(ctx context.Context, obj client.Object, opts ...InformerGetOption) (Informer, error) {
	gvk, err := apiutil.GVKForObject(obj, ic.scheme)
	if err != nil {
		return nil, err
	}

	_, i, err := ic.Informers.Get(ctx, gvk, obj, false, applyGetOptions(opts...))
	if err != nil {
		return nil, err
	}
	return i.Informer, nil
}

func (ic *informerCache) getInformerForKind(ctx context.Context, gvk schema.GroupVersionKind, obj runtime.Object) (bool, *internal.Cache, error) {
	started, cache, err := ic.Informers.Get(ctx, gvk, obj, ic.readerFailOnMissingInformer, &internal.GetOptions{})
	if err != nil {
		return false, nil, err
	}
	return started, cache, nil
}

func (ic *informerCache) SetMinimumRVForGVKAndKey(gvk schema.GroupVersionKind, key client.ObjectKey, rv int64) {
	ic.minimumRVs.Set(gvk, key, rv)
}

func (ic *informerCache) AddRequiredDeleteForObject(obj client.Object) error {
	gvk, err := apiutil.GVKForObject(obj, ic.scheme)
	if err != nil {
		return err
	}
	cache, started, ok := ic.Peek(gvk, obj)
	if !ok {
		return fmt.Errorf("informer for GVK %v not found in cache", gvk)
	}
	if !started {
		return &ErrCacheNotStarted{}
	}
	if !cache.Informer.HasSynced() {
		return fmt.Errorf("informer for GVK %v is not synced", gvk)
	}

	cache.Reader.ConsistencyHandler.AddPendingDelete(
		client.ObjectKey{Namespace: obj.GetNamespace(), Name: obj.GetName()},
		obj.GetUID(),
	)
	return nil
}

// RemoveInformer deactivates and removes the informer from the cache.
func (ic *informerCache) RemoveInformer(_ context.Context, obj client.Object) error {
	gvk, err := apiutil.GVKForObject(obj, ic.scheme)
	if err != nil {
		return err
	}

	ic.Informers.Remove(gvk, obj)
	return nil
}

// NeedLeaderElection implements the LeaderElectionRunnable interface
// to indicate that this can be started without requiring the leader lock.
func (ic *informerCache) NeedLeaderElection() bool {
	return false
}

// IndexField adds an indexer to the underlying informer, using extractValue function to get
// value(s) from the given field. This index can then be used by passing a field selector
// to List. For one-to-one compatibility with "normal" field selectors, only return one value.
// The values may be anything. They will automatically be prefixed with the namespace of the
// given object, if present. The objects passed are guaranteed to be objects of the correct type.
func (ic *informerCache) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	informer, err := ic.GetInformer(ctx, obj, BlockUntilSynced(false))
	if err != nil {
		return err
	}
	return indexByField(informer, field, extractValue)
}

func indexByField(informer Informer, field string, extractValue client.IndexerFunc) error {
	indexFunc := func(objRaw any) ([]string, error) {
		// TODO(directxman12): check if this is the correct type?
		obj, isObj := objRaw.(client.Object)
		if !isObj {
			return nil, fmt.Errorf("object of type %T is not an Object", objRaw)
		}
		meta, err := apimeta.Accessor(obj)
		if err != nil {
			return nil, err
		}
		ns := meta.GetNamespace()

		rawVals := extractValue(obj)
		var vals []string
		if ns == "" {
			// if we're not doubling the keys for the namespaced case, just create a new slice with same length
			vals = make([]string, len(rawVals))
		} else {
			// if we need to add non-namespaced versions too, double the length
			vals = make([]string, len(rawVals)*2)
		}
		for i, rawVal := range rawVals {
			// save a namespaced variant, so that we can ask
			// "what are all the object matching a given index *in a given namespace*"
			vals[i] = internal.KeyToNamespacedKey(ns, rawVal)
			if ns != "" {
				// if we have a namespace, also inject a special index key for listing
				// regardless of the object namespace
				vals[i+len(rawVals)] = internal.KeyToNamespacedKey("", rawVal)
			}
		}

		return vals, nil
	}

	return informer.AddIndexers(cache.Indexers{internal.FieldIndexName(field): indexFunc})
}

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

func (s *minimumRVStore) GetForKey(gvk schema.GroupVersionKind, key client.ObjectKey) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	keys, ok := s.minimums[gvk]
	if !ok {
		return 0
	}
	return keys[key]
}

func (s *minimumRVStore) GetMaxForGVK(gvk schema.GroupVersionKind) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	keys, ok := s.minimums[gvk]
	if !ok || len(keys) == 0 {
		return 0
	}
	var maxRV int64
	for _, rv := range keys {
		if rv > maxRV {
			maxRV = rv
		}
	}
	return maxRV
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
