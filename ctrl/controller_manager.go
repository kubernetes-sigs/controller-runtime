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
	"sync"

	"github.com/kubernetes-sigs/kubebuilder/pkg/client"
	"github.com/kubernetes-sigs/kubebuilder/pkg/config"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/inject"
	"github.com/kubernetes-sigs/kubebuilder/pkg/informer"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// DefaultControllerManager is the default ControllerManager.
var DefaultControllerManager = &ControllerManager{}

// ControllerManager initializes and starts Controllers.  ControllerManager should always be used to
// setup dependencies such as Informers and Configs, etc and injectInto them into Controllers.
type ControllerManager struct {
	controllers []*Controller

	// Config is the rest.config used to talk to the apiserver.  Defaults to one of in-cluster, environment variable
	// specified, or the ~/.kube/config.
	Config *rest.Config

	// Scheme is the scheme injected into Controllers, EventHandlers, Sources and Predicates
	Scheme *runtime.Scheme

	// Informers are the Informers injected into Controllers, EventHandlers, Sources and Predicates
	Informers informer.Informers

	// TODO(directxman12): Provide an escape hatch to get individual indexers
	// client is the client injected into Controllers, EventHandlers, Sources and Predicates
	client client.Interface

	// once ensures unspecified fields get default values
	once sync.Once

	// err is set when initializing
	err error

	// promises is the list of functions to run after initialization
	promises []func()
}

// AddController registers a Controller with the ControllerManager.
// Added Controllers will have Config and Informers injected into them at Start time.
func (cm *ControllerManager) AddController(c *Controller, promise func()) {
	cm.init()
	cm.controllers = append(cm.controllers, c)
	if promise != nil {
		cm.promises = append(cm.promises, promise)
	}
}

func (cm *ControllerManager) injectInto(i interface{}) {
	inject.InjectInformers(cm.Informers, i)
	inject.InjectConfig(cm.Config, i)
	inject.InjectScheme(cm.Scheme, i)
}

// Start starts all registered Controllers and blocks until the Stop channel is closed.
// Returns an error if there is an error starting any Controller.
// Injects Informers and Config into Controllers before Starting them.
func (cm *ControllerManager) Start(stop <-chan struct{}) error {
	cm.init()

	// Error initializing the ControllerManager.  Return.
	if cm.err != nil {
		return cm.err
	}

	// Inject dependencies into the controllers
	for _, c := range cm.controllers {
		cm.injectInto(c)
	}

	// Run the promises to setup the Controller Watch invocations now that the Informers has been initialized
	for _, p := range cm.promises {
		p()
	}

	// Inject a Read / Write client into all controllers
	// TODO(directxman12): Figure out how to allow users to request a client without requesting a watch
	objCache := client.ObjectCacheFromInformers(cm.Informers.KnownInformersByType(), cm.Scheme)
	cm.client = client.SplitReaderWriter{ReadInterface: objCache}
	for _, c := range cm.controllers {
		inject.InjectClient(cm.client, c)
	}

	// Start the Informers now that watches have been added
	cm.Informers.Start(stop)

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

// init defaults optional field values on a ControllerManager
func (cm *ControllerManager) init() {
	cm.once.Do(func() {
		// Initialize a rest.Config if none was specified
		if cm.Config == nil {
			cm.Config, cm.err = config.GetConfig()
		}

		// Use the Kubernetes client-go Scheme if none is specified
		if cm.Scheme == nil {
			cm.Scheme = scheme.Scheme
		}

		// Create a new set of Informers if none is specified
		if cm.Informers == nil {
			cm.Informers = &informer.SelfPopulatingInformers{
				Config: cm.Config,
				Scheme: cm.Scheme,
			}
		}
	})
}

// AddController registers a Controller with the DefaultControllerManager.
func AddController(c *Controller, promise func()) { DefaultControllerManager.AddController(c, promise) }

// Start starts all Controllers registered with the DefaultControllerManager.
func Start(stop <-chan struct{}) error { return DefaultControllerManager.Start(stop) }
