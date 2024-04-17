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

package source

import (
	"context"
	"fmt"
	"sync"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/interfaces"
	internal "sigs.k8s.io/controller-runtime/pkg/internal/source"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	// defaultBufferSize is the default number of event notifications that can be buffered.
	defaultBufferSize = 1024
)

// Source is a source of events (e.g. Create, Update, Delete operations on Kubernetes Objects, Webhook callbacks, etc)
// which should be processed by event.EventHandlers to enqueue reconcile.Requests.
//
// * Use Kind for events originating in the cluster (e.g. Pod Create, Pod Update, Deployment Update).
//
// * Use Channel for events originating outside the cluster (e.g. GitHub Webhook callback, Polling external urls).
//
// Users may build their own Source implementations.
type Source = interfaces.Source

// Syncing allows to wait for synchronization with context
type Syncing = interfaces.Syncing

// SyncingSource is a source that needs syncing prior to being usable. The controller
// will call its WaitForSync prior to starting workers.
type SyncingSource = interfaces.SyncingSource

// PrepareSyncing - a SyncingSource that also implements SourcePrepare and has WaitForSync method
type PrepareSyncing = interfaces.PrepareSyncing

// PrepareSource - Prepares a Source to be used with EventHandler and predicates
type PrepareSource = interfaces.PrepareSource

// PrepareSyncingObject - a SyncingSource that also implements PrepareSourceObject[T] and has WaitForSync method
type PrepareSyncingObject[T any] interface {
	interfaces.PrepareSyncingObject[T]
}

// Kind creates a KindSource with the given cache provider.
func Kind(cache cache.Cache, object client.Object) PrepareSyncing {
	return &internal.Kind[client.Object]{Type: object, Cache: cache}
}

// ObjectKind creates a typed KindSource with the given cache provider.
func ObjectKind[T client.Object](cache cache.Cache, object T) PrepareSyncingObject[T] {
	return &internal.Kind[T]{Type: object, Cache: cache}
}

var _ PrepareSource = &Channel{}

// Channel is used to provide a source of events originating outside the cluster
// (e.g. GitHub Webhook callback).  Channel requires the user to wire the external
// source (e.g. http handler) to write GenericEvents to the underlying channel.
type Channel struct {
	// once ensures the event distribution goroutine will be performed only once
	once sync.Once

	// Source is the source channel to fetch GenericEvents
	Source <-chan event.GenericEvent

	// dest is the destination channels of the added event handlers
	dest []chan event.GenericEvent

	// DestBufferSize is the specified buffer size of dest channels.
	// Default to 1024 if not specified.
	DestBufferSize int

	// destLock is to ensure the destination channels are safely added/removed
	destLock sync.Mutex

	predicates []predicate.Predicate

	handler handler.EventHandler
}

func (cs *Channel) String() string {
	return fmt.Sprintf("channel source: %p", cs)
}

// WaitForSync implements the source.SyncingSource interface
func (cs *Channel) WaitForSync(ctx context.Context) error {
	return nil
}

// Prepare implements Source preparation and should only be called when handler and predicates are available.
func (cs *Channel) Prepare(
	handler handler.EventHandler,
	prct ...predicate.Predicate,
) SyncingSource {
	cs.predicates = prct
	cs.handler = handler

	return cs
}

// Start implements Source and should only be called by the Controller.
func (cs *Channel) Start(
	ctx context.Context,
	queue workqueue.RateLimitingInterface,
) error {
	// Source should have been specified by the user.
	if cs.Source == nil {
		return fmt.Errorf("must specify Channel.Source")
	}

	if cs.handler == nil {
		return fmt.Errorf("must specify Channel.EventHandler")
	}

	// use default value if DestBufferSize not specified
	if cs.DestBufferSize == 0 {
		cs.DestBufferSize = defaultBufferSize
	}

	dst := make(chan event.GenericEvent, cs.DestBufferSize)

	cs.destLock.Lock()
	cs.dest = append(cs.dest, dst)
	cs.destLock.Unlock()

	cs.once.Do(func() {
		// Distribute GenericEvents to all EventHandler / Queue pairs Watching this source
		go cs.syncLoop(ctx)
	})

	go func() {
		for evt := range dst {
			shouldHandle := true
			for _, p := range cs.predicates {
				if !p.Generic(evt) {
					shouldHandle = false
					break
				}
			}

			if shouldHandle {
				func() {
					ctx, cancel := context.WithCancel(ctx)
					defer cancel()
					cs.handler.Generic(ctx, evt, queue)
				}()
			}
		}
	}()

	return nil
}

func (cs *Channel) doStop() {
	cs.destLock.Lock()
	defer cs.destLock.Unlock()

	for _, dst := range cs.dest {
		close(dst)
	}
}

func (cs *Channel) distribute(evt event.GenericEvent) {
	cs.destLock.Lock()
	defer cs.destLock.Unlock()

	for _, dst := range cs.dest {
		// We cannot make it under goroutine here, or we'll meet the
		// race condition of writing message to closed channels.
		// To avoid blocking, the dest channels are expected to be of
		// proper buffer size. If we still see it blocked, then
		// the controller is thought to be in an abnormal state.
		dst <- evt
	}
}

func (cs *Channel) syncLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			// Close destination channels
			cs.doStop()
			return
		case evt, stillOpen := <-cs.Source:
			if !stillOpen {
				// if the source channel is closed, we're never gonna get
				// anything more on it, so stop & bail
				cs.doStop()
				return
			}
			cs.distribute(evt)
		}
	}
}

// Informer is used to provide a source of events originating inside the cluster from Watches (e.g. Pod Create).
type Informer struct {
	// Informer is the controller-runtime Informer
	Informer cache.Informer

	predicates []predicate.Predicate

	handler handler.EventHandler
}

var _ PrepareSource = &Informer{}

// Prepare implements the source.PrepareSyncing interface
func (is *Informer) Prepare(
	h handler.EventHandler,
	prct ...predicate.Predicate,
) SyncingSource {
	is.handler = h
	is.predicates = prct

	return is
}

// WaitForSync implements the source.SyncingSource interface
func (is *Informer) WaitForSync(ctx context.Context) error {
	return nil
}

// Start is internal and should be called only by the Controller to register an EventHandler with the Informer
// to enqueue reconcile.Requests.
func (is *Informer) Start(ctx context.Context, queue workqueue.RateLimitingInterface) error {
	// Informer should have been specified by the user.
	if is.Informer == nil {
		return fmt.Errorf("must specify Informer.Informer")
	}

	// handler should have been specified by the user.
	if is.handler == nil {
		return fmt.Errorf("must specify Informer.handler with Prepare()")
	}

	_, err := is.Informer.AddEventHandler(internal.NewEventHandler(ctx, queue, handler.ObjectFuncAdapter[client.Object](is.handler), predicate.ObjectPredicatesAdapter[client.Object](is.predicates...)).HandlerFuncs())
	if err != nil {
		return err
	}
	return nil
}

func (is *Informer) String() string {
	return fmt.Sprintf("informer source: %p", is.Informer)
}

// // Func is no longer compatible
// var _ Source = Func(nil)

// // Func is a function that implements Source.
// type Func func(context.Context, handler.EventHandler, workqueue.RateLimitingInterface, ...predicate.Predicate) error

// // Start implements Source.
// func (f Func) Start(ctx context.Context, evt handler.EventHandler, queue workqueue.RateLimitingInterface,
// 	pr ...predicate.Predicate) error {
// 	return f(ctx, evt, queue, pr...)
// }

// func (f Func) String() string {
// 	return fmt.Sprintf("func source: %p", f)
// }
