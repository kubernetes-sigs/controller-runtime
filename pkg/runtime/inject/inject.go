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
	"github.com/kubernetes-sigs/controller-runtime/pkg/client"
	"github.com/kubernetes-sigs/controller-runtime/pkg/internal/informer"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
)

// Informers is used by the ControllerManager to inject Informers into Sources, EventHandlers, Predicates, and
// Reconciles
type Informers interface {
	InjectInformers(informer.Informers) error
}

// DoInformers will set informers on i and return the result if it implements Informers.  Returns
//// false if i does not implement Informers.
func DoInformers(informers informer.Informers, i interface{}) (bool, error) {
	if s, ok := i.(Informers); ok {
		return true, s.InjectInformers(informers)
	}
	return false, nil
}

// Config is used by the ControllerManager to inject Config into Sources, EventHandlers, Predicates, and
// Reconciles
type Config interface {
	InjectConfig(*rest.Config) error
}

// DoConfig will set config on i and return the result if it implements Config.  Returns
//// false if i does not implement Config.
func DoConfig(config *rest.Config, i interface{}) (bool, error) {
	if s, ok := i.(Config); ok {
		return true, s.InjectConfig(config)
	}
	return false, nil
}

// Client is used by the ControllerManager to inject Client into Sources, EventHandlers, Predicates, and
// Reconciles
type Client interface {
	InjectClient(client.Interface) error
}

// DoClient will set client on i and return the result if it implements Client.  Returns
//// false if i does not implement Client.
func DoClient(client client.Interface, i interface{}) (bool, error) {
	if s, ok := i.(Client); ok {
		return true, s.InjectClient(client)
	}
	return false, nil
}

// Scheme is used by the ControllerManager to inject Scheme into Sources, EventHandlers, Predicates, and
// Reconciles
type Scheme interface {
	InjectScheme(scheme *runtime.Scheme) error
}

// DoScheme will set client and return the result on i if it implements Scheme.  Returns
// false if i does not implement Scheme.
func DoScheme(scheme *runtime.Scheme, i interface{}) (bool, error) {
	if is, ok := i.(Scheme); ok {
		return true, is.InjectScheme(scheme)
	}
	return false, nil
}
