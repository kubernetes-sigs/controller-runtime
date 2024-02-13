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
	"sigs.k8s.io/controller-runtime/pkg/predicate"
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
	// Start is internal and should be called only by the Controller to register an EventHandler with the Informer
	// to enqueue reconcile.Requests.
	Start(context.Context, handler.EventHandler, workqueue.RateLimitingInterface, ...predicate.Predicate) error
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
var _ Source = &internal.Channel{}
var _ Source = internal.Func(nil)

// Kind creates a KindSource with the given cache provider.
func Kind(cache cache.Cache, object client.Object) SyncingSource {
	return &internal.Kind{Cache: cache, Type: object}
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
func Channel(broadcaster *internal.ChannelBroadcaster, options ...ChannelOption) Source {
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

	return &internal.Channel{Options: opts, Broadcaster: broadcaster}
}

// Informer creates an InformerSource with the given cache provider.
func Informer(informer cache.Informer) Source {
	return &internal.Informer{Informer: informer}
}

// Func creates a FuncSource with the given function.
func Func(f internal.Func) Source {
	return f
}
