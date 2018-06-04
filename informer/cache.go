package informer

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/kubernetes-sigs/kubebuilder/pkg/config"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	serializer "k8s.io/apimachinery/pkg/runtime/serializer"
	watch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// IndexInformerCache knows how to create or fetch informers for different group-version-kinds.
// It's safe to call InformerFor from multiple threads.
type IndexInformerCache interface {
	// InformerFor fetches or constructs an informer for the given object that corresponds to a single
	// API kind and resource.
	InformerFor(obj runtime.Object) (cache.SharedIndexInformer, error)
	// InformerForKind is similar to InformerFor, except that it takes a group-version-kind, instead
	// of the underlying object.
	InformerForKind(gvk schema.GroupVersionKind) (cache.SharedIndexInformer, error)
	// Start runs all the informers known to this cache until the given channel is closed.
	// It does not block.
	Start(stopCh <-chan struct{}) error
}

func NewInformerCacheOrDie(config *rest.Config) IndexInformerCache {
	discoveryClient := discovery.NewDiscoveryClientForConfigOrDie(config)
	groupResources, err := discovery.GetAPIGroupResources(discoveryClient)
	if err != nil {
		log.Fatalf("Could not get API GroupResoures %v", err)
	}
	discoMapper := discovery.NewRESTMapper(groupResources, dynamic.VersionInterfaces)

	return NewInformerCache(discoMapper, config, scheme.Scheme)
}

var _ IndexInformerCache = &IndexedCache{}

// IndexedCache lazily creates informers, and then caches them for the next time that informer is
// requested.  It uses a standard parameter codec constructed based on the given generated Scheme.
type IndexedCache struct {
	Config *rest.Config
	Scheme *runtime.Scheme
	mapper meta.RESTMapper

	once           sync.Once
	mu             sync.Mutex
	informersByGVK map[schema.GroupVersionKind]cache.SharedIndexInformer
	codecs         serializer.CodecFactory
	paramCodec     runtime.ParameterCodec
}

func (c *IndexedCache) init() {
	c.once.Do(func() {
		// TODO: Make Start return an error instead of dying on errors

		// Init a config if none provided
		if c.Config == nil {
			c.Config = config.GetConfigOrDie()
		}

		// Init a scheme if none provided
		if c.Scheme == nil {
			c.Scheme = scheme.Scheme
		}

		// Get a mapper
		dc := discovery.NewDiscoveryClientForConfigOrDie(c.Config)
		gr, err := discovery.GetAPIGroupResources(dc)
		if err != nil {
			log.Fatalf("Failed to get API Group Resources: %v", err)
		}
		c.mapper = discovery.NewRESTMapper(gr, dynamic.VersionInterfaces)

		// Setup the codecs
		c.codecs = serializer.NewCodecFactory(c.Scheme)
		c.paramCodec = runtime.NewParameterCodec(c.Scheme)
		c.informersByGVK = make(map[schema.GroupVersionKind]cache.SharedIndexInformer)
	})
}

// NewInformerCache creates a new IndexInformerCache with clients based on the given base config.
// The RESTMapper is used to convert kinds to resources.  It uses the given Scheme to convert between types
// and kinds.
func NewInformerCache(mapper meta.RESTMapper, baseConfig *rest.Config, scheme *runtime.Scheme) IndexInformerCache {
	return &IndexedCache{
		informersByGVK: make(map[schema.GroupVersionKind]cache.SharedIndexInformer),
		Config:         baseConfig,
		Scheme:         scheme,
		codecs:         serializer.NewCodecFactory(scheme),
		paramCodec:     runtime.NewParameterCodec(scheme),
		mapper:         mapper,
	}
}

func (c *IndexedCache) InformerForKind(gvk schema.GroupVersionKind) (cache.SharedIndexInformer, error) {
	c.init()
	obj, err := c.Scheme.New(gvk)
	if err != nil {
		return nil, err
	}
	return c.informerFor(gvk, obj)
}

func (c *IndexedCache) InformerFor(obj runtime.Object) (cache.SharedIndexInformer, error) {
	c.init()
	gvks, isUnversioned, err := c.Scheme.ObjectKinds(obj)
	if err != nil {
		return nil, err
	}
	if isUnversioned {
		return nil, fmt.Errorf("cannot create a new informer for the unversioned type %T", obj)
	}

	if len(gvks) < 1 {
		return nil, fmt.Errorf("no group-version-kinds associated with type %T", obj)
	}
	if len(gvks) > 1 {
		// this should only trigger for things like metav1.XYZ --
		// normal versioned types should be fine
		return nil, fmt.Errorf(
			"multiple group-version-kinds associated with type %T, refusing to guess at one", obj)
	}
	gvk := gvks[0]

	return c.informerFor(gvk, obj)
}

// informerFor actually fetches or constructs an informer.
func (c *IndexedCache) informerFor(gvk schema.GroupVersionKind, obj runtime.Object) (cache.SharedIndexInformer, error) {
	c.init()
	c.mu.Lock()
	defer c.mu.Unlock()

	informer, ok := c.informersByGVK[gvk]
	if ok {
		return informer, nil
	}

	gv := gvk.GroupVersion()

	cfg := rest.CopyConfig(c.Config)
	cfg.GroupVersion = &gv
	if gvk.Group == "" {
		cfg.APIPath = "/api"
	} else {
		cfg.APIPath = "/apis"
	}
	cfg.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: c.codecs}
	if cfg.UserAgent == "" {
		cfg.UserAgent = rest.DefaultKubernetesUserAgent()
	}
	client, err := rest.RESTClientFor(cfg)
	if err != nil {
		return nil, err
	}

	mapping, err := c.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, err
	}

	listGVK := gvk.GroupVersion().WithKind(gvk.Kind + "List")
	listObj, err := c.Scheme.New(listGVK)
	if err != nil {
		return nil, err
	}

	lw := &cache.ListWatch{
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
			opts.Watch = true
			return client.Get().
				Resource(mapping.Resource).
				VersionedParams(&opts, c.paramCodec).
				Watch()
		},
	}

	res := cache.NewSharedIndexInformer(
		lw, obj, 10*time.Hour, cache.Indexers{},
	)

	c.informersByGVK[gvk] = res
	return res, nil
}

func (c *IndexedCache) Start(stopCh <-chan struct{}) error {
	c.init()
	c.mu.Lock()
	defer c.mu.Unlock()

	// TODO: Start new informers automatically when they are created if Start has already been called.
	for _, informer := range c.informersByGVK {
		//fmt.Printf("Hello world %v\n\n")
		go informer.Run(stopCh)
	}
	return nil
}
