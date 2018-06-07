package informer

import (
	"os"
	"sync"
	"time"

	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/common"
	logf "github.com/kubernetes-sigs/kubebuilder/pkg/log"
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

var log = logf.KBLog.WithName("informers")

// Informers knows how to create or fetch informers for different group-version-kinds.
// It's safe to call InformerFor from multiple threads.
type Informers interface {
	// InformerFor fetches or constructs an informer for the given object that corresponds to a single
	// API kind and resource.
	InformerFor(obj runtime.Object) (cache.SharedIndexInformer, error)
	// InformerForKind is similar to InformerFor, except that it takes a group-version-kind, instead
	// of the underlying object.
	InformerForKind(gvk schema.GroupVersionKind) (cache.SharedIndexInformer, error)
	// KnownInformersByType returns all informers requested.
	KnownInformersByType() map[schema.GroupVersionKind]cache.SharedIndexInformer
	// Start runs all the informers known to this cache until the given channel is closed.
	// It does not block.
	Start(stopCh <-chan struct{}) error
}

var _ Informers = &SelfPopulatingInformers{}

type InformerCallback interface {
	Call(gvk schema.GroupVersionKind, c cache.SharedIndexInformer)
}

// SelfPopulatingInformers lazily creates informers, and then caches them for the next time that informer is
// requested.  It uses a standard parameter codec constructed based on the given generated scheme.
type SelfPopulatingInformers struct {
	Config    *rest.Config
	Scheme    *runtime.Scheme
	Mapper    meta.RESTMapper
	Callbacks []InformerCallback

	once           sync.Once
	mu             sync.Mutex
	informersByGVK map[schema.GroupVersionKind]cache.SharedIndexInformer
	codecs         serializer.CodecFactory
	paramCodec     runtime.ParameterCodec
	started        bool
	stopCh         <-chan struct{}
}

func (c *SelfPopulatingInformers) init() {
	c.once.Do(func() {
		// Init a scheme if none provided
		if c.Scheme == nil {
			c.Scheme = scheme.Scheme
		}

		if c.Mapper == nil {
			// TODO: don't initialize things that can fail
			Mapper, err := common.NewDiscoveryRESTMapper(c.Config)
			if err != nil {
				log.WithName("setup").Error(err, "Failed to get API Group-Resources")
				os.Exit(1)
			}
			c.Mapper = Mapper
		}

		// Setup the codecs
		c.codecs = serializer.NewCodecFactory(c.Scheme)
		c.paramCodec = runtime.NewParameterCodec(c.Scheme)
		c.informersByGVK = make(map[schema.GroupVersionKind]cache.SharedIndexInformer)
	})
}

func (c *SelfPopulatingInformers) KnownInformersByType() map[schema.GroupVersionKind]cache.SharedIndexInformer {
	c.init()
	return c.informersByGVK
}

func (c *SelfPopulatingInformers) InformerForKind(gvk schema.GroupVersionKind) (cache.SharedIndexInformer, error) {
	c.init()
	obj, err := c.Scheme.New(gvk)
	if err != nil {
		return nil, err
	}
	return c.informerFor(gvk, obj)
}

func (c *SelfPopulatingInformers) InformerFor(obj runtime.Object) (cache.SharedIndexInformer, error) {
	c.init()
	gvk, err := common.GVKForObject(obj, c.Scheme)
	if err != nil {
		return nil, err
	}
	return c.informerFor(gvk, obj)
}

// informerFor actually fetches or constructs an informer.
func (c *SelfPopulatingInformers) informerFor(gvk schema.GroupVersionKind, obj runtime.Object) (cache.SharedIndexInformer, error) {
	c.init()
	c.mu.Lock()
	defer c.mu.Unlock()

	informer, ok := c.informersByGVK[gvk]
	if ok {
		for _, callback := range c.Callbacks {
			callback.Call(gvk, informer)
		}
		return informer, nil
	}

	client, err := common.RESTClientForGVK(gvk, c.Config, c.codecs)
	if err != nil {
		return nil, err
	}

	mapping, err := c.Mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
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

	// Callback that an informer was added
	for _, callback := range c.Callbacks {
		callback.Call(gvk, res)
	}

	// If we already started the informers, start new ones as they get added
	if c.started {
		go res.Run(c.stopCh)
	}
	return res, nil
}

func (c *SelfPopulatingInformers) Start(stopCh <-chan struct{}) error {
	c.init()
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stopCh = stopCh
	for _, informer := range c.informersByGVK {
		go informer.Run(stopCh)
	}

	c.started = true
	return nil
}
