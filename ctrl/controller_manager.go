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

	"github.com/kubernetes-sigs/kubebuilder/pkg/config"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/inject"
	"github.com/kubernetes-sigs/kubebuilder/pkg/informer"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// DefaultControllerManager is the default ControllerManager.
var DefaultControllerManager = &ControllerManager{}

// ControllerManager initializes and starts Controllers.  ControllerManager should be used if there are multiple
// Controllers to share caches, stop channels, and other shared dependencies across Controllers.
type ControllerManager struct {
	controllers []*Controller

	// Config is the rest.config used to talk to the apiserver.  Defaults to one of in-cluster, environment variable
	// specified, or the ~/.kube/config.
	Config *rest.Config

	Scheme *runtime.Scheme

	// informers is the Informers
	informers informer.Informers

	// once ensures unspecified fields get default values
	once sync.Once

	err error

	promises []func()
}

// AddController registers a Controller with the ControllerManager.
// The ControllerManager Stop channel will be set on each Controller when it is registered.
func (cm *ControllerManager) AddController(c *Controller, promise func()) {
	cm.init()
	cm.controllers = append(cm.controllers, c)
	if promise != nil {
		cm.promises = append(cm.promises, promise)
	}
}

// Start starts all registered Controllers and blocks until the Stop channel is closed.
// Returns an error if there is an error starting any Controller.
func (cm *ControllerManager) Start(stop <-chan struct{}) error {
	cm.init()

	// Inject into each of the controllers
	for _, c := range cm.controllers {
		inject.InjectInformers(cm.informers, c)
		inject.InjectConfig(cm.Config, c)
	}

	// Run the promises that may add Watches to the informers
	for _, p := range cm.promises {
		p()
	}

	// Start the informers now that watches have been added
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

// init defaults field values on cm
func (cm *ControllerManager) init() {
	cm.once.Do(func() {
		if cm.Config == nil {
			cm.Config, cm.err = config.GetConfig()
		}

		if cm.Scheme == nil {
			cm.Scheme = scheme.Scheme
		}

		if cm.informers == nil {
			cm.informers = &informer.SelfPopulatingInformers{
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
