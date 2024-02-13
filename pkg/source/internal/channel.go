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

package internal

import (
	"context"
	"fmt"
	"sync"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
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

	mu sync.Mutex
	// isStarted is true if the source has been started. A source can only be started once.
	isStarted bool
}

func (cs *Channel) String() string {
	return fmt.Sprintf("channel source: %p", cs)
}

// Start implements Source and should only be called by the Controller.
func (cs *Channel) Start(
	ctx context.Context,
	handler handler.EventHandler,
	queue workqueue.RateLimitingInterface,
	prct ...predicate.Predicate) error {
	// Broadcaster should have been specified by the user.
	if cs.Broadcaster == nil {
		return fmt.Errorf("must create Channel with a non-nil Broadcaster")
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.isStarted {
		return fmt.Errorf("cannot start an already started Channel source")
	}
	cs.isStarted = true

	// Create a destination channel for the event handler
	// and add it to the list of destinations
	destination := make(chan event.GenericEvent, cs.Options.DestBufferSize)
	cs.Broadcaster.AddListener(destination)

	go func() {
		// Remove the listener and wait for the broadcaster
		// to stop sending events to the destination channel.
		defer cs.Broadcaster.RemoveListener(destination)

		cs.processReceivedEvents(
			ctx,
			destination,
			queue,
			handler,
			prct,
		)
	}()

	return nil
}

func (cs *Channel) processReceivedEvents(
	ctx context.Context,
	destination <-chan event.GenericEvent,
	queue workqueue.RateLimitingInterface,
	eventHandler handler.EventHandler,
	predicates []predicate.Predicate,
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
