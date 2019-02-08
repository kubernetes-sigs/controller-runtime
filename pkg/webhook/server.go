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

// Server is an admission webhook server that can serve traffic and
// generates related k8s resources for deploying.
type Server struct {
	// Host is the address that the server will listen on.
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

	// TODO(directxman12): should we make the mux configurable?

	// webhookMux is the multiplexer that handles different webhooks.
	webhookMux *http.ServeMux
	// webhooks keep track of all registered webhooks for dependency injection,
	// and to provide better panic messages on duplicate webhook registration.
	webhooks map[string]Webhook

	// setFields allows injecting dependencies from an external source
	setFields inject.Func

	// defaultingOnce ensures that the default fields are only ever set once.
	defaultingOnce sync.Once
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

// setDefaults does defaulting for the Server.
func (s *Server) setDefaults() {
	s.webhooks = map[string]Webhook{}
	s.webhookMux = http.NewServeMux()

	if s.Port <= 0 {
		s.Port = 443
	}
	if len(s.CertDir) == 0 {
		s.CertDir = path.Join("/tmp", "k8s-webhook-server", "serving-certs")
	}
}

// Register validates and registers webhook(s) in the server
func (s *Server) Register(webhooks ...Webhook) error {
	// TODO(directxman12): is it really worth the ergonomics hit to make this a catchable error?
	// In most cases you'll probably just bail immediately anyway.
	for i, webhook := range webhooks {
		// validate the webhook before registering it.
		err := webhook.Validate()
		if err != nil {
			return err
		}
		// TODO(directxman12): call setfields if we've already started the server
		_, found := s.webhooks[webhook.GetPath()]
		if found {
			return fmt.Errorf("can't register duplicate path: %v", webhook.GetPath())
		}
		s.webhooks[webhook.GetPath()] = webhooks[i]
		s.webhookMux.Handle(webhook.GetPath(), webhook)
	}

	return nil
}

// Start runs the server.
// It will install the webhook related resources depend on the server configuration.
func (s *Server) Start(stop <-chan struct{}) error {
	s.defaultingOnce.Do(s.setDefaults)

	// inject fields here as opposed to in Register so that we're certain to have our setFields
	// function available.
	for _, webhook := range s.webhooks {
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
		Handler: s.webhookMux,
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
