/*
Copyright 2024 The Kubernetes Authors.

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

package interfaces

import (
	"context"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/handler"
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

// PrepareSource - Prepares a Source to be used with EventHandler and predicates
type PrepareSource interface {
	Prepare(handler.EventHandler, ...predicate.Predicate) SyncingSource
}

// PrepareSourceObject - Prepares a Source preserving the object type
type PrepareSourceObject[T any] interface {
	PrepareObject(handler.ObjectHandler[T], ...predicate.ObjectPredicate[T]) SyncingSource
}

// Syncing allows to wait for synchronization with context
type Syncing interface {
	WaitForSync(ctx context.Context) error
}

// SyncingSource is a source that needs syncing prior to being usable. The controller
// will call its WaitForSync prior to starting workers.
type SyncingSource interface {
	Source
	Syncing
}

// PrepareSyncing - a SyncingSource that also implements SourcePrepare and has WaitForSync method
type PrepareSyncing interface {
	SyncingSource
	PrepareSource
}

// PrepareSyncingObject - a SyncingSource that also implements PrepareSourceObject[T] and has WaitForSync method
type PrepareSyncingObject[T any] interface {
	SyncingSource
	PrepareSourceObject[T]
}
