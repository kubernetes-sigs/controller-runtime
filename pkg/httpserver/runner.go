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

package httpserver

import (
	"context"
	"fmt"
	"net"
	"net/http"

	logf "sigs.k8s.io/controller-runtime/pkg/internal/log"
)

var log = logf.RuntimeLog.WithName("httpserver")

// DefaultBindAddress sets the default bind address for the HTTP server listener
var DefaultBindAddress = ":8080"

// Options use to provides Runner creation options
type Options struct {
	BindAddress string
}

// server HTTP debug server
type server struct {
	bindAddress string

	mux *http.ServeMux
}

// New returns new Runner instance
func New(options Options) Server {
	return &server{
		bindAddress: options.BindAddress,
		mux:         http.NewServeMux(),
	}
}

// Handle registers the handler for the given pattern.
// If a handler already exists for pattern, Handle panics.
func (s *server) Handle(path string, handler http.Handler) {
	s.mux.Handle(path, handler)
}

// HandleFunc registers the handler function for the given pattern
// in the DefaultServeMux.
// The documentation for ServeMux explains how patterns are matched.
func (s *server) HandleFunc(path string, handlerFunc http.HandlerFunc) {
	s.mux.HandleFunc(path, handlerFunc)
}

// Start use to start the HTTP server
func (s *server) Start(stop <-chan struct{}) error {
	listener, err := newListener(s.bindAddress)
	if err != nil {
		return err
	}
	server := http.Server{
		Handler: s.mux,
	}
	// Run the server
	go func() {
		log.Info("starting http server")
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Error(err, "http server error")
		}
	}()

	// Shutdown the server when stop is close
	<-stop
	return server.Shutdown(context.Background())
}

// newListener creates a new TCP listener bound to the given address.
func newListener(addr string) (net.Listener, error) {
	if addr == "" {
		// If the http server bind address is empty, default to ":8080"
		addr = DefaultBindAddress
	}

	log.Info("debug server is starting to listen", "addr", addr)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		er := fmt.Errorf("error listening on %s: %v", addr, err)
		log.Error(er, "debug server failed to listen. You may want to disable the debug server or use another port if it is due to conflicts")
		return nil, er
	}
	return ln, nil
}
