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

package ctrl

import (
	"fmt"
	"sync"

	"github.com/kubernetes-sigs/kubebuilder/pkg/client"
	"github.com/kubernetes-sigs/kubebuilder/pkg/config"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/common"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/inject"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/reconcile"
	"github.com/kubernetes-sigs/kubebuilder/pkg/informer"
	"k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/workqueue"
)

// ControllerManager initializes shared dependencies such as Caches and Clients, and starts Controllers.
//
// Dependencies may be retrieved from the ControllerManager using the Get* functions
type ControllerManager interface {
	// NewController creates a new initialized Controller with the Reconcile function
	// and registers it with the ControllerManager.
	NewController(ControllerArgs, reconcile.Reconcile) (Controller, error)

	// Start starts all registered Controllers and blocks until the Stop channel is closed.
	// Returns an error if there is an error starting any controller.
	Start(<-chan struct{}) error

	// GetConfig returns an initialized Config
	GetConfig() *rest.Config

	// GetScheme returns and initialized Scheme
	GetScheme() *runtime.Scheme

	// GetClient returns a Client configured with the Config
	GetClient() client.Interface

	// GetFieldIndexer returns a client.FieldIndexer configured with the Client
	GetFieldIndexer() client.FieldIndexer
}

var _ ControllerManager = &controllerManager{}

type controllerManager struct {
	// config is the rest.config used to talk to the apiserver.  Required.
	config *rest.Config

	// scheme is the scheme injected into Controllers, EventHandlers, Sources and Predicates.  Defaults
	// to scheme.scheme.
	scheme *runtime.Scheme

	// controllers is the set of Controllers that the controllerManager injects deps into and Starts.
	controllers []*controller

	// informers are injected into Controllers (,and transitively EventHandlers, Sources and Predicates).
	informers informer.Informers

	// TODO(directxman12): Provide an escape hatch to get individual indexers
	// client is the client injected into Controllers (and EventHandlers, Sources and Predicates).
	client client.Interface

	// fieldIndexes knows how to add field indexes over the Informers used by this controller,
	// which can later be consumed via field selectors from the injected client.
	fieldIndexes client.FieldIndexer

	mu      sync.Mutex
	started bool
	errChan chan error
	stop    <-chan struct{}
}

func (cm *controllerManager) NewController(ca ControllerArgs, r reconcile.Reconcile) (Controller, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if len(ca.Name) == 0 {
		return nil, fmt.Errorf("Must specify name for Controller.")
	}

	if ca.MaxConcurrentReconciles <= 0 {
		ca.MaxConcurrentReconciles = 1
	}

	// Inject dependencies into Reconcile
	cm.injectInto(r)

	// Create controller with dependencies set
	c := &controller{
		reconcile: r,
		informers: cm.informers,
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
	if _, err := inject.InjectConfig(cm.config, i); err != nil {
		return err
	}
	if _, err := inject.InjectClient(cm.client, i); err != nil {
		return err
	}
	if _, err := inject.InjectScheme(cm.scheme, i); err != nil {
		return err
	}
	if _, err := inject.InjectInformers(cm.informers, i); err != nil {
		return err
	}
	return nil
}

func (cm *controllerManager) GetConfig() *rest.Config {
	return cm.config
}

func (cm *controllerManager) GetClient() client.Interface {
	return cm.client
}

func (cm *controllerManager) GetScheme() *runtime.Scheme {
	return cm.scheme
}

func (cm *controllerManager) GetFieldIndexer() client.FieldIndexer {
	return cm.fieldIndexes
}

func (cm *controllerManager) Start(stop <-chan struct{}) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Start the Informers.
	cm.stop = stop
	cm.informers.Start(stop)

	// Start the controllers after the promises
	for _, c := range cm.controllers {
		// Controllers block, but we want to return an error if any have an error starting.
		// Write any Start errors to a channel so we can return them
		go func() {
			cm.errChan <- c.Start(stop)
		}()
	}

	cm.started = true
	select {
	case <-stop:
		// We are done
		return nil
	case err := <-cm.errChan:
		// Error starting a controller
		return err
	}
}

// ControllerManagerArgs are the arguments for creating a new ControllerManager
type ControllerManagerArgs struct {
	// Config is the config used to talk to an apiserver.  Defaults to:
	// 1. Config specified with the --config flag
	// 2. Config specified with the KUBECONFIG environment variable
	// 3. Incluster config (if running in a Pod)
	// 4. $HOME/.kube/config
	Config *rest.Config

	// Scheme is the scheme used to resolve runtime.Objects to GroupVersionKinds / Resources
	// Defaults to the kubernetes/client-go scheme.Scheme
	Scheme *runtime.Scheme
}

// NewControllerManager returns a new fully initialized ControllerManager.
func NewControllerManager(args ControllerManagerArgs) (ControllerManager, error) {
	cm := &controllerManager{config: args.Config, scheme: args.Scheme, errChan: make(chan error)}

	// Initialize a rest.config if none was specified
	if cm.config == nil {
		var err error
		cm.config, err = config.GetConfig()
		if err != nil {
			return nil, err
		}
	}

	// Use the Kubernetes client-go scheme if none is specified
	if cm.scheme == nil {
		cm.scheme = scheme.Scheme
	}

	spi := &informer.SelfPopulatingInformers{
		Config: cm.config,
		Scheme: cm.scheme,
	}
	cm.informers = spi
	cm.informers.InformerFor(&v1.Deployment{})

	// Inject a Read / Write client into all controllers
	// TODO(directxman12): Figure out how to allow users to request a client without requesting a watch
	objCache := client.ObjectCacheFromInformers(spi.KnownInformersByType(), cm.scheme)
	spi.Callbacks = append(spi.Callbacks, objCache)

	mapper, err := common.NewDiscoveryRESTMapper(cm.config)
	if err != nil {
		log.Error(err, "Failed to get API Group-Resources")
		return nil, err
	}

	cm.fieldIndexes = &client.InformerFieldIndexer{Informers: spi}

	// Inject the client after all the watches have been set
	writeObj := &client.Client{Config: cm.config, Scheme: cm.scheme, Mapper: mapper}
	cm.client = client.SplitReaderWriter{ReadInterface: objCache, WriteInterface: writeObj}
	return cm, nil
}
