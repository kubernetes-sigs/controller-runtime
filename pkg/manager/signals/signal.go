/*
Copyright 2017 The Kubernetes Authors.

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

package signals

import (
	"context"
	"os"
	"os/signal"
	"time"
)

var (
	onlyOneSignalHandler = make(chan struct{})
	// Define global signal channel for testing
	signalCh = make(chan os.Signal, 2)
)

// SetupSignalHandlerWithDelay registers for SIGTERM and SIGINT. A context is
// returned which is canceled on one of these signals after waiting for the
// specified delay. In particular, the delay can be used to give external
// Kubernetes controllers (such as kube-proxy) time to observe the termination
// of this manager before starting shutdown of any webhook servers to avoid
// receiving connection attempts after closing webhook listeners. If a second
// signal is caught, the program is terminated with exit code 1.
func SetupSignalHandlerWithDelay(delay time.Duration) context.Context {
	close(onlyOneSignalHandler) // panics when called twice

	ctx, cancel := context.WithCancel(context.Background())

	signal.Notify(signalCh, shutdownSignals...)
	go func() {
		<-signalCh
		// Cancel the context after delaying for the specified duration but
		// avoid blocking if a second signal is caught
		go func() {
			<-time.After(delay)
			cancel()
		}()
		<-signalCh
		os.Exit(1) // second signal. Exit directly.
	}()

	return ctx
}

// SetupSignalHandler is a special case of SetupSignalHandlerWithDelay with no
// delay for backwards compatibility
func SetupSignalHandler() context.Context {
	return SetupSignalHandlerWithDelay(time.Duration(0))
}
