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

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	internal "sigs.k8s.io/controller-runtime/pkg/source/internal"

	"sigs.k8s.io/controller-runtime/pkg/cache"
)

// Source is a source of events (eh.g. Create, Update, Delete operations on Kubernetes Objects, Webhook callbacks, etc)
// which should be processed by event.EventHandlers to enqueue reconcile.Requests.
//
// * Use Kind for events originating in the cluster (e.g. Pod Create, Pod Update, Deployment Update).
//
// * Use Channel for events originating outside the cluster (eh.g. GitHub Webhook callback, Polling external urls).
//
// Users may build their own Source implementations.
type Source interface {
	// Start is internal and should be called only by the Controller to start a goroutine that enqueues
	// reconcile.Requests. It should NOT block, instead, it should start a goroutine and return immediately.
	// The context passed to Start can be used to cancel the blocking operations in the Start method. To cancel the
	// goroutine/ shutdown the source a user should call Stop.
	Start(context.Context, workqueue.RateLimitingInterface) error

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
	Shutdown() error
}

// SyncingSource is a source that needs syncing prior to being usable. The controller
// will call its WaitForSync prior to starting workers.
type SyncingSource interface {
	Source
	WaitForSync(ctx context.Context) error
}

var _ Source = &internal.Kind{}
var _ SyncingSource = &internal.Kind{}
var _ Source = &internal.Informer{}
var _ SyncingSource = &internal.Informer{}
var _ Source = &internal.Channel{}
var _ Source = &internal.FuncSource{}

// Kind creates a KindSource with the given cache provider.
func Kind(cache cache.Cache, object client.Object, eventhandler handler.EventHandler) SyncingSource {
	return &internal.Kind{Cache: cache, Type: object, EventHandler: eventhandler}
}

// NewChannelBroadcaster creates a new ChannelBroadcaster for the given channel.
// A ChannelBroadcaster is a wrapper around a channel that allows multiple listeners to all
// receive the events from the channel.
var NewChannelBroadcaster = internal.NewChannelBroadcaster

// ChannelOption is a functional option for configuring a Channel source.
type ChannelOption func(*internal.ChannelOptions)

// WithDestBufferSize specifies the buffer size of dest channels.
func WithDestBufferSize(destBufferSize int) ChannelOption {
	return func(o *internal.ChannelOptions) {
		if destBufferSize <= 0 {
			return // ignore invalid buffer size
		}

		o.DestBufferSize = destBufferSize
	}
}

// Channel creates a ChannelSource with the given buffer size.
func Channel(broadcaster *internal.ChannelBroadcaster, eventhandler handler.EventHandler, options ...ChannelOption) Source {
	opts := internal.ChannelOptions{
		// 1024 is the default number of event notifications that can be buffered.
		DestBufferSize: 1024,
	}
	for _, o := range options {
		if o == nil {
			continue // ignore nil options
		}
		o(&opts)
	}

	return &internal.Channel{Options: opts, Broadcaster: broadcaster, EventHandler: eventhandler}
}

// Informer creates an InformerSource with the given cache provider.
func Informer(informer cache.Informer, eventhandler handler.EventHandler) Source {
	return &internal.Informer{Informer: informer, EventHandler: eventhandler}
}

// StartFunc is a function that starts a goroutine and returns immediately.
type StartFunc = internal.StartFunc

// FuncOption is a functional option for configuring a Func source.
type FuncOption func(*internal.FuncOptions)

// WithShutdownFunc specifies the ShutdownFunc of the Func source.
func WithShutdownFunc(fn func() error) FuncOption {
	return func(o *internal.FuncOptions) {
		o.ShutdownFunc = fn
	}
}

// Func creates a FuncSource with the given function.
func Func(f StartFunc, options ...FuncOption) Source {
	opts := internal.FuncOptions{}
	for _, o := range options {
		if o == nil {
			continue // ignore nil options
		}
		o(&opts)
	}
	return &internal.FuncSource{StartFunc: f, Options: opts}
}
