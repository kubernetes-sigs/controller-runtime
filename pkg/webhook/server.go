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

package webhook

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"path"
	"strconv"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
)

const (
	certName = "tls.crt"
	keyName  = "tls.key"
)

// ServerOptions are options for configuring an admission webhook server.
type ServerOptions struct {
	// Address that the server will listen on.
	// Defaults to "" - all addresses.
	Host string

	// Port is the port number that the server will serve.
	// It will be defaulted to 443 if unspecified.
	Port int32

	// CertDir is the directory that contains the server key and certificate.
	// If using FSCertWriter in Provisioner, the server itself will provision the certificate and
	// store it in this directory.
	// If using SecretCertWriter in Provisioner, the server will provision the certificate in a secret,
	// the user is responsible to mount the secret to the this location for the server to consume.
	CertDir string
}

// Server is an admission webhook server that can serve traffic and
// generates related k8s resources for deploying.
type Server struct {
	// ServerOptions contains options for configuring the admission server.
	ServerOptions

	sMux *http.ServeMux
	// registry maps a path to a http.Handler.
	registry map[string]http.Handler

	// setFields allows injecting dependencies from an external source
	setFields inject.Func

	once sync.Once
}

// Webhook defines the basics that a webhook should support.
type Webhook interface {
	// Webhooks handle HTTP requests.
	http.Handler

	// GetPath returns the path that the webhook registered.
	GetPath() string
	// Validate validates if the webhook itself is valid.
	// If invalid, a non-nil error will be returned.
	Validate() error
}

// NewServer creates a new admission webhook server.
func NewServer(options ServerOptions) (*Server, error) {
	as := &Server{
		sMux:          http.NewServeMux(),
		registry:      map[string]http.Handler{},
		ServerOptions: options,
	}

	return as, nil
}

// setDefault does defaulting for the Server.
func (s *Server) setDefault() {
	if s.registry == nil {
		s.registry = map[string]http.Handler{}
	}
	if s.sMux == nil {
		s.sMux = http.NewServeMux()
	}
	if s.Port <= 0 {
		s.Port = 443
	}
	if len(s.CertDir) == 0 {
		s.CertDir = path.Join("k8s-webhook-server", "cert")
	}
}

// Register validates and registers webhook(s) in the server
func (s *Server) Register(webhooks ...Webhook) error {
	for i, webhook := range webhooks {
		// validate the webhook before registering it.
		err := webhook.Validate()
		if err != nil {
			return err
		}
		// TODO(directxman12): call setfields if we've already started the server
		_, found := s.registry[webhook.GetPath()]
		if found {
			return fmt.Errorf("can't register duplicate path: %v", webhook.GetPath())
		}
		s.registry[webhook.GetPath()] = webhooks[i]
		s.sMux.Handle(webhook.GetPath(), webhook)
	}

	return nil
}

// Handle registers a http.Handler for the given pattern.
func (s *Server) Handle(pattern string, handler http.Handler) {
	s.sMux.Handle(pattern, handler)
}

// Start runs the server.
// It will install the webhook related resources depend on the server configuration.
func (s *Server) Start(stop <-chan struct{}) error {
	s.once.Do(s.setDefault)

	// inject fields here as opposed to in Register so that we're certain to have our setFields
	// function available.
	for _, webhook := range s.registry {
		if err := s.setFields(webhook); err != nil {
			return err
		}
	}

	// TODO: watch the cert dir. Reload the cert if it changes
	cert, err := tls.LoadX509KeyPair(path.Join(s.CertDir, certName), path.Join(s.CertDir, keyName))
	if err != nil {
		return err
	}

	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	listener, err := tls.Listen("tcp", net.JoinHostPort(s.Host, strconv.Itoa(int(s.Port))), cfg)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Handler: s.sMux,
	}

	idleConnsClosed := make(chan struct{})
	go func() {
		<-stop

		// TODO: use a context with reasonable timeout
		if err := srv.Shutdown(context.Background()); err != nil {
			// Error from closing listeners, or context timeout
			log.Error(err, "error shutting down the HTTP server")
		}
		close(idleConnsClosed)
	}()

	err = srv.Serve(listener)
	if err != nil && err != http.ErrServerClosed {
		return err
	}

	<-idleConnsClosed
	return nil
}

// InjectFunc injects the field setter into the server.
func (s *Server) InjectFunc(f inject.Func) error {
	s.setFields = f
	return nil
}
