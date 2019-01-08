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
	"strings"
)

type aggregateError []error

func (e aggregateError) Error() string {
	var errStrs []string
	for _, err := range e {
		errStrs = append(errStrs, err.Error())
	}
	return strings.Join(errStrs, ",")
}

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// provideCtx provides the given context with or without the
// key "Dependencies" (to allow avoiding conflict with the normal
// context object in injection methods)
func provideCtx(ctx Context) Context {
	ctxVal := reflect.ValueOf(ctx)
	return &depContext{
		key:         "Dependencies",
		value:       ctxVal,
		typ:         ctxVal.Type(),
		keyOptional: true,
		next:        ctx,
	}
}

// Into reads the given object for implementations
// of injectable pseudo-interfaces and struct tags,
// and injects dependencies based on those.
// As a special case, the given context will also "Provide" itself
// with or without key "Dependencies".
func Into(ctx Context, target interface{}) (bool, error) {
	// the context provides itself
	ctx = provideCtx(ctx)

	targetV := reflect.ValueOf(target)
	targetType := targetV.Type()

	// NB(directxman12): certain conditions here are panics,
	// because they violate the pseudo-signature of the method or violate invariants
	// (we can't actually express that signature without generics)

	populatedAll := true

	// check for struct tags
	isStructPtr := targetType.Kind() == reflect.Ptr && targetType.Elem().Kind() == reflect.Struct
	if isStructPtr {
		targetElemType := targetType.Elem()
		targetElemV := targetV.Elem()

		// check for the simple case of struct tags
		for i := 0; i < targetElemType.NumField(); i++ {
			if !injectForField(ctx, targetElemType, targetElemV, i) {
				populatedAll = false
			}
		}
	}

	var callErrs []error

	// check for methods implementing pseudo-interfaces
	for i := 0; i < targetType.NumMethod(); i++ {
		populated, err := injectForMethod(ctx, targetType, targetV, i)
		if err != nil {
			callErrs = append(callErrs, err)
		}
		if !populated {
			populatedAll = false
		}
	}

	var err error
	if len(callErrs) > 0 {
		err = aggregateError(callErrs)
	}

	return populatedAll, err
}

// injectForField injects into the given field if the it has an `inject:""` tag.
// It will return false only if a given field should have been injected but was not.
func injectForField(ctx Context, targetType reflect.Type, targetV reflect.Value, fieldInd int) bool {
	fieldType := targetType.Field(fieldInd)
	key, tagPresent := fieldType.Tag.Lookup("inject")
	if !tagPresent {
		return true
	}

	fieldVal := targetV.Field(fieldInd)
	if !fieldVal.CanSet() {
		panic(fmt.Sprintf("field %q is private, and cannot be injected", fieldType.Name))
	}
	if !ctx.PopulateThis(key, fieldVal.Addr().Interface()) {
		return false
	}

	return true
}

// injectForMethod injects by calling the given method if it has the right signature.
// It will return false only if a given method should have been called but was not,
// or if there was an error while calling it.
func injectForMethod(ctx Context, targetPtrType reflect.Type, targetPtrV reflect.Value, methodInd int) (bool, error) {
	methodType := targetPtrType.Method(methodInd)
	meetsSig, key := meetsInjectPseudoSignature(methodType)
	if !meetsSig {
		return true, nil
	}
	argVal := reflect.New(methodType.Type.In(1)).Elem()
	if !ctx.PopulateThis(key, argVal.Addr().Interface()) {
		return false, nil
	}

	ret := targetPtrV.Method(methodInd).Call([]reflect.Value{argVal})
	if !ret[0].IsNil() {
		return false, ret[0].Interface().(error)
	}

	return true, nil
}

// meetsInjectPseudoSignature checks that we meet the pseudo-signature
// required for inject methods (`Inject<XYZ>(<SomeType>) error` or `Inject<XYZ>(*<SomeType>) error`).
// If true and XYZ isn't SomeType, the value of XYZ is returned.
func meetsInjectPseudoSignature(method reflect.Method) (bool, string) {
	if !strings.HasPrefix(method.Name, "Inject") {
		return false, ""
	}
	if method.Type.NumIn() != 2 { // first in is the receiver
		return false, ""
	}
	if method.Type.NumOut() != 1 || method.Type.Out(0) != errorType {
		return false, ""
	}
	argType := method.Type.In(1)
	isPtr := argType.Kind() == reflect.Ptr
	var argName string
	if isPtr {
		argName = argType.Elem().Name()
	} else {
		argName = argType.Name()
	}
	key := method.Name[6:]
	if key == argName {
		key = ""
	}

	return true, key
}
