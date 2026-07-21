/*
Copyright 2026 The Kubernetes Authors.

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

package cache

import (
	"fmt"

	toolscache "k8s.io/client-go/tools/cache"
)

// TypedInformer adds type-safe variants for the non-type-safe methods in Informer.
type TypedInformer[object toolscache.Object] interface {
	Informer

	// AddTypedEventHandler adds a type-safe event handler to the informer.
	AddTypedEventHandler(handler toolscache.TypedResourceEventHandler[object], options ...toolscache.HandlerOptions) (toolscache.ResourceEventHandlerRegistration, error)

	// AddTypedIndexers adds type-safe indexers to the informer.
	AddTypedIndexers(indexers toolscache.TypedIndexers[object]) error
}

// NewTypedInformer wraps an Informer with type-safe event handler and indexer methods.
func NewTypedInformer[object toolscache.Object](informer Informer) TypedInformer[object] {
	if typedInformer, ok := informer.(TypedInformer[object]); ok {
		return typedInformer
	}
	return &typedInformer[object]{Informer: informer}
}

type typedInformer[object toolscache.Object] struct {
	Informer
}

func (i *typedInformer[object]) AddTypedEventHandler(handler toolscache.TypedResourceEventHandler[object], options ...toolscache.HandlerOptions) (toolscache.ResourceEventHandlerRegistration, error) {
	var o toolscache.HandlerOptions
	switch len(options) {
	case 0:
	case 1:
		o = options[0]
	default:
		return nil, fmt.Errorf("at most one HandlerOptions may be passed, got %d", len(options))
	}
	return i.AddEventHandlerWithOptions(&typedResourceEventHandler[object]{handler: handler}, o)
}

func (i *typedInformer[object]) AddTypedIndexers(indexers toolscache.TypedIndexers[object]) error {
	return i.AddIndexers(toolscache.TypedIndexersToIndexers(indexers))
}

// typedResourceEventHandler adapts a TypedResourceEventHandler to a ResourceEventHandler.
type typedResourceEventHandler[object toolscache.Object] struct {
	handler toolscache.TypedResourceEventHandler[object]
}

func (h *typedResourceEventHandler[object]) OnAdd(obj any, isInInitialList bool) {
	h.handler.OnAdd(obj.(object), isInInitialList)
}

func (h *typedResourceEventHandler[object]) OnUpdate(oldObj, newObj any) {
	h.handler.OnUpdate(oldObj.(object), newObj.(object))
}

func (h *typedResourceEventHandler[object]) OnDelete(obj any) {
	if tombstone, ok := obj.(toolscache.DeletedFinalStateUnknown); ok {
		var optionalObj object
		if tombstone.Obj != nil {
			optionalObj = tombstone.Obj.(object)
		}
		h.handler.OnDelete(toolscache.DeletedObject[object]{OptionalObj: optionalObj, FinalStateUnknown: &tombstone})
		return
	}

	h.handler.OnDelete(toolscache.DeletedObject[object]{OptionalObj: obj.(object)})
}
