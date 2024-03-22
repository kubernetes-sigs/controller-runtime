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
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// syncedPollPeriod controls how often you look at the status of your sync funcs
const syncedPollPeriod = 100 * time.Millisecond

// Kind is used to provide a source of events originating inside the cluster from Watches (e.g. Pod Create).
type Kind struct {
	// Type is the type of object to watch.  e.g. &v1.Pod{}
	Type client.Object
	// Cache used to watch APIs
	Cache cache.Cache

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

	// startupDoneCh will close once the startup goroutine has finished running. If the
	// startupErr is nil, the source has finished starting up and is ready to be used.
	startupDoneCh chan struct{}
	// startupErr contains an error if one was encountered during startup.
	startupErr atomic.Value

	// informer is the informer that we obtained from the cache.
	informer cache.Informer
	// registration is the registered EventHandler handle.
	registration toolscache.ResourceEventHandlerRegistration
}

type startupErr struct {
	isCanceled bool
	err        error
}

// Start is internal and should be called only by the Controller to start a source which enqueue reconcile.Requests.
// It should NOT block, instead, it should start a goroutine and return immediately.
// The context passed to Start can be used to cancel this goroutine. After the context is canceled, Shutdown can be
// called to wait for the goroutine to exit.
// Start can be called only once, it is thus not possible to share a single source between multiple controllers.
func (ks *Kind) Start(ctx context.Context, queue workqueue.RateLimitingInterface) error {
	if ks.Type == nil {
		return fmt.Errorf("must create Kind with a non-nil Type")
	}
	if ks.Cache == nil {
		return fmt.Errorf("must create Kind with a non-nil Cache")
	}
	if ks.EventHandler == nil {
		return fmt.Errorf("must create Kind with a non-nil EventHandler")
	}

	ks.mu.Lock()
	defer ks.mu.Unlock()
	if ks.isStarted {
		return fmt.Errorf("cannot start an already started Kind source")
	}
	if ks.shuttingDown {
		return nil
	}
	ks.isStarted = true

	ks.startupDoneCh = make(chan struct{})
	go func() {
		defer close(ks.startupDoneCh)

		err := ks.registerEventHandler(
			ctx,
			NewEventHandler(ctx, queue, ks.EventHandler).HandlerFuncs(),
		)
		if err != nil {
			ks.startupErr.Store(startupErr{
				isCanceled: errors.Is(ctx.Err(), context.Canceled),
				err:        err,
			})
		}
	}()

	return nil
}

func getInformer(ctx context.Context, informerCache cache.Cache, resourceType client.Object) (cache.Informer, error) {
	var informer cache.Informer

	// Tries to get an informer until it returns true,
	// an error or the specified context is cancelled or expired.
	if err := wait.PollUntilContextCancel(ctx, 10*time.Second, true, func(pollCtx context.Context) (bool, error) {
		// Lookup the Informer from the Cache.
		var err error
		informer, err = informerCache.GetInformer(pollCtx, resourceType, cache.BlockUntilSynced(false))
		if err != nil {
			kindMatchErr := &meta.NoKindMatchError{}
			discoveryErr := &discovery.ErrGroupDiscoveryFailed{}
			switch {
			case errors.As(err, &kindMatchErr):
				// We got a NoKindMatchError, which means the kind is a CRD and it's not installed yet.
				// We should retry until it's installed.
				log.Error(err, "waiting for CRD to be installed", "groupKind", kindMatchErr.GroupKind)
				return false, nil // Retry.
			case errors.As(err, &discoveryErr):
				// We got a ErrGroupDiscoveryFailed, which means the kind is a CRD and it's not installed yet.
				// We should retry until it's installed.
				for gv, err := range discoveryErr.Groups {
					log.Error(err, "waiting for CRD to be installed", "groupVersion", gv)
				}
				return false, nil // Retry.
			case runtime.IsNotRegisteredError(err):
				// We got a IsNotRegisteredError, which means the kind is not registered to the Scheme.
				// This is a programming error, so we should stop retrying and return the error.
				return true, fmt.Errorf("kind must be registered to the Scheme: %w", err)
			default:
				// We got an error that is not a NoKindMatchError or IsNotRegisteredError, so we should
				// stop retrying and return the error.
				return true, fmt.Errorf("failed to get informer from cache: %w", err)
			}
		}
		return true, nil
	}); err != nil {
		return nil, err
	}

	return informer, nil
}

func (ks *Kind) registerEventHandler(ctx context.Context, eventHandler toolscache.ResourceEventHandlerFuncs) error {
	informer, err := getInformer(ctx, ks.Cache, ks.Type)
	if err != nil {
		return err
	}

	// We want to prevent unnecessarily registering the eventHandler after
	// the source has been shut down. Then we return early.
	if func() bool {
		ks.mu.RLock()
		defer ks.mu.RUnlock()

		return ks.shuttingDown
	}() {
		return nil
	}

	// Save the informer so that we use it to unregister the event handler in Stop.
	ks.informer = informer

	// Register the event handler and save the registration so that we can unregister it in Stop.
	registration, err := ks.informer.AddEventHandler(eventHandler)
	if err != nil {
		return err
	}
	ks.registration = registration

	return nil
}

func (ks *Kind) String() string {
	if ks.Type != nil {
		return fmt.Sprintf("kind source: %T", ks.Type)
	}
	return "kind source: unknown type"
}

// WaitForSync implements SyncingSource to allow controllers to wait with starting
// workers until the cache is synced.
//
// WaitForSync blocks until the cache is synced or the passed context is canceled.
// If the passed context is canceled, with a non-context.Canceled error, WaitForSync
// will return an error. Also, when the cache is stopped before it is synced, this
// function will remain blocked until the passed context is canceled.
func (ks *Kind) WaitForSync(ctx context.Context) error {
	if func() bool {
		ks.mu.RLock()
		defer ks.mu.RUnlock()

		return !ks.isStarted
	}() {
		return fmt.Errorf("cannot wait for sync on an unstarted source")
	}

	select {
	case <-ctx.Done():
		// do nothing
	case <-ks.startupDoneCh:
		// The startup goroutine has finished running.

		// If the startup was successful, we will wait for the cache to be synced.
		if startErr, _ := ks.startupErr.Load().(startupErr); startErr.err == nil {
			if err := wait.PollUntilContextCancel(ctx, syncedPollPeriod, true, func(_ context.Context) (bool, error) {
				return ks.registration.HasSynced(), nil
			}); err != nil {
				return fmt.Errorf("informer did not sync in time: %w", err)
			}

			// Happy path: the startup goroutine returned without an error and the cache was synced.
			return nil
		}
	}

	// Wait for the context to be cancelled (in case the startup was completed with an error).
	<-ctx.Done()

	// Return a timeout error if the context was not cancelled.
	if !errors.Is(ctx.Err(), context.Canceled) {
		return fmt.Errorf("timed out trying to get an informer from cache for Kind %T", ks.Type)
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
func (ks *Kind) Shutdown() error {
	if func() bool {
		ks.mu.Lock()
		defer ks.mu.Unlock()

		// Ensure that when we release the lock, we stop an in-process & future calls to Start().
		ks.shuttingDown = true

		return !ks.isStarted
	}() {
		return nil
	}

	// Wait for the started channel to be closed.
	<-ks.startupDoneCh

	var errs []error

	// Check if we have any startup errors. We ignore the errors if the context was canceled,
	// because it means that the source was stopped by the user.
	if startErr, _ := ks.startupErr.Load().(startupErr); startErr.err != nil && !startErr.isCanceled {
		errs = append(errs, fmt.Errorf("failed to start source: %w", startErr.err))
	}

	// Remove event handler if it was registered.
	if ks.informer != nil && ks.registration != nil {
		if err := ks.informer.RemoveEventHandler(ks.registration); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop source: %w", err))
		}
	}

	return kerrors.NewAggregate(errs)
}
