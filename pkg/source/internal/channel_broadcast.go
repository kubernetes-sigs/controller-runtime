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
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/event"
)

// ChannelBroadcaster is a wrapper around a channel that allows multiple listeners to all
// receive the events from the channel.
type ChannelBroadcaster struct {
	source <-chan event.GenericEvent

	mu           sync.Mutex
	rcCount      uint
	managementCh chan managementMsg
	doneCh       chan struct{}
}

type managementOperation bool

const (
	addChannel    managementOperation = true
	removeChannel managementOperation = false
)

type managementMsg struct {
	operation managementOperation
	ch        chan event.GenericEvent
}

// NewChannelBroadcaster creates a new ChannelBroadcaster for the given channel.
func NewChannelBroadcaster(source <-chan event.GenericEvent) *ChannelBroadcaster {
	return &ChannelBroadcaster{
		source: source,
	}
}

// AddListener adds a new listener to the ChannelBroadcaster. Each listener
// will receive all events from the source channel. All listeners have to be
// removed using RemoveListener before the ChannelBroadcaster can be garbage
// collected.
func (sc *ChannelBroadcaster) AddListener(ch chan event.GenericEvent) {
	var managementCh chan managementMsg
	var doneCh chan struct{}
	isFirst := false
	func() {
		sc.mu.Lock()
		defer sc.mu.Unlock()

		isFirst = sc.rcCount == 0
		sc.rcCount++

		if isFirst {
			sc.managementCh = make(chan managementMsg)
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
		managementCh <- managementMsg{
			operation: addChannel,
			ch:        ch,
		}
	}
}

func startLoop(
	source <-chan event.GenericEvent,
	managementCh chan managementMsg,
	doneCh chan struct{},
) {
	defer close(doneCh)

	var destinations []chan event.GenericEvent

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
func (sc *ChannelBroadcaster) RemoveListener(ch chan event.GenericEvent) {
	var managementCh chan managementMsg
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
		managementCh <- managementMsg{
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
