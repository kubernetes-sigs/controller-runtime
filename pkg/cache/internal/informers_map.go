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
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// clientListWatcherFunc knows how to create a ListWatcher
type createListWatcherFunc func(schema.GroupVersionKind, *specificInformersMap, listOptionsModifier) (*cache.ListWatch, error)

type listOptionsModifier func(*metav1.ListOptions)

// newSpecificInformersMap returns a new specificInformersMap (like
// the generic InformersMap, except that it doesn't implement WaitForCacheSync).
func newSpecificInformersMap(config *rest.Config,
	scheme *runtime.Scheme,
	mapper meta.RESTMapper,
	resync time.Duration,
	namespace string,
	createListWatcher createListWatcherFunc) *specificInformersMap {
	ip := &specificInformersMap{
		config:            config,
		Scheme:            scheme,
		mapper:            mapper,
		informersByGVK:    make(map[schema.GroupVersionKind]MappedInformer),
		codecs:            serializer.NewCodecFactory(scheme),
		paramCodec:        runtime.NewParameterCodec(scheme),
		resync:            resync,
		startWait:         make(chan struct{}),
		createListWatcher: createListWatcher,
		namespace:         namespace,
	}
	return ip
}

// MappedInformer is used internal for holding an InformerReader or InformerReaderPool.
type MappedInformer interface {
	Run(<-chan struct{})
	HasSynced() bool
}

// InformerReader can return both an Informer and CacheReader.
type InformerReader interface {
	GetInformer() Informer
	GetReader() *CacheReader
}

// InformerReaderPool contains a set of InformerReaders indexed by an owner name string, typically that
// of a controller. Filtered informers use this interface to identify which controller has established
// an Informer.
type InformerReaderPool interface {
	Add(string, schema.GroupVersionKind, cache.SharedIndexInformer, listWatchFilter) InformerReader
	Get(string) (InformerReader, bool)
	HasFilter(string, listWatchFilter) bool
}

var _ MappedInformer = &globalInformerReader{}
var _ InformerReader = &globalInformerReader{}

type globalInformerReader struct {
	// Reader wraps Informer and implements the client.Reader interface for a single type.
	reader   *CacheReader
	informer cache.SharedIndexInformer
}

func newGlobalInformerReader(gvk schema.GroupVersionKind, i cache.SharedIndexInformer) globalInformerReader {
	return globalInformerReader{
		reader: &CacheReader{
			groupVersionKind: gvk,
			indexers:         []cache.Indexer{i.GetIndexer()},
		},
		informer: i,
	}
}

func (ir globalInformerReader) Run(stop <-chan struct{}) { ir.informer.Run(stop) }
func (ir globalInformerReader) HasSynced() bool          { return ir.informer.HasSynced() }
func (ir globalInformerReader) GetInformer() Informer    { return ir.informer }
func (ir globalInformerReader) GetReader() *CacheReader  { return ir.reader }

var _ MappedInformer = &informerReaderPool{}
var _ InformerReaderPool = &informerReaderPool{}

type informerReaderPool map[string]filteredInformerReader

func (p informerReaderPool) Run(stop <-chan struct{}) {
	for _, ir := range p {
		for _, i := range ir.informers {
			go i.informer.Run(stop)
		}
	}
}

func (p informerReaderPool) HasSynced() bool {
	for _, ir := range p {
		if !ir.informers.HasSynced() {
			return false
		}
	}
	return true
}

func (p informerReaderPool) Add(ownerName string, gvk schema.GroupVersionKind, i cache.SharedIndexInformer,
	filter listWatchFilter) InformerReader {

	e, ok := p[ownerName]
	if ok {
		for _, fi := range e.informers {
			if fi.filter.equals(filter) {
				return e
			}
		}
	}
	e.groupVersionKind = gvk
	e.informers = append(e.informers, filteredInformer{informer: i, filter: filter})
	return e
}

func (p informerReaderPool) Get(ownerName string) (InformerReader, bool) {
	ir, ok := p[ownerName]
	return ir, ok
}

func (p informerReaderPool) HasFilter(ownerName string, filter listWatchFilter) bool {
	if ir, ok := p[ownerName]; ok {
		for _, i := range ir.informers {
			if filter.equals(i.filter) {
				return true
			}
		}
	}
	return false
}

var _ InformerReader = &filteredInformerReader{}

type filteredInformerReader struct {
	groupVersionKind schema.GroupVersionKind
	informers        filteredInformers
}

func (ir filteredInformerReader) GetInformer() Informer { return ir.informers }
func (ir filteredInformerReader) GetReader() *CacheReader {
	r := &CacheReader{
		groupVersionKind: ir.groupVersionKind,
		indexers:         make([]cache.Indexer, len(ir.informers)),
	}
	// TODO(estroz) r.indexers is populated each time this method is called so only current informer's indexers
	// are added, which is inefficient. A better way should be found to handle adding/removing an indexer
	// when its informer is added/removed from the filteredInformerReader.
	for idx, i := range ir.informers {
		r.indexers[idx] = i.informer.GetIndexer()
	}
	return r
}

// specificInformersMap create and caches Informers for (runtime.Object, schema.GroupVersionKind) pairs.
// It uses a standard parameter codec constructed based on the given generated Scheme.
type specificInformersMap struct {
	// Scheme maps runtime.Objects to GroupVersionKinds
	Scheme *runtime.Scheme

	// config is used to talk to the apiserver
	config *rest.Config

	// mapper maps GroupVersionKinds to Resources
	mapper meta.RESTMapper

	// informersByGVK is the cache of informers keyed by groupVersionKind
	informersByGVK map[schema.GroupVersionKind]MappedInformer

	// codecs is used to create a new REST client
	codecs serializer.CodecFactory

	// paramCodec is used by list and watch
	paramCodec runtime.ParameterCodec

	// stop is the stop channel to stop informers
	stop <-chan struct{}

	// resync is the base frequency the informers are resynced
	// a 10 percent jitter will be added to the resync period between informers
	// so that all informers will not send list requests simultaneously.
	resync time.Duration

	// mu guards access to the map
	mu sync.RWMutex

	// start is true if the informers have been started
	started bool

	// startWait is a channel that is closed after the
	// informer has been started.
	startWait chan struct{}

	// createClient knows how to create a client and a list object,
	// and allows for abstracting over the particulars of structured vs
	// unstructured objects.
	createListWatcher createListWatcherFunc

	// namespace is the namespace that all ListWatches are restricted to
	// default or empty string means all namespaces
	namespace string
}

// Start calls Run on each of the informers and sets started to true.  Blocks on the stop channel.
// It doesn't return start because it can't return an error, and it's not a runnable directly.
func (ip *specificInformersMap) Start(stop <-chan struct{}) {
	func() {
		ip.mu.Lock()
		defer ip.mu.Unlock()

		// Set the stop channel so it can be passed to informers that are added later
		ip.stop = stop

		// Start each informer
		for _, mi := range ip.informersByGVK {
			go mi.Run(stop)
		}

		// Set started to true so we immediately start any informers added later.
		ip.started = true
		close(ip.startWait)
	}()
	<-stop
}

func (ip *specificInformersMap) waitForStarted(stop <-chan struct{}) bool {
	select {
	case <-ip.startWait:
		return true
	case <-stop:
		return false
	}
}

// HasSyncedFuncs returns all the HasSynced functions for the informers in this map.
func (ip *specificInformersMap) HasSyncedFuncs() []cache.InformerSynced {
	ip.mu.RLock()
	defer ip.mu.RUnlock()
	syncedFuncs := make([]cache.InformerSynced, 0, len(ip.informersByGVK))
	for _, mi := range ip.informersByGVK {
		syncedFuncs = append(syncedFuncs, mi.HasSynced)
	}
	return syncedFuncs
}

// Get will create a new Informer and add it to the map of specificInformersMap if none exists.  Returns
// the Informer from the map.
func (ip *specificInformersMap) Get(ctx context.Context, gvk schema.GroupVersionKind, obj runtime.Object,
	listOpts ...client.ListOption) (InformerReader, bool, error) {

	// Extract owner context from ctx.
	ownerName := ""
	if ownerNameVal := ctx.Value(InformerOwnerNameKey{}); ownerNameVal != nil {
		ownerName = ownerNameVal.(string)
	}

	// TODO(estroz): remove client.InNamespace list option.
	filter := newListWatchFilter(listOpts)

	// Return the informer if it is found
	ir, started, ok := func() (InformerReader, bool, bool) {
		ip.mu.RLock()
		defer ip.mu.RUnlock()
		mi, ok := ip.informersByGVK[gvk]
		if ok {
			switch t := mi.(type) {
			case InformerReader:
				return t, ip.started, ok
			case InformerReaderPool:
				if t.HasFilter(ownerName, filter) {
					ir, hasIR := t.Get(ownerName)
					return ir, ip.started, hasIR
				}
			default:
				panic(fmt.Sprintf("type %T is not permitted in informer map", t))
			}
		}

		return nil, ip.started, false
	}()

	if !ok {
		var err error
		if ir, started, err = ip.addInformerToMap(ownerName, gvk, obj, filter); err != nil {
			return nil, started, err
		}
	}

	if started && !ir.GetInformer().HasSynced() {
		// Wait for it to sync before returning the Informer so that folks don't read from a stale cache.
		if !cache.WaitForCacheSync(ctx.Done(), ir.GetInformer().HasSynced) {
			return nil, started, apierrors.NewTimeoutError(fmt.Sprintf("failed waiting for %T Informer to sync", obj), 0)
		}
	}

	return ir, started, nil
}

func (ip *specificInformersMap) addInformerToMap(ownerName string, gvk schema.GroupVersionKind, obj runtime.Object,
	filter listWatchFilter) (InformerReader, bool, error) {

	ip.mu.Lock()
	defer ip.mu.Unlock()

	// Check the cache to see if we already have an Informer. If we do, return the Informer.
	// This is for the case where 2 routines tried to get the informer when it wasn't in the map
	// so neither returned early, but the first one created it.
	mi, ok := ip.informersByGVK[gvk]
	if ok {
		switch t := mi.(type) {
		case InformerReader:
			return t, ip.started, nil
		case InformerReaderPool:
			if ir, hasIR := t.Get(ownerName); hasIR && t.HasFilter(ownerName, filter) {
				return ir, ip.started, nil
			}
			i, err := ip.newSharedIndexInformer(gvk, obj, filter)
			if err != nil {
				return nil, false, err
			}
			ir := t.Add(ownerName, gvk, i, filter)
			return ir, ip.started, nil
		default:
			panic(fmt.Sprintf("type %T is not permitted in informer map", t))
		}
	}

	// Create a NewSharedIndexInformer and add it to the map.
	i, err := ip.newSharedIndexInformer(gvk, obj, filter)
	if err != nil {
		return nil, false, err
	}

	// TODO(estroz): either return an error or stop all filtered informers if a global informer is being added
	// and filtered informers already exist.
	var ir InformerReader
	if ownerName == "" || filter.isEmpty() {
		mi = newGlobalInformerReader(gvk, i)
		ir = mi.(InformerReader)
	} else {
		mi = make(informerReaderPool)
		ir = mi.(informerReaderPool).Add(ownerName, gvk, i, filter)
	}
	ip.informersByGVK[gvk] = mi

	// Start the Informer if need by
	// TODO(seans): write thorough tests and document what happens here - can you add indexers?
	// can you add eventhandlers?
	if ip.started {
		go mi.Run(ip.stop)
	}
	return ir, ip.started, nil
}

func (ip *specificInformersMap) newSharedIndexInformer(gvk schema.GroupVersionKind, obj runtime.Object, filter listWatchFilter) (cache.SharedIndexInformer, error) {
	lw, err := ip.createListWatcher(gvk, ip, filter.toListOptionsModifier())
	if err != nil {
		return nil, err
	}
	return cache.NewSharedIndexInformer(lw, obj, resyncPeriod(ip.resync)(), cache.Indexers{
		cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
	}), nil
}

// newListWatch returns a new ListWatch object that can be used to create a SharedIndexInformer.
func createStructuredListWatch(gvk schema.GroupVersionKind, ip *specificInformersMap, optionsModifier listOptionsModifier) (*cache.ListWatch, error) {
	// Kubernetes APIs work against Resources, not GroupVersionKinds.  Map the
	// groupVersionKind to the Resource API we will use.
	mapping, err := ip.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, err
	}

	client, err := apiutil.RESTClientForGVK(gvk, ip.config, ip.codecs)
	if err != nil {
		return nil, err
	}
	listGVK := gvk.GroupVersion().WithKind(gvk.Kind + "List")
	listObj, err := ip.Scheme.New(listGVK)
	if err != nil {
		return nil, err
	}

	// TODO: the functions that make use of this ListWatch should be adapted to
	//  pass in their own contexts instead of relying on this fixed one here.
	ctx := context.TODO()
	// Create a new ListWatch for the obj
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			optionsModifier(&opts)
			res := listObj.DeepCopyObject()
			isNamespaceScoped := ip.namespace != "" && mapping.Scope.Name() != meta.RESTScopeNameRoot
			err := client.Get().NamespaceIfScoped(ip.namespace, isNamespaceScoped).Resource(mapping.Resource.Resource).VersionedParams(&opts, ip.paramCodec).Do(ctx).Into(res)
			return res, err
		},
		// Setup the watch function
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			optionsModifier(&opts)
			// Watch needs to be set to true separately
			opts.Watch = true
			isNamespaceScoped := ip.namespace != "" && mapping.Scope.Name() != meta.RESTScopeNameRoot
			return client.Get().NamespaceIfScoped(ip.namespace, isNamespaceScoped).Resource(mapping.Resource.Resource).VersionedParams(&opts, ip.paramCodec).Watch(ctx)
		},
	}, nil
}

func createUnstructuredListWatch(gvk schema.GroupVersionKind, ip *specificInformersMap, optionsModifier listOptionsModifier) (*cache.ListWatch, error) {
	// Kubernetes APIs work against Resources, not GroupVersionKinds.  Map the
	// groupVersionKind to the Resource API we will use.
	mapping, err := ip.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, err
	}
	dynamicClient, err := dynamic.NewForConfig(ip.config)
	if err != nil {
		return nil, err
	}

	// TODO: the functions that make use of this ListWatch should be adapted to
	//  pass in their own contexts instead of relying on this fixed one here.
	ctx := context.TODO()
	// Create a new ListWatch for the obj
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			optionsModifier(&opts)
			if ip.namespace != "" && mapping.Scope.Name() != meta.RESTScopeNameRoot {
				return dynamicClient.Resource(mapping.Resource).Namespace(ip.namespace).List(ctx, opts)
			}
			return dynamicClient.Resource(mapping.Resource).List(ctx, opts)
		},
		// Setup the watch function
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			optionsModifier(&opts)
			// Watch needs to be set to true separately
			opts.Watch = true
			if ip.namespace != "" && mapping.Scope.Name() != meta.RESTScopeNameRoot {
				return dynamicClient.Resource(mapping.Resource).Namespace(ip.namespace).Watch(ctx, opts)
			}
			return dynamicClient.Resource(mapping.Resource).Watch(ctx, opts)
		},
	}, nil
}

// resyncPeriod returns a function which generates a duration each time it is
// invoked; this is so that multiple controllers don't get into lock-step and all
// hammer the apiserver with list requests simultaneously.
func resyncPeriod(resync time.Duration) func() time.Duration {
	return func() time.Duration {
		// the factor will fall into [0.9, 1.1)
		factor := rand.Float64()/5.0 + 0.9
		return time.Duration(float64(resync.Nanoseconds()) * factor)
	}
}
