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
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	internal "sigs.k8s.io/controller-runtime/pkg/source/internal"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
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

// Kind creates a KindSource with the given cache provider.
func Kind[T client.Object](cache cache.Cache, object T, handler handler.TypedEventHandler[T], predicates ...predicate.TypedPredicate[T]) SyncingSource {
	return &internal.Kind[T]{
		Type:       object,
		Cache:      cache,
		Handler:    handler,
		Predicates: predicates,
	}
}

var _ Source = &internal.Kind[client.Object]{}
var _ SyncingSource = &internal.Kind[client.Object]{}

// Informer is used to provide a source of events originating inside the cluster from Watches (e.g. Pod Create).
type Informer = internal.Informer

var _ Source = &internal.Informer{}

type channelOpts[T any] struct {
	bufferSize *int
	predicates []predicate.TypedPredicate[T]
}

// ChannelOpt allows to configure a source.Channel.
type ChannelOpt[T any] func(*channelOpts[T])

// WithPredicates adds the configured predicates to a source.Channel.
func WithPredicates[T any](p ...predicate.TypedPredicate[T]) ChannelOpt[T] {
	return func(c *channelOpts[T]) {
		c.predicates = append(c.predicates, p...)
	}
}

// WithBufferSize configures the buffer size for a source.Channel. By
// default, the buffer size is 1024.
func WithBufferSize[T any](bufferSize int) ChannelOpt[T] {
	return func(c *channelOpts[T]) {
		c.bufferSize = &bufferSize
	}
}

// Channel is used to provide a source of events originating outside the cluster
// (e.g. GitHub Webhook callback).  Channel requires the user to wire the external
// source (e.g. http handler) to write GenericEvents to the underlying channel.
func Channel[T any](source <-chan event.TypedGenericEvent[T], handler handler.TypedEventHandler[T], opts ...ChannelOpt[T]) Source {
	c := &channelOpts[T]{}
	for _, opt := range opts {
		opt(c)
	}

	return &internal.Channel[T]{
		Source:     source,
		Handler:    handler,
		BufferSize: c.bufferSize,
		Predicates: c.predicates,
	}
}

var _ Source = &internal.Channel[client.Object]{}

// Func is a function that implements Source.
type Func = internal.Func

var _ Source = internal.Func(nil)
