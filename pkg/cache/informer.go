package cache

import (
	"sync"
	"time"

	"reflect"

	"github.com/kubernetes-sigs/controller-runtime/pkg/client"
	"github.com/kubernetes-sigs/controller-runtime/pkg/client/apiutil"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// Cache implements ReadInterface by reading objects from a cache populated by Informers
type Cache interface {
	client.ReadInterface
	Informers
	IndexField(obj runtime.Object, field string, extractValue client.IndexerFunc) error
}

// Informers knows how to create or fetch informers for different group-version-kinds.
// It's safe to call GetInformer from multiple threads.
type Informers interface {
	// GetInformer fetches or constructs an informer for the given object that corresponds to a single
	// API kind and resource.
	GetInformer(obj runtime.Object) (cache.SharedIndexInformer, error)

	// GetInformerForKind is similar to GetInformer, except that it takes a group-version-kind, instead
	// of the underlying object.
	GetInformerForKind(gvk schema.GroupVersionKind) (cache.SharedIndexInformer, error)

	// Start runs all the informers known to this cache until the given channel is closed.
	// It does not block.
	Start(stopCh <-chan struct{}) error

	KnownInformersByType() map[schema.GroupVersionKind]cache.SharedIndexInformer
}

// Options are the optional arguments for creating a new Informers object
type Options struct {
	// Scheme is the scheme to use for mapping objects to GroupVersionKinds
	Scheme *runtime.Scheme

	// Mapper is the RESTMapper to use for mapping GroupVersionKinds to Resources
	Mapper meta.RESTMapper

	// Resync is the resync period
	Resync *time.Duration
}

// New initializes and returns a new Informers object
func New(config *rest.Config, opts Options) (Cache, error) {
	// Use the default Kubernetes Scheme if unset
	if opts.Scheme == nil {
		opts.Scheme = scheme.Scheme
	}

	// Construct a new Mapper if unset
	if opts.Mapper == nil {
		var err error
		opts.Mapper, err = apiutil.NewDiscoveryRESTMapper(config)
		if err != nil {
			log.WithName("setup").Error(err, "Failed to get API Group-Resources")
			return nil, err
		}
	}

	// Default the resync period to 10 hours if unset
	if opts.Resync == nil {
		r := 10 * time.Hour
		opts.Resync = &r
	}

	i := &informers{
		objectCache: objectCache{
			cachesByType: make(map[reflect.Type]*singleObjectCache),
			scheme:       opts.Scheme,
		},
		mapper:         opts.Mapper,
		config:         config,
		scheme:         opts.Scheme,
		codecs:         serializer.NewCodecFactory(opts.Scheme),
		paramCodec:     runtime.NewParameterCodec(opts.Scheme),
		informersByGVK: make(map[schema.GroupVersionKind]cache.SharedIndexInformer),
		resync:         *opts.Resync,
	}
	i.objectCache.informers = i

	return i, nil
}

// informers lazily creates informers, and then caches them for the next time that informer is
// requested.  It uses a standard parameter codec constructed based on the given generated scheme.
type informers struct {
	objectCache

	mapper meta.RESTMapper
	scheme *runtime.Scheme
	config *rest.Config

	mu             sync.Mutex
	informersByGVK map[schema.GroupVersionKind]cache.SharedIndexInformer
	codecs         serializer.CodecFactory
	paramCodec     runtime.ParameterCodec
	started        bool
	stop           <-chan struct{}
	resync         time.Duration
}

func (c *informers) KnownInformersByType() map[schema.GroupVersionKind]cache.SharedIndexInformer {
	return c.informersByGVK
}

func (c *informers) GetInformerForKind(gvk schema.GroupVersionKind) (cache.SharedIndexInformer, error) {
	// Map the gvk to an object
	obj, err := c.scheme.New(gvk)
	if err != nil {
		return nil, err
	}
	return c.informerFor(gvk, obj)
}

func (c *informers) GetInformer(obj runtime.Object) (cache.SharedIndexInformer, error) {
	gvk, err := apiutil.GVKForObject(obj, c.scheme)
	if err != nil {
		return nil, err
	}
	return c.informerFor(gvk, obj)
}

// informerFor will create a new informer and added it to the map of informers if none exists.  Returns
// the informer from the map.
func (c *informers) informerFor(gvk schema.GroupVersionKind, obj runtime.Object) (cache.SharedIndexInformer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check the cache to see if we already have an informer.  If we do, return the informer.
	informer, ok := c.informersByGVK[gvk]
	if ok {
		return informer, nil
	}

	// Get a new ListWatch that will be used to create the SharedIndexInformer
	lw, err := c.newListWatcher(gvk)
	if err != nil {
		return nil, err
	}

	// Create a NewSharedIndexInformer and add it to the map.
	c.informersByGVK[gvk] = cache.NewSharedIndexInformer(lw, obj, c.resync, cache.Indexers{})
	in := c.informersByGVK[gvk]

	// Update the cache client with the informer.
	c.objectCache.addInformer(gvk, in)

	// If we are already running, then immediately start the informer.  Otherwise wait until Start is called
	// so that we have the stop Channel available.
	if c.started {
		go in.Run(c.stop)
	}

	return in, nil
}

// newListWatcher returns a new ListWatch object that can be used to create a SharedIndexInformer.
func (c *informers) newListWatcher(gvk schema.GroupVersionKind) (*cache.ListWatch, error) {
	// Construct a RESTClient for the GroupVersionKind that we will use to
	// talk to the apiserver.
	client, err := apiutil.RESTClientForGVK(gvk, c.config, c.codecs)
	if err != nil {
		return nil, err
	}

	// Kubernetes APIs work against Resources, not GroupVersionKinds.  Map the
	// GroupVersionKind to the Resource API we will use.
	mapping, err := c.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, err
	}

	// Get a listObject for listing that the ListWatch can DeepCopy
	listGVK := gvk.GroupVersion().WithKind(gvk.Kind + "List")
	listObj, err := c.scheme.New(listGVK)
	if err != nil {
		return nil, err
	}

	// Create a new ListWatch for the obj
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			res := listObj.DeepCopyObject()
			err := client.Get().
				Resource(mapping.Resource).
				VersionedParams(&opts, c.paramCodec).
				Do().
				Into(res)
			return res, err
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			// Setup the watch function
			opts.Watch = true
			return client.Get().
				Resource(mapping.Resource).
				VersionedParams(&opts, c.paramCodec).
				Watch()
		},
	}, nil
}

// Start calls Run on each of the informers and sets started to true.  Blocks on the stop channel.
func (c *informers) Start(stop <-chan struct{}) error {
	func() {
		c.mu.Lock()
		defer c.mu.Unlock()

		// Set the stop channel so it can be passed to informers that are added later
		c.stop = stop

		// Start each informer
		for _, informer := range c.informersByGVK {
			go informer.Run(stop)
		}

		// Set started to true so we immediately start any informers added later.
		c.started = true
	}()
	<-stop
	return nil
}
