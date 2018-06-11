package client

import (
	"context"
	"reflect"
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	serializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	"github.com/kubernetes-sigs/controller-runtime/pkg/client/apiutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Options are creation options for a Client
type Options struct {
	// Scheme, if provided, will be used to map go structs to GroupVersionKinds
	Scheme *runtime.Scheme

	// Mapper, if provided, will be used to map GroupVersionKinds to Resources
	Mapper meta.RESTMapper
}

// New returns a new Client using the provided config and Options.
func New(config *rest.Config, options Options) (Client, error) {
	// Init a scheme if none provided
	if options.Scheme == nil {
		options.Scheme = scheme.Scheme
	}

	// Init a Mapper if none provided
	if options.Mapper == nil {
		var err error
		options.Mapper, err = apiutil.NewDiscoveryRESTMapper(config)
		if err != nil {
			return nil, err
		}
	}

	c := &populatingClient{
		config:          config,
		scheme:          options.Scheme,
		mapper:          options.Mapper,
		codecs:          serializer.NewCodecFactory(options.Scheme),
		paramCodec:      runtime.NewParameterCodec(options.Scheme),
		clientsByType:   make(map[reflect.Type]rest.Interface),
		resourcesByType: make(map[reflect.Type]string),
	}

	return c, nil
}

var _ Client = &populatingClient{}

// populatingClient is an Client that reads and writes directly from/to an API server.  It lazily initialized
// new clients when they are used.
type populatingClient struct {
	config *rest.Config
	scheme *runtime.Scheme
	mapper meta.RESTMapper

	codecs          serializer.CodecFactory
	paramCodec      runtime.ParameterCodec
	clientsByType   map[reflect.Type]rest.Interface
	resourcesByType map[reflect.Type]string
	mu              sync.RWMutex
}

// makeClient maps obj to a Kubernetes Resource and constructs a populatingClient for that Resource.
func (c *populatingClient) makeClient(obj runtime.Object) (rest.Interface, string, error) {
	gvk, err := apiutil.GVKForObject(obj, c.scheme)
	if err != nil {
		return nil, "", err
	}
	client, err := apiutil.RESTClientForGVK(gvk, c.config, c.codecs)
	if err != nil {
		return nil, "", err
	}
	mapping, err := c.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, "", err
	}
	return client, mapping.Resource, nil
}

// clientFor returns a raw rest.Client for the given object type.
func (c *populatingClient) clientFor(obj runtime.Object) (rest.Interface, string, error) {
	typ := reflect.TypeOf(obj)

	// It's better to do creation work twice than to not let multiple
	// people make requests at once
	c.mu.RLock()
	client, known := c.clientsByType[typ]
	resource, _ := c.resourcesByType[typ]
	c.mu.RUnlock()

	// Initialize a new Client
	if !known {
		var err error
		client, resource, err = c.makeClient(obj)
		if err != nil {
			return nil, "", err
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		c.clientsByType[typ] = client
		c.resourcesByType[typ] = resource
	}

	return client, resource, nil
}

func (c *populatingClient) metaAndClientFor(obj runtime.Object) (v1.Object, rest.Interface, string, error) {
	client, resource, err := c.clientFor(obj)
	if err != nil {
		return nil, nil, "", err
	}
	meta, err := meta.Accessor(obj)
	if err != nil {
		return nil, nil, "", err
	}
	return meta, client, resource, err
}

// Create implements Client
func (c *populatingClient) Create(ctx context.Context, obj runtime.Object) error {
	meta, client, resource, err := c.metaAndClientFor(obj)
	if err != nil {
		return err
	}
	return client.Post().
		Namespace(meta.GetNamespace()).
		Resource(resource).
		Body(obj).
		Do().
		Into(obj)
}

// Update implements Client
func (c *populatingClient) Update(ctx context.Context, obj runtime.Object) error {
	meta, client, resource, err := c.metaAndClientFor(obj)
	if err != nil {
		return err
	}
	return client.Put().
		Namespace(meta.GetNamespace()).
		Resource(resource).
		Name(meta.GetName()).
		Body(obj).
		Do().
		Into(obj)
}

// Delete implements Client
func (c *populatingClient) Delete(ctx context.Context, obj runtime.Object) error {
	meta, client, resource, err := c.metaAndClientFor(obj)
	if err != nil {
		return err
	}
	return client.Delete().
		Namespace(meta.GetNamespace()).
		Resource(resource).
		Name(meta.GetName()).
		Do().
		Error()
}

// Get implements Client
func (c *populatingClient) Get(ctx context.Context, key ObjectKey, obj runtime.Object) error {
	client, resource, err := c.clientFor(obj)
	if err != nil {
		return err
	}
	return client.Get().
		Namespace(key.Namespace).
		Resource(resource).
		Name(key.Name).
		Do().
		Into(obj)
}

// List implements Client
func (c *populatingClient) List(ctx context.Context, opts *ListOptions, obj runtime.Object) error {
	client, resource, err := c.clientFor(obj)
	if err != nil {
		return err
	}
	ns := ""
	if opts != nil {
		ns = opts.Namespace
	}
	return client.Get().
		Namespace(ns).
		Resource(resource).
		Body(obj).
		VersionedParams(opts.AsListOptions(), c.paramCodec).
		Do().
		Into(obj)
}
