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
	"fmt"
	"reflect"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/util/sets"
)

// Injector injects values into objects calling their `Inject` methods.
type Injector struct {
	// providers are functions that provide values for injection.
	providers map[reflect.Type]reflect.Value

	dependencies map[reflect.Type]reflect.Value
	sync.Once
}

// AddProvider takes a func with a single return value and no arguments.  This function will be used to instantiate
// dependencies injected into other objects.
func (inj *Injector) AddProvider(provider interface{}) error {
	value := reflect.ValueOf(provider)
	if value.Type().Kind() != reflect.Func {
		return fmt.Errorf("AddProvider requires value to be of type func, not %v", value.Type().Kind())
	}
	if value.Type().NumOut() != 1 {
		return fmt.Errorf("AddProvider value must return exactly 1 value")
	}
	if value.Type().NumIn() != 0 {
		return fmt.Errorf("AddProvider value must have no inputs")
	}
	if inj.providers == nil {
		inj.providers = map[reflect.Type]reflect.Value{}
	}
	inj.providers[value.Type().Out(0)] = value
	return nil
}

// AddDependency takes a
func (inj *Injector) AddDependency(dependency interface{}) error {
	value := reflect.ValueOf(dependency)
	if inj.dependencies == nil {
		inj.dependencies = map[reflect.Type]reflect.Value{}
	}
	inj.dependencies[value.Type()] = value

	return nil
}

// Inject inject values into into by calling is methods starting with "Inject".
// For each "Inject" method, the Injector will either
// - Find the corresponding provider function, invoke it to get an output value, and then invoke the method with the
//   provider output.
// - Return an error saying that a corresponding provider function was not found.
//
// Provider functions are matched to Inject functions by the provider output reflect.Type and the inject input
// reflect.Type.
func (inj *Injector) Inject(into interface{}) error {
	// Allow the injector to be injected by adding a provider for it
	inj.Once.Do(func() {
		inj.AddDependency(inj)
		inj.AddDependency(*inj)
	})

	// find all injectable methods on the object
	methods := getInjectableMethods(reflect.TypeOf(into))

	// inject each of the methods
	value := reflect.ValueOf(into)

	// for each injectable function, find a provider for the value to inject
	for i := range methods {
		m := methods[i]

		// get the method from the object to inject into (different than the method on the type)
		injectInputType := m.Type.In(1)
		injectMethod := value.MethodByName(m.Name)

		// look for the matching provider based on function input type
		if p, found := inj.providers[injectInputType]; found {
			// invoke the provider function to get a value
			out := p.Call([]reflect.Value{})

			// invoke the inject method to set the value
			injectMethod.Call([]reflect.Value{out[0]})
			continue
		}

		if p, found := inj.dependencies[injectInputType]; found {
			injectMethod.Call([]reflect.Value{p})
			continue
		}

		return fmt.Errorf("could not find Provider or Dependency for %s (requires %v)", m.Name, injectInputType)
	}

	return nil
}

// getInjectableMethods returns a map of methods used for injection for the provided type.
// methods are keyed by their index
func getInjectableMethods(t reflect.Type) []reflect.Method {
	var methods []reflect.Method
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)

		// identify injection methods
		if !isInjectableName(m) {
			continue
		}
		// 2 inputs - the target object being injected into and the value to inject
		if m.Type.NumIn() != 2 {
			continue
		}

		// sanity check
		if m.Type.In(0) != t {
			continue
		}

		// looks good, add to the map
		methods = append(methods, m)
	}
	return methods
}

var blacklist = sets.NewString("Cache", "Client", "Config", "Decoder", "Func", "Scheme", "StopChannel")

func isInjectableName(m reflect.Method) bool {
	if !strings.HasPrefix(m.Name, "Inject") {
		return false
	}
	name := strings.TrimPrefix(m.Name, "Inject")
	return !blacklist.Has(name)
}
