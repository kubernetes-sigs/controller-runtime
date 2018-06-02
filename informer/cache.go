package informer

import (
	"sync"
	"fmt"
	"time"

	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/rest"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	serializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/api/meta"
	watch "k8s.io/apimachinery/pkg/watch"
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

// indexInformerCache lazily creates informers, and then caches them for the next time that informer is
// requested.  It uses a standard parameter codec constructed based on the given generated scheme.
type indexInformerCache struct {
	informersByGVK map[schema.GroupVersionKind]cache.SharedIndexInformer
	mu sync.Mutex
	config *rest.Config
	scheme *runtime.Scheme
	codecs serializer.CodecFactory
	paramCodec runtime.ParameterCodec
	mapper meta.RESTMapper
}

// NewInformerCache creates a new IndexInformerCache with clients based on the given base config.
// The RESTMapper is used to convert kinds to resources.  It uses the given scheme to convert between types
// and kinds.
func NewInformerCache(mapper meta.RESTMapper, baseConfig *rest.Config, scheme *runtime.Scheme) IndexInformerCache {
	return &indexInformerCache{
		informersByGVK: make(map[schema.GroupVersionKind]cache.SharedIndexInformer),
		config: baseConfig,
		scheme: scheme,
		codecs: serializer.NewCodecFactory(scheme),
		paramCodec: runtime.NewParameterCodec(scheme),
		mapper: mapper,
	}
}

func (c *indexInformerCache) InformerForKind(gvk schema.GroupVersionKind) (cache.SharedIndexInformer, error) {
	obj, err := c.scheme.New(gvk)
	if err != nil {
		return nil, err
	}
	return c.informerFor(gvk, obj)
}

func (c *indexInformerCache) InformerFor(obj runtime.Object) (cache.SharedIndexInformer, error) {
	gvks, isUnversioned, err := c.scheme.ObjectKinds(obj)
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
		return nil, fmt.Errorf("multiple group-version-kinds associated with type %T, refusing to guess at one")
	}
	gvk := gvks[0]

	return c.informerFor(gvk, obj)
}

// informerFor actually fetches or constructs an informer.
func (c *indexInformerCache) informerFor(gvk schema.GroupVersionKind, obj runtime.Object) (cache.SharedIndexInformer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()


	informer, ok := c.informersByGVK[gvk]
	if ok {
		return informer, nil
	}

	gv := gvk.GroupVersion()

	cfg := rest.CopyConfig(c.config)
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

	listGVK := gvk.GroupVersion().WithKind(gvk.Kind+"List")
	listObj, err := c.scheme.New(listGVK)
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

func (c *indexInformerCache) Start(stopCh <-chan struct{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, informer := range c.informersByGVK {
		go informer.Run(stopCh)
	}
	return nil
}
