/*
Copyright 2020 The Kubernetes Authors.

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

package clusterconnector

import (
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	logf "sigs.k8s.io/controller-runtime/pkg/internal/log"
	internalrecorder "sigs.k8s.io/controller-runtime/pkg/internal/recorder"
	"sigs.k8s.io/controller-runtime/pkg/manager/runner"
	"sigs.k8s.io/controller-runtime/pkg/recorder"
)

// ClusterConnector contains everything thats needed to build a controller
// that watches and interacts with objects in the given cluster.
type ClusterConnector interface {
	// SetFields will set any dependencies on an object for which the object has implemented the inject
	// interface - e.g. inject.Client.
	SetFields(interface{}) error

	// GetConfig returns an initialized Config
	GetConfig() *rest.Config

	// GetScheme returns an initialized Scheme
	GetScheme() *runtime.Scheme

	// GetClient returns a client configured with the Config. This client may
	// not be a fully "direct" client -- it may read from a cache, for
	// instance.  See Options.NewClient for more information on how the default
	// implementation works.
	GetClient() client.Client

	// GetFieldIndexer returns a client.FieldIndexer configured with the client
	GetFieldIndexer() client.FieldIndexer

	// GetCache returns a cache.Cache
	GetCache() cache.Cache

	// GetEventRecorderFor returns a new EventRecorder for the provided name
	GetEventRecorderFor(name string) record.EventRecorder

	// GetRESTMapper returns a RESTMapper
	GetRESTMapper() meta.RESTMapper

	// GetAPIReader returns a reader that will be configured to use the API server.
	// This should be used sparingly and only when the client does not fit your
	// use case.
	GetAPIReader() client.Reader

	// AddToManager adds the ClusterConnector to a manager
	AddToManager(mgr Manager) error
}

// NewClientFunc allows a user to define how to create a client
type NewClientFunc func(cache cache.Cache, config *rest.Config, options client.Options) (client.Client, error)

// DefaultNewClient creates the default caching client
func DefaultNewClient(cache cache.Cache, config *rest.Config, options client.Options) (client.Client, error) {
	// Create the Client for Write operations.
	c, err := client.New(config, options)
	if err != nil {
		return nil, err
	}

	return &client.DelegatingClient{
		Reader: &client.DelegatingReader{
			CacheReader:  cache,
			ClientReader: c,
		},
		Writer:       c,
		StatusClient: c,
	}, nil
}

// Options holds all possible Options for a ClusterConnector
type Options struct {
	// Scheme is the scheme used to resolve runtime.Objects to GroupVersionKinds / Resources
	// Defaults to the kubernetes/client-go scheme.Scheme, but it's almost always better
	// idea to pass your own scheme in.  See the documentation in pkg/scheme for more information.
	Scheme *runtime.Scheme

	// MapperProvider provides the rest mapper used to map go types to Kubernetes APIs
	MapperProvider func(c *rest.Config) (meta.RESTMapper, error)

	// SyncPeriod determines the minimum frequency at which watched resources are
	// reconciled. A lower period will correct entropy more quickly, but reduce
	// responsiveness to change if there are many watched resources. Change this
	// value only if you know what you are doing. Defaults to 10 hours if unset.
	// there will a 10 percent jitter between the SyncPeriod of all controllers
	// so that all controllers will not send list requests simultaneously.
	SyncPeriod *time.Duration

	// NewCache is the function that will create the cache to be used
	// by the manager. If not set this will use the default new cache function.
	NewCache cache.NewCacheFunc

	// NewClient will create the client to be used by the manager.
	// If not set this will create the default DelegatingClient that will
	// use the cache for reads and the client for writes.
	NewClient NewClientFunc

	// DryRunClient specifies whether the client should be configured to enforce
	// dryRun mode.
	DryRunClient *bool

	// Namespace if specified restricts the manager's cache to watch objects in
	// the desired namespace Defaults to all namespaces
	//
	// Note: If a namespace is specified, controllers can still Watch for a
	// cluster-scoped resource (e.g Node).  For namespaced resources the cache
	// will only hold objects from the desired namespace.
	Namespace *string

	// EventBroadcaster records Events emitted by the manager and sends them to the Kubernetes API
	// Use this to customize the event correlator and spam filter
	EventBroadcaster record.EventBroadcaster

	// Dependency injection for testing
	newRecorderProvider func(config *rest.Config, scheme *runtime.Scheme, logger logr.Logger, broadcaster record.EventBroadcaster) (recorder.Provider, error)
}

// Apply implements Option
func (o Options) Apply(target *Options) {
	if o.Scheme != nil {
		target.Scheme = o.Scheme
	}
	if o.MapperProvider != nil {
		target.MapperProvider = o.MapperProvider
	}
	if o.SyncPeriod != nil {
		target.SyncPeriod = o.SyncPeriod
	}
	if o.NewCache != nil {
		target.NewCache = o.NewCache
	}
	if o.NewClient != nil {
		target.NewClient = o.NewClient
	}

	if o.DryRunClient != nil {
		target.DryRunClient = o.DryRunClient
	}

	if o.Namespace != nil {
		target.Namespace = o.Namespace
	}

	if o.EventBroadcaster != nil {
		target.EventBroadcaster = o.EventBroadcaster
	}

	if o.newRecorderProvider != nil {
		target.newRecorderProvider = o.newRecorderProvider
	}
}

var _ Option = Options{}

// Option can be used to customize a ClusterConnector
type Option interface {
	Apply(o *Options)
}

// setOptionsDefaults set default values for Options fields
func setOptionsDefaults(options Options) Options {
	// Use the Kubernetes client-go scheme if none is specified
	if options.Scheme == nil {
		options.Scheme = scheme.Scheme
	}

	if options.MapperProvider == nil {
		options.MapperProvider = func(c *rest.Config) (meta.RESTMapper, error) {
			return apiutil.NewDynamicRESTMapper(c)
		}
	}

	// Allow newClient to be mocked
	if options.NewClient == nil {
		options.NewClient = DefaultNewClient
	}

	// Allow newCache to be mocked
	if options.NewCache == nil {
		options.NewCache = cache.New
	}

	if options.DryRunClient == nil {
		var b bool
		options.DryRunClient = &b
	}

	if options.Namespace == nil {
		var s string
		options.Namespace = &s
	}

	// Allow newRecorderProvider to be mocked
	if options.newRecorderProvider == nil {
		options.newRecorderProvider = internalrecorder.NewProvider
	}

	if options.EventBroadcaster == nil {
		options.EventBroadcaster = record.NewBroadcaster()
	}

	return options
}

// Manager manages the lifecycle of Runnables
type Manager interface {
	Add(r runner.Runnable) error
}

// New creates a new ClusterConnector
func New(config *rest.Config, mgr Manager, name string, opts ...Option) (ClusterConnector, error) {
	cc, err := NewUnmanaged(config, name, opts...)
	if err != nil {
		return nil, err
	}

	if err := cc.AddToManager(mgr); err != nil {
		return nil, err
	}

	return cc, nil
}

// NewUnmanaged creates a new unmanaged ClusterConnector. It must be manually added to a
// Manager by calling its AddToManager.
func NewUnmanaged(config *rest.Config, name string, opts ...Option) (ClusterConnector, error) {
	log := logf.RuntimeLog.WithName("clusterconnector").WithValues("name", name)
	if config == nil {
		return nil, fmt.Errorf("must specify Config")
	}

	options := Options{}
	for _, opt := range opts {
		opt.Apply(&options)
	}

	// Set default values for options fields
	options = setOptionsDefaults(options)

	// Create the mapper provider
	mapper, err := options.MapperProvider(config)
	if err != nil {
		log.Error(err, "Failed to get API Group-Resources")
		return nil, err
	}

	// Create the cache for the cached read client and registering informers
	cache, err := options.NewCache(config, cache.Options{
		Scheme:    options.Scheme,
		Mapper:    mapper,
		Resync:    options.SyncPeriod,
		Namespace: *options.Namespace},
	)
	if err != nil {
		return nil, err
	}

	apiReader, err := client.New(config, client.Options{Scheme: options.Scheme, Mapper: mapper})
	if err != nil {
		return nil, err
	}

	writeObj, err := options.NewClient(cache, config, client.Options{Scheme: options.Scheme, Mapper: mapper})
	if err != nil {
		return nil, err
	}

	if *options.DryRunClient {
		writeObj = client.NewDryRunClient(writeObj)
	}

	// Create the recorder provider to inject event recorders for the components.
	// TODO(directxman12): the log for the event provider should have a context (name, tags, etc) specific
	// to the particular controller that it's being injected into, rather than a generic one like is here.
	recorderProvider, err := options.newRecorderProvider(config, options.Scheme, log.WithName("events"), options.EventBroadcaster)
	if err != nil {
		return nil, err
	}

	return &clusterConnector{
		config:           config,
		scheme:           options.Scheme,
		cache:            cache,
		fieldIndexes:     cache,
		client:           writeObj,
		apiReader:        apiReader,
		recorderProvider: recorderProvider,
		mapper:           mapper,
	}, nil

}
