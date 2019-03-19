/*
Copyright 2019 The Kubernetes Authors.

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

package metrics

import (
	"context"
	"net"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ServeMetrics serves the metrics until the stop channel is closed.
// It serves the metrics on `/metrics` on the port configured through the MetricsBindAddress.
func ServeMetrics(listener net.Listener, stop <-chan struct{}) error {
	handler := promhttp.HandlerFor(Registry, promhttp.HandlerOpts{
		ErrorHandling: promhttp.HTTPErrorOnError,
	})
	// TODO(JoelSpeed): Use existing Kubernetes machinery for serving metrics
	mux := http.NewServeMux()
	mux.Handle("/metrics", handler)
	server := http.Server{
		Handler: mux,
	}

	errc := make(chan error, 0)
	// Run the server
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errc <- err
		}
	}()

	// Shutdown the server when stop is closed and catch any errors.
	select {
	case err := <-errc:
		return err
	case <-stop:
		if err := server.Shutdown(context.Background()); err != nil {
			return err
		}
	}

	return nil
}
