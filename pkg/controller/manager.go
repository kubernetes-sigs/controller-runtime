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

package controller

import (
	"fmt"

	"sync"

	"github.com/kubernetes-sigs/controller-runtime/pkg/cache"
	"github.com/kubernetes-sigs/controller-runtime/pkg/client"
	"github.com/kubernetes-sigs/controller-runtime/pkg/client/apiutil"
	"github.com/kubernetes-sigs/controller-runtime/pkg/controller/reconcile"
	"github.com/kubernetes-sigs/controller-runtime/pkg/runtime/inject"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/workqueue"
)

// Manager initializes shared dependencies such as Caches and Clients, and starts Controllers.
//
// Dependencies may be retrieved from the Manager using the Get* functions
type Manager interface {
	// NewController creates a new initialized Controller with the Reconcile function
	// and registers it with the Manager.
	NewController(Options, reconcile.Reconcile) (Controller, error)

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
}

var _ Manager = &controllerManager{}

type controllerManager struct {
	// config is the rest.config used to talk to the apiserver.  Required.
	config *rest.Config

	// scheme is the scheme injected into Controllers, EventHandlers, Sources and Predicates.  Defaults
	// to scheme.scheme.
	scheme *runtime.Scheme

	// controllers is the set of Controllers that the controllerManager injects deps into and Starts.
	controllers []*controller

	cache cache.Cache

	// TODO(directxman12): Provide an escape hatch to get individual indexers
	// client is the client injected into Controllers (and EventHandlers, Sources and Predicates).
	client client.Client

	// fieldIndexes knows how to add field indexes over the Cache used by this controller,
	// which can later be consumed via field selectors from the injected client.
	fieldIndexes client.FieldIndexer

	mu      sync.Mutex
	started bool
	errChan chan error
	stop    <-chan struct{}

	startCache func(stop <-chan struct{}) error
}

func (cm *controllerManager) NewController(ca Options, r reconcile.Reconcile) (Controller, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if r == nil {
		return nil, fmt.Errorf("must specify Reconcile")
	}

	if len(ca.Name) == 0 {
		return nil, fmt.Errorf("must specify Name for Controller")
	}

	if ca.MaxConcurrentReconciles <= 0 {
		ca.MaxConcurrentReconciles = 1
	}

	// Inject dependencies into Reconcile
	if err := cm.injectInto(r); err != nil {
		return nil, err
	}

	// Create controller with dependencies set
	c := &controller{
		reconcile: r,
		cache:     cm.cache,
		config:    cm.config,
		scheme:    cm.scheme,
		client:    cm.client,
		queue:     workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), ca.Name),
		maxConcurrentReconciles: ca.MaxConcurrentReconciles,
		name:   ca.Name,
		inject: cm.injectInto,
	}
	cm.controllers = append(cm.controllers, c)

	// If already started, start the controller
	if cm.started {
		go func() {
			cm.errChan <- c.Start(cm.stop)
		}()
	}
	return c, nil
}

func (cm *controllerManager) injectInto(i interface{}) error {
	if _, err := inject.ConfigInto(cm.config, i); err != nil {
		return err
	}
	if _, err := inject.ClientInto(cm.client, i); err != nil {
		return err
	}
	if _, err := inject.SchemeInto(cm.scheme, i); err != nil {
		return err
	}
	if _, err := inject.CacheInto(cm.cache, i); err != nil {
		return err
	}
	return nil
}

func (cm *controllerManager) GetConfig() *rest.Config {
	return cm.config
}

func (cm *controllerManager) GetClient() client.Client {
	return cm.client
}

func (cm *controllerManager) GetScheme() *runtime.Scheme {
	return cm.scheme
}

func (cm *controllerManager) GetFieldIndexer() client.FieldIndexer {
	return cm.fieldIndexes
}

func (cm *controllerManager) Start(stop <-chan struct{}) error {
	func() {
		cm.mu.Lock()
		defer cm.mu.Unlock()

		// Start the Cache.
		cm.stop = stop

		// Allow the function to start the cache to be mocked out for testing
		if cm.startCache == nil {
			cm.startCache = cm.cache.Start
		}
		go func() {
			if err := cm.startCache(stop); err != nil {
				cm.errChan <- err
			}
		}()

		// Start the controllers after the promises
		for _, c := range cm.controllers {
			// Controllers block, but we want to return an error if any have an error starting.
			// Write any Start errors to a channel so we can return them
			ctrl := c
			go func() {
				cm.errChan <- ctrl.Start(stop)
			}()
		}

		cm.started = true
	}()

	select {
	case <-stop:
		// We are done
		return nil
	case err := <-cm.errChan:
		// Error starting a controller
		return err
	}
}

// ManagerArgs are the arguments for creating a new Manager
type ManagerArgs struct {
	// Config is the config used to talk to an apiserver.  Defaults to:
	// 1. Config specified with the --config flag
	// 2. Config specified with the KUBECONFIG environment variable
	// 3. Incluster config (if running in a Pod)
	// 4. $HOME/.kube/config
	Config *rest.Config

	// Scheme is the scheme used to resolve runtime.Objects to GroupVersionKinds / Resources
	// Defaults to the kubernetes/client-go scheme.Scheme
	Scheme *runtime.Scheme

	// Mapper is the rest mapper used to map go types to Kubernetes APIs
	MapperProvider func(c *rest.Config) (meta.RESTMapper, error)

	// Dependency injection for testing
	newCache  func(config *rest.Config, opts cache.Options) (cache.Cache, error)
	newClient func(config *rest.Config, options client.Options) (client.Client, error)
}

// NewManager returns a new fully initialized Manager.
func NewManager(args ManagerArgs) (Manager, error) {
	cm := &controllerManager{config: args.Config, scheme: args.Scheme, errChan: make(chan error)}

	// Initialize a rest.config if none was specified
	if cm.config == nil {
		return nil, fmt.Errorf("must specify Config")
	}

	// Use the Kubernetes client-go scheme if none is specified
	if cm.scheme == nil {
		cm.scheme = scheme.Scheme
	}

	// Create a new RESTMapper for mapping GroupVersionKinds to Resources
	if args.MapperProvider == nil {
		args.MapperProvider = apiutil.NewDiscoveryRESTMapper
	}
	mapper, err := args.MapperProvider(cm.config)
	if err != nil {
		log.Error(err, "Failed to get API Group-Resources")
		return nil, err
	}

	// Allow newClient to be mocked
	if args.newClient == nil {
		args.newClient = client.New
	}
	// Create the Client for Write operations.
	writeObj, err := args.newClient(cm.config, client.Options{Scheme: cm.scheme, Mapper: mapper})
	if err != nil {
		return nil, err
	}

	// TODO(directxman12): Figure out how to allow users to request a client without requesting a watch
	if args.newCache == nil {
		args.newCache = cache.New
	}
	cm.cache, err = args.newCache(cm.config, cache.Options{Scheme: cm.scheme, Mapper: mapper})
	if err != nil {
		return nil, err
	}

	cm.fieldIndexes = cm.cache
	cm.client = client.DelegatingClient{ReadInterface: cm.cache, WriteInterface: writeObj}
	return cm, nil
}
