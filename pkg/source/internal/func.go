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
)

// StartFunc is a function that starts a goroutine and returns immediately.
type StartFunc func(context.Context, workqueue.RateLimitingInterface) error

// FuncOptions contains the options for the Func source.
type FuncOptions struct {
	// ShutdownFunc is an optional function that is called when the source
	// is shutting down. It should block until all goroutines have finished.
	ShutdownFunc func() error
}

// FuncSource is a source which is implemented by the provided Func.
type FuncSource struct {
	StartFunc StartFunc
	Options   FuncOptions

	mu sync.RWMutex
	// shuttingDown is true if the source has been shuttingDown and causes any following/ in-progress
	// Start calls to no-op.
	shuttingDown bool
	// isStarted is true if the source has been started. A source can only be started once.
	isStarted bool
}

// Start is internal and should be called only by the Controller to start a source which
// enqueues reconcile.Requests.
// It should NOT block, instead, it should start a goroutine and return immediately. The
// context passed to Start can be used to cancel this goroutine. Shutdown can be called
// to wait for the goroutine to exit.
// Start can be called only once, it is thus not possible to share a single source between
// multiple controllers.
func (f *FuncSource) Start(ctx context.Context, queue workqueue.RateLimitingInterface) error {
	if f.StartFunc == nil {
		return fmt.Errorf("must create FuncSource with a non-nil StartFunc")
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.isStarted {
		return fmt.Errorf("cannot start an already started source")
	}
	if f.shuttingDown {
		return nil
	}
	f.isStarted = true

	return f.StartFunc(ctx, queue)
}

func (f *FuncSource) String() string {
	return "Func"
}

// Shutdown marks a Source as shutting down. At that point the source cannot
// be started anymore, Start will no-op and return immediately.
//
// In addition, Shutdown blocks until all goroutines have finished. This must
// happen either before Shutdown gets called or while it is waiting. To trigger
// the termination of the goroutines, the context passed to Start should be
// canceled.
//
// Shutdown may be called multiple times, even concurrently. All such calls will
// block until all goroutines have terminated.
func (f *FuncSource) Shutdown() error {
	if func() bool {
		f.mu.RLock()
		defer f.mu.RUnlock()

		// Ensure that when we release the lock, we stop an in-process & future calls to Start().
		f.shuttingDown = true

		return !f.isStarted
	}() {
		return nil
	}

	if f.Options.ShutdownFunc == nil {
		return nil
	}

	return f.Options.ShutdownFunc()
}
