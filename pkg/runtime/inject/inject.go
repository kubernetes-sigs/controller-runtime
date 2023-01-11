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

// Package inject is used by a Manager to inject types into Sources, EventHandlers, Predicates, and Reconciles.
//
// Deprecated: Use manager.Options fields directly. This package will be removed in a future version.
package inject

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Client is used by the ControllerManager to inject client into Sources, EventHandlers, Predicates, and
// Reconciles.
//
// Deprecated: Dependency injection methods are deprecated and going to be removed in a future version.
type Client interface {
	InjectClient(client.Client) error
}

// ClientInto will set client on i and return the result if it implements Client. Returns
// false if i does not implement Client.
//
// Deprecated: Dependency injection methods are deprecated and going to be removed in a future version.
func ClientInto(client client.Client, i interface{}) (bool, error) {
	if s, ok := i.(Client); ok {
		return true, s.InjectClient(client)
	}
	return false, nil
}

// Scheme is used by the ControllerManager to inject Scheme into Sources, EventHandlers, Predicates, and
// Reconciles.
//
// Deprecated: Dependency injection methods are deprecated and going to be removed in a future version.
type Scheme interface {
	InjectScheme(scheme *runtime.Scheme) error
}

// SchemeInto will set scheme and return the result on i if it implements Scheme.  Returns
// false if i does not implement Scheme.
//
// Deprecated: Dependency injection methods are deprecated and going to be removed in a future version.
func SchemeInto(scheme *runtime.Scheme, i interface{}) (bool, error) {
	if is, ok := i.(Scheme); ok {
		return true, is.InjectScheme(scheme)
	}
	return false, nil
}

// Mapper is used to inject the rest mapper to components that may need it.
//
// Deprecated: Dependency injection methods are deprecated and going to be removed in a future version.
type Mapper interface {
	InjectMapper(meta.RESTMapper) error
}

// MapperInto will set the rest mapper on i and return the result if it implements Mapper.
// Returns false if i does not implement Mapper.
//
// Deprecated: Dependency injection methods are deprecated and going to be removed in a future version.
func MapperInto(mapper meta.RESTMapper, i interface{}) (bool, error) {
	if m, ok := i.(Mapper); ok {
		return true, m.InjectMapper(mapper)
	}
	return false, nil
}

// Func injects dependencies into i.
//
// Deprecated: Dependency injection methods are deprecated and going to be removed in a future version.
type Func func(i interface{}) error

// Injector is used by the ControllerManager to inject Func into Controllers.
//
// Deprecated: Dependency injection methods are deprecated and going to be removed in a future version.
type Injector interface {
	InjectFunc(f Func) error
}

// InjectorInto will set f and return the result on i if it implements Injector.  Returns
// false if i does not implement Injector.
//
// Deprecated: Dependency injection methods are deprecated and going to be removed in a future version.
func InjectorInto(f Func, i interface{}) (bool, error) {
	if ii, ok := i.(Injector); ok {
		return true, ii.InjectFunc(f)
	}
	return false, nil
}

// Logger is used to inject Loggers into components that need them
// and don't otherwise have opinions.
//
// Deprecated: Dependency injection methods are deprecated and going to be removed in a future version.
type Logger interface {
	InjectLogger(l logr.Logger) error
}

// LoggerInto will set the logger on the given object if it implements inject.Logger,
// returning true if a InjectLogger was called, and false otherwise.
//
// Deprecated: Dependency injection methods are deprecated and going to be removed in a future version.
func LoggerInto(l logr.Logger, i interface{}) (bool, error) {
	if injectable, wantsLogger := i.(Logger); wantsLogger {
		return true, injectable.InjectLogger(l)
	}
	return false, nil
}
