/*
Copyright 2017 The Kubernetes Authors.

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

package controllertest

import (
	"fmt"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

var _ cache.SharedIndexInformer = &FakeInformer{}

// FakeInformer provides fake Informer functionality for testing.
type FakeInformer struct {
	// Synced is returned by the HasSynced functions to implement the Informer interface
	Synced bool

	// RunCount is incremented each time RunInformersAndControllers is called
	RunCount int

	handlers []*listenHandler
	mu       sync.RWMutex
}

type listenHandler struct {
	cache.ResourceEventHandler
}

// AddIndexers does nothing.  TODO(community): Implement this.
func (f *FakeInformer) AddIndexers(indexers cache.Indexers) error {
	return nil
}

// GetIndexer does nothing.  TODO(community): Implement this.
func (f *FakeInformer) GetIndexer() cache.Indexer {
	return nil
}

// Informer returns the fake Informer.
func (f *FakeInformer) Informer() cache.SharedIndexInformer {
	return f
}

// HasSynced implements the Informer interface.  Returns f.Synced.
func (f *FakeInformer) HasSynced() bool {
	return f.Synced
}

// AddEventHandler implements the Informer interface.  Adds an EventHandler to the fake Informers. TODO(community): Implement Registration.
func (f *FakeInformer) AddEventHandler(handler cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	lh := &listenHandler{ResourceEventHandler: handler}
	f.handlers = append(f.handlers, lh)
	return &lh, nil
}

// Run implements the Informer interface.  Increments f.RunCount.
func (f *FakeInformer) Run(<-chan struct{}) {
	f.RunCount++
}

// Add fakes an Add event for obj.
func (f *FakeInformer) Add(obj metav1.Object) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, h := range f.handlers {
		h.OnAdd(obj)
	}
}

// Update fakes an Update event for obj.
func (f *FakeInformer) Update(oldObj, newObj metav1.Object) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, h := range f.handlers {
		h.OnUpdate(oldObj, newObj)
	}
}

// Delete fakes an Delete event for obj.
func (f *FakeInformer) Delete(obj metav1.Object) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, h := range f.handlers {
		h.OnDelete(obj)
	}
}

// AddEventHandlerWithResyncPeriod does nothing.  TODO(community): Implement this.
func (f *FakeInformer) AddEventHandlerWithResyncPeriod(handler cache.ResourceEventHandler, resyncPeriod time.Duration) (cache.ResourceEventHandlerRegistration, error) {
	return nil, nil
}

// RemoveEventHandler does nothing.  TODO(community): Implement this.
func (f *FakeInformer) RemoveEventHandler(handle cache.ResourceEventHandlerRegistration) error {
	lh, ok := handle.(*listenHandler)
	if !ok {
		return fmt.Errorf("invalid key type %t", handle)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	handlers := make([]*listenHandler, 0, len(f.handlers))
	for _, h := range f.handlers {
		if h == lh {
			continue
		}
		handlers = append(handlers, h)
	}
	f.handlers = handlers
	return nil
}

// GetStore does nothing.  TODO(community): Implement this.
func (f *FakeInformer) GetStore() cache.Store {
	return nil
}

// GetController does nothing.  TODO(community): Implement this.
func (f *FakeInformer) GetController() cache.Controller {
	return nil
}

// LastSyncResourceVersion does nothing.  TODO(community): Implement this.
func (f *FakeInformer) LastSyncResourceVersion() string {
	return ""
}

// SetWatchErrorHandler does nothing.  TODO(community): Implement this.
func (f *FakeInformer) SetWatchErrorHandler(cache.WatchErrorHandler) error {
	return nil
}

// SetTransform does nothing.  TODO(community): Implement this.
func (f *FakeInformer) SetTransform(t cache.TransformFunc) error {
	return nil
}

// IsStopped does nothing.  TODO(community): Implement this.
func (f *FakeInformer) IsStopped() bool {
	return false
}
