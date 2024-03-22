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
	"errors"
	"fmt"
	"sync"

	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// Informer is used to provide a source of events originating inside the cluster from Watches (e.g. Pod Create).
type Informer struct {
	// Informer is the controller-runtime Informer
	Informer cache.Informer

	// EventHandler is the handler to call when events are received.
	EventHandler handler.EventHandler

	mu sync.RWMutex
	// isStarted is true if Start has been called.
	// - A source can only be started once.
	// - WaitForSync can only be called after the source is started.
	// - Shutdown will exit early if the source was never started.
	isStarted bool
	// shuttingDown is true if Shutdown has been called.
	// - If true, Start will exit early.
	shuttingDown bool

	// startupErr may contain an error if one was encountered during startup.
	startupErr startupErr

	// registration is the registered EventHandler handle.
	registration toolscache.ResourceEventHandlerRegistration
}

// Start is internal and should be called only by the Controller to start a source which enqueue reconcile.Requests.
// It should NOT block, instead, it should start a goroutine and return immediately.
// The context passed to Start can be used to cancel this goroutine. After the context is canceled, Shutdown can be
// called to wait for the goroutine to exit.
// Start can be called only once, it is thus not possible to share a single source between multiple controllers.
func (is *Informer) Start(ctx context.Context, queue workqueue.RateLimitingInterface) error {
	if is.Informer == nil {
		return fmt.Errorf("must create Informer with a non-nil Informer")
	}
	if is.EventHandler == nil {
		return fmt.Errorf("must create Informer with a non-nil EventHandler")
	}

	is.mu.Lock()
	defer is.mu.Unlock()
	if is.isStarted {
		return fmt.Errorf("cannot start an already started Informer source")
	}
	if is.shuttingDown {
		return nil
	}
	is.isStarted = true

	registration, err := is.Informer.AddEventHandler(NewEventHandler(ctx, queue, is.EventHandler).HandlerFuncs())
	if err != nil {
		is.startupErr = startupErr{
			isCanceled: errors.Is(ctx.Err(), context.Canceled),
			err:        fmt.Errorf("failed to add EventHandler to informer: %w", err),
		}
	}
	is.registration = registration

	return nil
}

func (is *Informer) String() string {
	return fmt.Sprintf("informer source: %p", is.Informer)
}

// WaitForSync implements SyncingSource to allow controllers to wait with starting
// workers until the cache is synced.
//
// WaitForSync blocks until the cache is synced or the passed context is canceled.
// If the passed context is canceled, with a non-context.Canceled error, WaitForSync
// will return an error. Also, when the cache is stopped before it is synced, this
// function will remain blocked until the passed context is canceled.
func (is *Informer) WaitForSync(ctx context.Context) error {
	if func() bool {
		is.mu.RLock()
		defer is.mu.RUnlock()

		return !is.isStarted
	}() {
		return fmt.Errorf("cannot wait for sync on an unstarted source")
	}

	// If the startup function was successful, we will wait for the cache to sync.
	if is.startupErr.err == nil {
		if err := wait.PollUntilContextCancel(ctx, syncedPollPeriod, true, func(_ context.Context) (bool, error) {
			return is.registration.HasSynced(), nil
		}); err != nil {
			return fmt.Errorf("informer did not sync in time: %w", err)
		}

		// Happy path: the cache has synced.
		return nil
	}

	// We got a startup error, so we will block until the context is canceled.
	<-ctx.Done()

	// Return a timeout error if the context was not cancelled.
	if !errors.Is(ctx.Err(), context.Canceled) {
		return fmt.Errorf("timed out waiting for informer that failed to start")
	}

	// If the context was cancelled, we return nil.
	// This might happen if the controller is stopped or just the WaitForSync call is cancelled.
	// In both cases, the Shutdown method should be used to retrieve errors and clean up resources.
	return nil
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
func (is *Informer) Shutdown() error {
	if func() bool {
		is.mu.RLock()
		defer is.mu.RUnlock()

		// Ensure that when we release the lock, we stop an in-process & future calls to Start().
		is.shuttingDown = true

		return !is.isStarted
	}() {
		return nil
	}

	var errs []error

	// Check if we have any startup errors. We ignore the errors if the context was canceled, because it means that the
	// source was stopped by the user.
	if is.startupErr.err != nil && !is.startupErr.isCanceled {
		errs = append(errs, fmt.Errorf("failed to start source: %w", is.startupErr.err))
	}

	// Remove event handler if it was registered.
	if is.Informer != nil && is.registration != nil {
		if err := is.Informer.RemoveEventHandler(is.registration); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop source: %w", err))
		}
	}

	return kerrors.NewAggregate(errs)
}
