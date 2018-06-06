package client

import (
	"reflect"
	"sync"
	"context"

	"k8s.io/client-go/rest"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/api/meta"
	serializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/common"
)

var _ Interface = &Client{}

// Client is an Interface that works by reading and writing
// directly from/to an API server.
type Client struct {
	Config *rest.Config
	Scheme *runtime.Scheme
	Mapper meta.RESTMapper

	once           sync.Once
	codecs         serializer.CodecFactory
	paramCodec     runtime.ParameterCodec
	clientsByType  map[reflect.Type]rest.Interface
	resourcesByType map[reflect.Type]string
	mu             sync.RWMutex
}

// TODO: pass discovery info down from controller manager

func (c *Client) init() {
	c.once.Do(func() {
		// Init a scheme if none provided
		if c.Scheme == nil {
			c.Scheme = scheme.Scheme
		}

		// Setup the codecs
		c.codecs = serializer.NewCodecFactory(c.Scheme)
		c.paramCodec = runtime.NewParameterCodec(c.Scheme)
		c.clientsByType = make(map[reflect.Type]rest.Interface)
	})
}

func (c *Client) makeClient(obj runtime.Object) (rest.Interface, string, error) {
	gvk, err := common.GVKForObject(obj, c.Scheme)
	if err != nil {
		return nil, "", err
	}
	client, err := common.RESTClientForGVK(gvk, c.Config, c.codecs)
	if err != nil {
		return nil, "", err
	}
	mapping, err := c.Mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, "", err
	}
	return client, mapping.Resource, nil
}

// ClientFor returns a raw rest.Interface for the given object type.
func (c *Client) clientFor(obj runtime.Object) (rest.Interface, string, error) {
	c.init()
	typ := reflect.TypeOf(obj)

	// It's better to do creation work twice than to not let multiple
	// people make requests at once
	c.mu.RLock()
	client, known := c.clientsByType[typ]
	resource, _ := c.resourcesByType[typ]
	c.mu.RUnlock()

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

func (c *Client) Create(ctx context.Context, obj runtime.Object) error {
	client, resource, err := c.clientFor(obj)
	if err != nil {
		return err
	}
	meta, err := meta.Accessor(obj)
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

func (c *Client) Update(ctx context.Context, obj runtime.Object) error {
	client, resource, err := c.clientFor(obj)
	if err != nil {
		return err
	}
	meta, err := meta.Accessor(obj)
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

func (c *Client) Delete(ctx context.Context, obj runtime.Object) error {
	client, resource, err := c.clientFor(obj)
	if err != nil {
		return err
	}
	meta, err := meta.Accessor(obj)
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

func (c *Client) Get(ctx context.Context, key ObjectKey, obj runtime.Object) error {
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

func (c *Client) List(ctx context.Context, opts *ListOptions, obj runtime.Object) error {
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
