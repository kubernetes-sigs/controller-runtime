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

package internal

import (
	"fmt"
	"sync"
	"time"

	"github.com/kubernetes-sigs/controller-runtime/pkg/client"
	"github.com/kubernetes-sigs/controller-runtime/pkg/client/apiutil"
	"k8s.io/apimachinery/pkg/api/meta"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	toolscache "k8s.io/client-go/tools/cache"
)

func NewInformerProvider(config *rest.Config,
	scheme *runtime.Scheme,
	mapper meta.RESTMapper,
	resync time.Duration,
	provider *CacheProvider) *InformerProvider {
	ip := &InformerProvider{
		config:         config,
		scheme:         scheme,
		mapper:         mapper,
		informersByGVK: make(map[schema.GroupVersionKind]toolscache.SharedIndexInformer),
		codecs:         serializer.NewCodecFactory(scheme),
		paramCodec:     runtime.NewParameterCodec(scheme),
		resync:         resync,
		cache:          provider,
	}
	return ip
}

// InformerProvider lazily creates informers, and then caches them for the next time that informer is
// requested.  It uses a standard parameter codec constructed based on the given generated scheme.
type InformerProvider struct {
	// config is used to talk to the apiserver
	config *rest.Config

	// scheme maps runtime.Objects to GroupVersionKinds
	scheme *runtime.Scheme

	// mapper maps GroupVersionKinds to Resources
	mapper meta.RESTMapper

	// informersByGVK is the cache of informers keyed by groupVersionKind
	informersByGVK map[schema.GroupVersionKind]cache.SharedIndexInformer

	// codecs is used to create a new REST client
	codecs serializer.CodecFactory

	// paramCodec is used by list and watch
	paramCodec runtime.ParameterCodec

	// stop is the stop channel to stop informers
	stop <-chan struct{}

	// resync is the frequency the informers are resynced
	resync time.Duration

	// cache is used to register new informers with the cache when they are created
	cache *CacheProvider

	// mu guards access to the map
	mu sync.Mutex

	// start is true if the informers have been started
	started bool
}

// GetInformerForKind returns the informer for the GroupVersionKind
func (ip *InformerProvider) GetInformerForKind(gvk schema.GroupVersionKind) (cache.SharedIndexInformer, error) {
	// Map the gvk to an object
	obj, err := ip.scheme.New(gvk)
	if err != nil {
		return nil, err
	}
	return ip.getInformer(gvk, obj)
}

// GetInformer returns the informer for the obj
func (ip *InformerProvider) GetInformer(obj runtime.Object) (cache.SharedIndexInformer, error) {
	gvk, err := apiutil.GVKForObject(obj, ip.scheme)
	if err != nil {
		return nil, err
	}
	return ip.getInformer(gvk, obj)
}

// Start calls Run on each of the informers and sets started to true.  Blocks on the stop channel.
func (ip *InformerProvider) Start(stop <-chan struct{}) error {
	func() {
		ip.mu.Lock()
		defer ip.mu.Unlock()

		// Set the stop channel so it can be passed to informers that are added later
		ip.stop = stop

		// Start each informer
		for _, informer := range ip.informersByGVK {
			go informer.Run(stop)
		}

		// Set started to true so we immediately start any informers added later.
		ip.started = true
	}()
	<-stop
	return nil
}

// IndexField adds an indexer to the underlying cache, using extraction function to get
// value(s) from the given field.  This index can then be used by passing a field selector
// to List. For one-to-one compatibility with "normal" field selectors, only return one value.
// The values may be anything.  They will automatically be prefixed with the namespace of the
// given object, if present.  The objects passed are guaranteed to be objects of the correct type.
func (ip *InformerProvider) IndexField(obj runtime.Object, field string, extractValue client.IndexerFunc) error {
	informer, err := ip.GetInformer(obj)
	if err != nil {
		return err
	}
	return indexByField(informer.GetIndexer(), field, extractValue)
}

// WaitForCacheSync waits until all the caches have been synced
func (ip *InformerProvider) WaitForCacheSync(stop <-chan struct{}) bool {
	syncedFuncs := make([]toolscache.InformerSynced, 0, len(ip.informersByGVK))
	for _, informer := range ip.informersByGVK {
		syncedFuncs = append(syncedFuncs, informer.HasSynced)
	}
	return toolscache.WaitForCacheSync(stop, syncedFuncs...)
}

func indexByField(indexer cache.Indexer, field string, extractor client.IndexerFunc) error {
	indexFunc := func(objRaw interface{}) ([]string, error) {
		// TODO(directxman12): check if this is the correct type?
		obj, isObj := objRaw.(runtime.Object)
		if !isObj {
			return nil, fmt.Errorf("object of type %T is not an Object", objRaw)
		}
		meta, err := apimeta.Accessor(obj)
		if err != nil {
			return nil, err
		}
		ns := meta.GetNamespace()

		rawVals := extractor(obj)
		var vals []string
		if ns == "" {
			// if we're not doubling the keys for the namespaced case, just re-use what was returned to us
			vals = rawVals
		} else {
			// if we need to add non-namespaced versions too, double the length
			vals = make([]string, len(rawVals)*2)
		}
		for i, rawVal := range rawVals {
			// save a namespaced variant, so that we can ask
			// "what are all the object matching a given index *in a given namespace*"
			vals[i] = keyToNamespacedKey(ns, rawVal)
			if ns != "" {
				// if we have a namespace, also inject a special index key for listing
				// regardless of the object namespace
				vals[i+len(rawVals)] = keyToNamespacedKey("", rawVal)
			}
		}

		return vals, nil
	}

	return indexer.AddIndexers(cache.Indexers{fieldIndexName(field): indexFunc})
}

// getInformer will either return the informer found in the map, or create a new one if none is found.
// If the informer is created, getInformer will block until the new informer is synced.
func (ip *InformerProvider) getInformer(gvk schema.GroupVersionKind, obj runtime.Object) (
	cache.SharedIndexInformer, error) {
	i, err, wait, found := ip.informerFor(gvk, obj)

	// Do this stuff after informerFor returns so it isn't locking the mutex

	// At to the cache if newly created.  ip.cache has its own mutex.
	if !found {
		ip.cache.addInformer(obj, gvk, i)
	}

	// Wait if newly created and informer was started.
	if wait {
		// Wait for it to sync before returning the Informer so that folks don't read from a stale cache.
		if !toolscache.WaitForCacheSync(ip.stop, i.HasSynced) {
			return nil, fmt.Errorf("failed waiting for %T Informer to sync", obj)
		}
	}
	return i, err
}

// informerFor will create a new Informer and added it to the map of InformerProvider if none exists.  Returns
// the Informer from the map.
func (ip *InformerProvider) informerFor(gvk schema.GroupVersionKind, obj runtime.Object) (
	cache.SharedIndexInformer, error, bool, bool) {
	wait := false
	found := true
	ip.mu.Lock()
	defer ip.mu.Unlock()

	// Check the cache to see if we already have an Informer.  If we do, return the Informer.
	i, ok := ip.informersByGVK[gvk]
	if ok {
		return i, nil, wait, found
	}

	found = false
	// Create a new Informer
	i, err := ip.newInformer(gvk, obj)
	if err != nil {
		return nil, err, wait, found
	}

	// If we are already running, then immediately start the Informer.  Otherwise wait until Start is called
	// so that we have the stop Channel available.
	if ip.started {
		// Start the Informer.
		go i.Run(ip.stop)
		wait = true
	}

	return i, nil, wait, found
}

// newInformer creates a new Informer and adds it to the cache
func (ip *InformerProvider) newInformer(gvk schema.GroupVersionKind, obj runtime.Object) (cache.SharedIndexInformer, error) {
	// Get a new ListWatch that will be used to create the SharedIndexInformer
	lw, err := ip.newListWatch(gvk)
	if err != nil {
		return nil, err
	}

	// Create a NewSharedIndexInformer and add it to the map.
	i := cache.NewSharedIndexInformer(lw, obj, ip.resync, cache.Indexers{})
	ip.informersByGVK[gvk] = i

	// Update the cache client with the Informer.
	//ip.objectCache.addInformer(gvk, i)

	return i, nil
}

// newListWatch returns a new ListWatch object that can be used to create a SharedIndexInformer.
func (ip *InformerProvider) newListWatch(gvk schema.GroupVersionKind) (*cache.ListWatch, error) {
	// Construct a RESTClient for the groupVersionKind that we will use to
	// talk to the apiserver.
	client, err := apiutil.RESTClientForGVK(gvk, ip.config, ip.codecs)
	if err != nil {
		return nil, err
	}

	// Kubernetes APIs work against Resources, not GroupVersionKinds.  Map the
	// groupVersionKind to the Resource API we will use.
	mapping, err := ip.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, err
	}

	// Get a listObject for listing that the ListWatch can DeepCopy
	listGVK := gvk.GroupVersion().WithKind(gvk.Kind + "List")
	listObj, err := ip.scheme.New(listGVK)
	if err != nil {
		return nil, err
	}

	// Create a new ListWatch for the obj
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			res := listObj.DeepCopyObject()
			err := client.Get().Resource(mapping.Resource).VersionedParams(&opts, ip.paramCodec).Do().Into(res)
			return res, err
		},
		// Setup the watch function
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			// Watch needs to be set to true separately
			opts.Watch = true
			return client.Get().Resource(mapping.Resource).VersionedParams(&opts, ip.paramCodec).Watch()
		},
	}, nil
}
