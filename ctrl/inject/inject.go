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

package inject

import (
	"github.com/kubernetes-sigs/kubebuilder/pkg/client"
	"github.com/kubernetes-sigs/kubebuilder/pkg/informer"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
)

// Informers is used by the ControllerManager to inject Informers into Sources, EventHandlers, Predicates, and
// Reconciles
type Informers interface {
	InjectInformers(informer.Informers) error
}

func InjectInformers(c informer.Informers, i interface{}) (bool, error) {
	if s, ok := i.(Informers); ok {
		return true, s.InjectInformers(c)
	}
	return false, nil
}

// Config is used by the ControllerManager to inject Config into Sources, EventHandlers, Predicates, and
// Reconciles
type Config interface {
	InjectConfig(*rest.Config) error
}

func InjectConfig(c *rest.Config, i interface{}) (bool, error) {
	if s, ok := i.(Config); ok {
		return true, s.InjectConfig(c)
	}
	return false, nil
}

// Client is used by the ControllerManager to inject Client into Sources, EventHandlers, Predicates, and
// Reconciles
type Client interface {
	InjectClient(client.Interface) error
}

func InjectClient(cacheClient client.Interface, i interface{}) (bool, error) {
	if s, ok := i.(Client); ok {
		return true, s.InjectClient(cacheClient)
	}
	return false, nil
}

// Scheme is used by the ControllerManager to inject Scheme into Sources, EventHandlers, Predicates, and
// Reconciles
type Scheme interface {
	InjectScheme(scheme *runtime.Scheme) error
}

func InjectScheme(s *runtime.Scheme, i interface{}) (bool, error) {
	if is, ok := i.(Scheme); ok {
		return true, is.InjectScheme(s)
	}
	return false, nil
}
