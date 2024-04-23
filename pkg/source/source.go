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
	"errors"
	"fmt"
	"sync"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	internal "sigs.k8s.io/controller-runtime/pkg/internal/source"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"sigs.k8s.io/controller-runtime/pkg/cache"
)

// Source is a source of events (e.g. Create, Update, Delete operations on Kubernetes Objects, Webhook callbacks, etc)
// which should be processed by event.EventHandlers to enqueue reconcile.Requests.
//
// * Use Kind for events originating in the cluster (e.g. Pod Create, Pod Update, Deployment Update).
//
// * Use Channel for events originating outside the cluster (e.g. GitHub Webhook callback, Polling external urls).
//
// Users may build their own Source implementations.
type Source interface {
	// Start is internal and should be called only by the Controller to register an EventHandler with the Informer
	// to enqueue reconcile.Requests.
	Start(context.Context, workqueue.RateLimitingInterface) error
}

// SyncingSource is a source that needs syncing prior to being usable. The controller
// will call its WaitForSync prior to starting workers.
type SyncingSource interface {
	Source
	WaitForSync(ctx context.Context) error
}

var _ Source = &internal.Kind[client.Object]{}
var _ SyncingSource = &internal.Kind[client.Object]{}

// Kind creates a KindSource with the given cache provider.
func Kind[T client.Object](cache cache.Cache, object T, handler handler.TypedEventHandler[T], predicates ...predicate.TypedPredicate[T]) SyncingSource {
	return &internal.Kind[T]{
		Type:       object,
		Cache:      cache,
		Handler:    handler,
		Predicates: predicates,
	}
}

var _ Source = &channel[string]{}

// ChannelOpt allows to configure a source.Channel.
type ChannelOpt[T any] func(*channel[T])

// WithPredicates adds the configured predicates to a source.Channel.
func WithPredicates[T any](p ...predicate.TypedPredicate[T]) ChannelOpt[T] {
	return func(c *channel[T]) {
		c.predicates = append(c.predicates, p...)
	}
}

// WithBufferSize configures the buffer size for a source.Channel. By
// default, the buffer size is 1024.
func WithBufferSize[T any](bufferSize int) ChannelOpt[T] {
	return func(c *channel[T]) {
		c.bufferSize = bufferSize
	}
}

// Channel is used to provide a source of events originating outside the cluster
// (e.g. GitHub Webhook callback).  Channel requires the user to wire the external
// source (e.g. http handler) to write GenericEvents to the underlying channel.
func Channel[T any](broadcaster *channelBroadcaster[T], handler handler.TypedEventHandler[T], opts ...ChannelOpt[T]) Source {
	c := &channel[T]{
		broadcaster: broadcaster,
		handler:     handler,
		bufferSize:  1024,
	}
	for _, opt := range opts {
		opt(c)
	}

	return c
}

var _ Source = &channel[string]{}

type channel[T any] struct {
	// broadcaster contains the source channel for events.
	broadcaster *channelBroadcaster[T]

	handler handler.TypedEventHandler[T]

	predicates []predicate.TypedPredicate[T]

	bufferSize int

	mu sync.Mutex
	// isStarted is true if the source has been started. A source can only be started once.
	isStarted bool
}

func (cs *channel[T]) String() string {
	return fmt.Sprintf("channel source: %p", cs)
}

// Start implements Source and should only be called by the Controller.
func (cs *channel[T]) Start(
	ctx context.Context,
	queue workqueue.RateLimitingInterface,
) error {
	// Source should have been specified by the user.
	if cs.broadcaster == nil {
		return fmt.Errorf("must create Channel with a non-nil broadcaster")
	}
	if cs.handler == nil {
		return errors.New("must create Channel with a non-nil handler")
	}
	if cs.bufferSize == 0 {
		return errors.New("must create Channel with a >0 bufferSize")
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.isStarted {
		return fmt.Errorf("cannot start an already started Channel source")
	}
	cs.isStarted = true

	// Create a destination channel for the event handler
	// and add it to the list of destinations
	destination := make(chan event.TypedGenericEvent[T], cs.bufferSize)
	cs.broadcaster.AddListener(destination)

	go func() {
		// Remove the listener and wait for the broadcaster
		// to stop sending events to the destination channel.
		defer cs.broadcaster.RemoveListener(destination)

		cs.processReceivedEvents(
			ctx,
			destination,
			queue,
			cs.handler,
			cs.predicates,
		)
	}()

	return nil
}

func (cs *channel[T]) processReceivedEvents(
	ctx context.Context,
	destination <-chan event.TypedGenericEvent[T],
	queue workqueue.RateLimitingInterface,
	eventHandler handler.TypedEventHandler[T],
	predicates []predicate.TypedPredicate[T],
) {
eventloop:
	for {
		select {
		case <-ctx.Done():
			return
		case event, stillOpen := <-destination:
			if !stillOpen {
				return
			}

			// Check predicates against the event first
			// and continue the outer loop if any of them fail.
			for _, p := range predicates {
				if !p.Generic(event) {
					continue eventloop
				}
			}

			// Call the event handler with the event.
			eventHandler.Generic(ctx, event, queue)
		}
	}
}

// NewChannelBroadcaster creates a new ChannelBroadcaster for the given channel.
// A ChannelBroadcaster is a wrapper around a channel that allows multiple listeners to all
// receive the events from the channel.
func NewChannelBroadcaster[T any](source <-chan event.TypedGenericEvent[T]) *channelBroadcaster[T] {
	return &channelBroadcaster[T]{
		source: source,
	}
}

// ChannelBroadcaster is a wrapper around a channel that allows multiple listeners to all
// receive the events from the channel.
type channelBroadcaster[T any] struct {
	source <-chan event.TypedGenericEvent[T]

	mu           sync.Mutex
	rcCount      uint
	managementCh chan managementMsg[T]
	doneCh       chan struct{}
}

type managementOperation bool

const (
	addChannel    managementOperation = true
	removeChannel managementOperation = false
)

type managementMsg[T any] struct {
	operation managementOperation
	ch        chan event.TypedGenericEvent[T]
}

// AddListener adds a new listener to the ChannelBroadcaster. Each listener
// will receive all events from the source channel. All listeners have to be
// removed using RemoveListener before the ChannelBroadcaster can be garbage
// collected.
func (sc *channelBroadcaster[T]) AddListener(ch chan event.TypedGenericEvent[T]) {
	var managementCh chan managementMsg[T]
	var doneCh chan struct{}
	isFirst := false
	func() {
		sc.mu.Lock()
		defer sc.mu.Unlock()

		isFirst = sc.rcCount == 0
		sc.rcCount++

		if isFirst {
			sc.managementCh = make(chan managementMsg[T])
			sc.doneCh = make(chan struct{})
		}

		managementCh = sc.managementCh
		doneCh = sc.doneCh
	}()

	if isFirst {
		go startLoop(sc.source, managementCh, doneCh)
	}

	// If the goroutine is not yet stopped, send a message to add the
	// destination channel. The routine might be stopped already because
	// the source channel was closed.
	select {
	case <-doneCh:
	default:
		managementCh <- managementMsg[T]{
			operation: addChannel,
			ch:        ch,
		}
	}
}

func startLoop[T any](
	source <-chan event.TypedGenericEvent[T],
	managementCh chan managementMsg[T],
	doneCh chan struct{},
) {
	defer close(doneCh)

	var destinations []chan event.TypedGenericEvent[T]

	// Close all remaining destinations in case the Source channel is closed.
	defer func() {
		for _, dst := range destinations {
			close(dst)
		}
	}()

	// Wait for the first destination to be added before starting the loop.
	for len(destinations) == 0 {
		managementMsg := <-managementCh
		if managementMsg.operation == addChannel {
			destinations = append(destinations, managementMsg.ch)
		}
	}

	for {
		select {
		case msg := <-managementCh:

			switch msg.operation {
			case addChannel:
				destinations = append(destinations, msg.ch)
			case removeChannel:
			SearchLoop:
				for i, dst := range destinations {
					if dst == msg.ch {
						destinations = append(destinations[:i], destinations[i+1:]...)
						close(dst)
						break SearchLoop
					}
				}

				if len(destinations) == 0 {
					return
				}
			}

		case evt, stillOpen := <-source:
			if !stillOpen {
				return
			}

			for _, dst := range destinations {
				// We cannot make it under goroutine here, or we'll meet the
				// race condition of writing message to closed channels.
				// To avoid blocking, the dest channels are expected to be of
				// proper buffer size. If we still see it blocked, then
				// the controller is thought to be in an abnormal state.
				dst <- evt
			}
		}
	}
}

// RemoveListener removes a listener from the ChannelBroadcaster. The listener
// will no longer receive events from the source channel. If this is the last
// listener, this function will block until the ChannelBroadcaster's is stopped.
func (sc *channelBroadcaster[T]) RemoveListener(ch chan event.TypedGenericEvent[T]) {
	var managementCh chan managementMsg[T]
	var doneCh chan struct{}
	isLast := false
	func() {
		sc.mu.Lock()
		defer sc.mu.Unlock()

		sc.rcCount--
		isLast = sc.rcCount == 0

		managementCh = sc.managementCh
		doneCh = sc.doneCh
	}()

	// If the goroutine is not yet stopped, send a message to remove the
	// destination channel. The routine might be stopped already because
	// the source channel was closed.
	select {
	case <-doneCh:
	default:
		managementCh <- managementMsg[T]{
			operation: removeChannel,
			ch:        ch,
		}
	}

	// Wait for the doneCh to be closed (in case we are the last one)
	if isLast {
		<-doneCh
	}

	// Wait for the destination channel to be closed.
	<-ch
}

var _ Source = &Informer{}

// Informer is used to provide a source of events originating inside the cluster from Watches (e.g. Pod Create).
type Informer struct {
	// Informer is the controller-runtime Informer
	Informer   cache.Informer
	Handler    handler.EventHandler
	Predicates []predicate.Predicate

	mu sync.Mutex
	// isStarted is true if the source has been started. A source can only be started once.
	isStarted bool
}

// Start is internal and should be called only by the Controller to register an EventHandler with the Informer
// to enqueue reconcile.Requests.
func (is *Informer) Start(ctx context.Context, queue workqueue.RateLimitingInterface) error {
	// Informer should have been specified by the user.
	if is.Informer == nil {
		return fmt.Errorf("must create Informer with a non-nil informer")
	}
	if is.Handler == nil {
		return errors.New("must create Informer with a non-nil handler")
	}

	is.mu.Lock()
	defer is.mu.Unlock()
	if is.isStarted {
		return fmt.Errorf("cannot start an already started Informer source")
	}
	is.isStarted = true

	_, err := is.Informer.AddEventHandler(internal.NewEventHandler(ctx, queue, is.Handler, is.Predicates).HandlerFuncs())
	if err != nil {
		return err
	}
	return nil
}

func (is *Informer) String() string {
	return fmt.Sprintf("informer source: %p", is.Informer)
}

var _ Source = Func(nil)

// Func is a function that implements Source.
type Func func(context.Context, workqueue.RateLimitingInterface) error

// Start implements Source.
func (f Func) Start(ctx context.Context, queue workqueue.RateLimitingInterface) error {
	return f(ctx, queue)
}

func (f Func) String() string {
	return fmt.Sprintf("func source: %p", f)
}
