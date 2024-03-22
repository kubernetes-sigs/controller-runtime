/*
Copyright 2023 The Kubernetes Authors.

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

package internal

import (
	"context"
	"fmt"
	"sync"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// ChannelOptions contains the options for the Channel source.
type ChannelOptions struct {
	// DestBufferSize is the specified buffer size of dest channels.
	// Default to 1024 if not specified.
	DestBufferSize int
}

// Channel is used to provide a source of events originating outside the cluster
// (e.g. GitHub Webhook callback).  Channel requires the user to wire the external
// source (eh.g. http handler) to write GenericEvents to the underlying channel.
type Channel struct {
	Options ChannelOptions

	// Broadcaster contains the source channel for events.
	Broadcaster *ChannelBroadcaster

	// EventHandler is the handler to call when events are received.
	EventHandler handler.EventHandler

	mu sync.Mutex
	// isStarted is true if Start has been called.
	// - A source can only be started once.
	// - WaitForSync can only be called after the source is started.
	// - Shutdown will exit early if the source was never started.
	isStarted bool
	// shuttingDown is true if Shutdown has been called.
	// - If true, Start will exit early.
	shuttingDown bool

	// doneCh is closed when the source stopped listening to the broadcaster.
	doneCh chan struct{}
}

func (cs *Channel) String() string {
	return fmt.Sprintf("channel source: %p", cs)
}

// Start is internal and should be called only by the Controller to start a source which enqueue reconcile.Requests.
// It should NOT block, instead, it should start a goroutine and return immediately.
// The context passed to Start can be used to cancel this goroutine. After the context is canceled, Shutdown can be
// called to wait for the goroutine to exit.
// Start can be called only once, it is thus not possible to share a single source between multiple controllers.
func (cs *Channel) Start(ctx context.Context, queue workqueue.RateLimitingInterface) error {
	if cs.Broadcaster == nil {
		return fmt.Errorf("must create Channel with a non-nil Broadcaster")
	}
	if cs.EventHandler == nil {
		return fmt.Errorf("must create Channel with a non-nil EventHandler")
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.isStarted {
		return fmt.Errorf("cannot start an already started Channel source")
	}
	if cs.shuttingDown {
		return nil
	}
	cs.isStarted = true

	// Create a destination channel for the event handler
	// and add it to the list of destinations
	destination := make(chan event.GenericEvent, cs.Options.DestBufferSize)
	cs.Broadcaster.AddListener(destination)

	cs.doneCh = make(chan struct{})
	go func() {
		defer close(cs.doneCh)
		// Remove the listener and wait for the broadcaster
		// to stop sending events to the destination channel.
		defer cs.Broadcaster.RemoveListener(destination)

		cs.processReceivedEvents(
			ctx,
			destination,
			queue,
		)
	}()

	return nil
}

func (cs *Channel) processReceivedEvents(
	ctx context.Context,
	destination <-chan event.GenericEvent,
	queue workqueue.RateLimitingInterface,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, stillOpen := <-destination:
			if !stillOpen {
				return
			}

			// Call the event handler with the event.
			cs.EventHandler.Generic(ctx, event, queue)
		}
	}
}

// Shutdown marks a Source as shutting down. At that point no new
// informers can be started anymore and Start will return without
// doing anything.
//
// In addition, Shutdown blocks until all goroutines have terminated. For that
// to happen, the close channel(s) that they were started with must be closed,
// either before Shutdown gets called or while it is waiting.
//
// Shutdown may be called multiple times, even concurrently. All such calls will
// block until all goroutines have terminated.
func (cs *Channel) Shutdown() error {
	if func() bool {
		cs.mu.Lock()
		defer cs.mu.Unlock()

		// Ensure that when we release the lock, we stop an in-process & future calls to Start().
		cs.shuttingDown = true

		// If we haven't started yet, there's nothing to stop.
		return !cs.isStarted
	}() {
		return nil
	}

	// Wait for the listener goroutine to exit.
	<-cs.doneCh

	return nil
}
