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

package manager

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	internalrecorder "sigs.k8s.io/controller-runtime/pkg/internal/recorder"
	"sigs.k8s.io/controller-runtime/pkg/recorder"
)

// Manager initializes shared dependencies such as Caches and Clients, and provides them to runnables.
type Manager interface {
	// Add will set reqeusted dependencies on the component, and cause the component to be
	// started when Start is called.  Add will inject any dependencies for which the argument
	// implements the inject interface - e.g. inject.Client
	Add(Runnable) error

	// SetFields will set any dependencies on an object for which the object has implemented the inject
	// interface - e.g. inject.Client.
	SetFields(interface{}) error

	// Start starts all registered Controllers and blocks until the Stop channel is closed.
	// Returns an error if there is an error starting any controller.
	Start(<-chan struct{}) error

	// GetConfig returns an initialized Config
	GetConfig() *rest.Config

	// GetScheme returns and initialized Scheme
	GetScheme() *runtime.Scheme

	// GetClient returns a client configured with the Config
	GetClient() client.Client

	// GetFieldIndexer returns a client.FieldIndexer configured with the client
	GetFieldIndexer() client.FieldIndexer

	// GetCache returns a cache.Cache
	GetCache() cache.Cache

	// GetRecorder returns a new EventRecorder for the provided name
	GetRecorder(name string) record.EventRecorder
}

// Options are the arguments for creating a new Manager
type Options struct {
	// Scheme is the scheme used to resolve runtime.Objects to GroupVersionKinds / Resources
	// Defaults to the kubernetes/client-go scheme.Scheme
	Scheme *runtime.Scheme

	// Mapper is the rest mapper used to map go types to Kubernetes APIs
	MapperProvider func(c *rest.Config) (meta.RESTMapper, error)

	// Dependency injection for testing
	newCache            func(config *rest.Config, opts cache.Options) (cache.Cache, error)
	newClient           func(config *rest.Config, options client.Options) (client.Client, error)
	newRecorderProvider func(config *rest.Config, scheme *runtime.Scheme) (recorder.Provider, error)
}

// Runnable allows a component to be started.
type Runnable interface {
	// Start starts running the component.  The component will stop running when the channel is closed.
	// Start blocks until the channel is closed or an error occurs.
	Start(<-chan struct{}) error
}

// RunnableFunc implements Runnable
type RunnableFunc func(<-chan struct{}) error

// Start implements Runnable
func (r RunnableFunc) Start(s <-chan struct{}) error {
	return r(s)
}

// New returns a new Manager
func New(config *rest.Config, options Options) (Manager, error) {
	cm := &controllerManager{config: config, scheme: options.Scheme, errChan: make(chan error)}

	// Initialize a rest.config if none was specified
	if cm.config == nil {
		return nil, fmt.Errorf("must specify Config")
	}

	// Use the Kubernetes client-go scheme if none is specified
	if cm.scheme == nil {
		cm.scheme = scheme.Scheme
	}

	// Set default values for options fields
	options = setOptionsDefaults(options)

	mapper, err := options.MapperProvider(cm.config)
	if err != nil {
		log.Error(err, "Failed to get API Group-Resources")
		return nil, err
	}

	// Create the Client for Write operations.
	writeObj, err := options.newClient(cm.config, client.Options{Scheme: cm.scheme, Mapper: mapper})
	if err != nil {
		return nil, err
	}

	cm.cache, err = options.newCache(cm.config, cache.Options{Scheme: cm.scheme, Mapper: mapper})
	if err != nil {
		return nil, err
	}

	cm.fieldIndexes = cm.cache
	cm.client = client.DelegatingClient{Reader: cm.cache, Writer: writeObj}

	// Create the recorder provider to inject event recorders for the components.
	cm.recorderProvider, err = options.newRecorderProvider(cm.config, cm.scheme)
	if err != nil {
		return nil, err
	}

	return cm, nil
}

// setOptionsDefaults set default values for Options fields
func setOptionsDefaults(options Options) Options {
	if options.MapperProvider == nil {
		options.MapperProvider = apiutil.NewDiscoveryRESTMapper
	}

	// Allow newClient to be mocked
	if options.newClient == nil {
		options.newClient = client.New
	}

	// Allow newCache to be mocked
	// TODO(directxman12): Figure out how to allow users to request a client without requesting a watch
	if options.newCache == nil {
		options.newCache = cache.New
	}

	// Allow newRecorderProvider to be mocked
	if options.newRecorderProvider == nil {
		options.newRecorderProvider = internalrecorder.NewProvider
	}

	return options
}
