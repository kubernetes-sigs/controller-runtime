/*
Copyright 2019 The Kubernetes Authors.

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
)

// Context carries dependencies across component boundaries.
type Context interface {
	// PopulateThis populates the given pointer with an instance of its type,
	// if available.  The target *must* be a pointer.  Key is an optional key
	// to differentiate different values of the same type.  If it's the same
	// as the type name, pass the empty string.
	PopulateThis(key string, target interface{}) bool
}

// Nothing returns a new Context that doesn't provide any dependencies.
func Nothing() Context {
	return emptyContext{}
}

// emptyContext doesn't provide anything
type emptyContext struct{}

func (c emptyContext) PopulateThis(_ string, _ interface{}) bool { return false }

// depContext is a Context that provides a dependency of a given type.
// It'll call out to next if it can provide the type asked for.
type depContext struct {
	typ         reflect.Type
	value       reflect.Value
	key         string
	keyOptional bool
	next        Context
}

func (c *depContext) PopulateThis(key string, target interface{}) bool {
	targetVal := extractOrCheckValue(target)
	if matchesType(targetVal.Type(), c.typ) && ((c.keyOptional && key == "") || key == c.key) {
		targetVal.Set(c.value)
		return true
	}

	return c.next.PopulateThis(key, target)
}

// matchesType checks if the given two types are the same, or if the second implements the first
// (if the first is an interface).
func matchesType(targetType reflect.Type, otherType reflect.Type) bool {
	if otherType == targetType {
		return true
	} else if targetType.Kind() == reflect.Interface && otherType.Implements(targetType) {
		return true
	}

	return false
}

// extractOrCheckValue checks to make sure the given object is acceptable
// for use as an injection target (it's a pointer) and pulls out the
// underlying value.
func extractOrCheckValue(target interface{}) reflect.Value {
	targetVal := reflect.ValueOf(target)
	if targetVal.Type().Kind() != reflect.Ptr {
		// We need a pointer to set
		panic(fmt.Sprintf("must pass a pointer to PopulateThis, not a %v", targetVal.Type()))
	}
	targetVal = reflect.Indirect(targetVal)

	return targetVal
}

// ProvideA provides the given dependency in the given context.
func ProvideA(ctx Context, obj interface{}) Context {
	return ProvideASpecific(ctx, "", obj)
}

// ProvideASpecific provides the given dependency in the specific case where
// the given name is asked for.  If this type implements any interfaces, it will
// automatically be the implementation of those interfaces.
func ProvideASpecific(ctx Context, key string, obj interface{}) Context {
	val := reflect.ValueOf(obj)
	return &depContext{
		typ:   val.Type(),
		value: val,
		key:   key,
		next:  ctx,
	}
}

// ProvideSome provides the given dependencies in the given context.
// It's roughly equivalent to calling ProvideA repeatedly.
func ProvideSome(ctx Context, objs ...interface{}) Context {
	for _, obj := range objs {
		ctx = ProvideA(ctx, obj)
	}

	return ctx
}
