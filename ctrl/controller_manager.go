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

	"github.com/kubernetes-sigs/kubebuilder/pkg/client"
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

type ControllerManager interface {
	NewController(ControllerArgs, reconcile.Reconcile) Controller
	Start(<-chan struct{}) error
	GetConfig() *rest.Config
	GetClient() client.Interface
	GetScheme() *runtime.Scheme
	GetFieldIndexer() client.FieldIndexer
}

var _ ControllerManager = &controllerManager{}

// controllerManager initializes and starts Controllers.  controllerManager should always be used to
// setup dependencies such as Informers and Configs, etc and setControllerFields them into Controllers.
//
// Must specify the config.
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
}

// NewController registers a controller with the controllerManager.
// Added Controllers will have config and Informers injected into them at Start time.
func (cm *controllerManager) NewController(ca ControllerArgs, r reconcile.Reconcile) Controller {
	if ca.MaxConcurrentReconciles <= 0 {
		ca.MaxConcurrentReconciles = 1
	}
	if len(ca.Name) == 0 {
		ca.Name = "controller-unamed"
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
	return c
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

// Start starts all registered Controllers and blocks until the Stop channel is closed.
// Returns an error if there is an error starting any controller.
// Injects Informers and config into Controllers before Starting them.
func (cm *controllerManager) Start(stop <-chan struct{}) error {
	// Start the Informers.
	cm.informers.Start(stop)

	// Start the controllers after the promises
	controllerErrors := make(chan error)
	for _, c := range cm.controllers {
		// Controllers block, but we want to return an error if any have an error starting.
		// Write any Start errors to a channel so we can return them
		go func() {
			controllerErrors <- c.Start(stop)
		}()
	}
	select {
	case <-stop:
		// We are done
		return nil
	case err := <-controllerErrors:
		// Error starting a controller
		return err
	}
}

type ControllerManagerArgs struct {
	Config *rest.Config
	Scheme *runtime.Scheme
}

func NewControllerManager(args ControllerManagerArgs) (ControllerManager, error) {
	cm := &controllerManager{config: args.Config, scheme: args.Scheme}

	// Initialize a rest.config if none was specified
	if cm.config == nil {
		err := fmt.Errorf("must specify non-nil config for NewControllerManager")
		log.Error(err, "", "config", cm.config)
		return nil, err
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
