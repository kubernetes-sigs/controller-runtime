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
	"time"

	"github.com/kubernetes-sigs/controller-runtime/pkg/cache/internal"
	"github.com/kubernetes-sigs/controller-runtime/pkg/client"
	"github.com/kubernetes-sigs/controller-runtime/pkg/client/apiutil"
	logf "github.com/kubernetes-sigs/controller-runtime/pkg/runtime/log"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
)

var log = logf.KBLog.WithName("object-cache")

// Cache implements ReadInterface by reading objects from a cache populated by Informers
type Cache interface {
	// Cache implements the client ReadInterface
	client.Reader

	// Cache implements Informers
	Informers
}

// Informers knows how to create or fetch informers for different group-version-kinds.
// It's safe to call GetInformer from multiple threads.
type Informers interface {
	// GetInformer fetches or constructs an informer for the given object that corresponds to a single
	// API kind and resource.
	GetInformer(obj runtime.Object) (toolscache.SharedIndexInformer, error)

	// GetInformerForKind is similar to GetInformer, except that it takes a group-version-kind, instead
	// of the underlying object.
	GetInformerForKind(gvk schema.GroupVersionKind) (toolscache.SharedIndexInformer, error)

	// Start runs all the informers known to this cache until the given channel is closed.
	// It does not block.
	Start(stopCh <-chan struct{}) error

	// WaitForCacheSync waits for all the caches to sync.  Returns false if it could not sync a cache.
	WaitForCacheSync(stop <-chan struct{}) bool

	// IndexField adds an index to a field.
	IndexField(obj runtime.Object, field string, extractValue client.IndexerFunc) error
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

var _ Informers = &cache{}
var _ client.Reader = &cache{}
var _ Cache = &cache{}

// cache is a Kubernetes Object cache populated from Informers.  cache wraps a CacheProvider and InformerProvider.
type cache struct {
	*internal.CacheProvider
	*internal.InformerProvider
}

// New initializes and returns a new Cache
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

	cp := internal.NewCacheProvider()
	ip := internal.NewInformerProvider(config, opts.Scheme, opts.Mapper, *opts.Resync, cp)
	cp.SetInformerProvider(ip)
	c := &cache{InformerProvider: ip, CacheProvider: cp}
	return c, nil
}
