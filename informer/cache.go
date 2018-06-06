package informer

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/kubernetes-sigs/kubebuilder/pkg/config"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/common"
	logf "github.com/kubernetes-sigs/kubebuilder/pkg/log"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	serializer "k8s.io/apimachinery/pkg/runtime/serializer"
	watch "k8s.io/apimachinery/pkg/watch"
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

// SelfPopulatingInformers lazily creates informers, and then caches them for the next time that informer is
// requested.  It uses a standard parameter codec constructed based on the given generated Scheme.
type SelfPopulatingInformers struct {
	Config *rest.Config
	Scheme *runtime.Scheme
	Mapper meta.RESTMapper

	once           sync.Once
	mu             sync.Mutex
	informersByGVK map[schema.GroupVersionKind]cache.SharedIndexInformer
	codecs         serializer.CodecFactory
	paramCodec     runtime.ParameterCodec
	started        bool
}

func (c *SelfPopulatingInformers) init() {
	c.once.Do(func() {
		// Init a config if none provided
		if c.Config == nil {
			c.Config = config.GetConfigOrDie()
		}

		// Init a scheme if none provided
		if c.Scheme == nil {
			c.Scheme = scheme.Scheme
		}

		if c.Mapper == nil {
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
func (c *SelfPopulatingInformers) informerFor(gvk schema.GroupVersionKind, obj runtime.Object) (cache.SharedIndexInformer, error) {
	c.init()
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return nil, fmt.Errorf("cannot create informers after informers have been started")
	}

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
	return res, nil
}

func (c *SelfPopulatingInformers) Start(stopCh <-chan struct{}) error {
	c.init()
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, informer := range c.informersByGVK {
		go informer.Run(stopCh)
	}

	c.started = true
	return nil
}
