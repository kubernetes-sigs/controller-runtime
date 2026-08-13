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

package server

import (
	"context"
	"crypto/tls"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	certutil "k8s.io/client-go/util/cert"
)

// TestCreateListener_LazyCertInit verifies that when cert files are absent at
// listener creation time, the server installs a lazy GetCertificate that
// causes handshakes to fail until the files appear, then succeeds once they do.
//
// This covers the race where a certificate provisioner (e.g. cert-controller,
// cert-manager) writes the cert files after the manager — and thus the metrics
// server listener — has already started.
func TestCreateListener_LazyCertInit(t *testing.T) {
	certDir := t.TempDir()
	certPath := filepath.Join(certDir, "tls.crt")
	keyPath := filepath.Join(certDir, "tls.key")

	srv := &defaultServer{
		options: Options{
			SecureServing: true,
			CertDir:       certDir,
			CertName:      "tls.crt",
			KeyName:       "tls.key",
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l, err := srv.createListener(ctx, logr.Discard())
	if err != nil {
		t.Fatalf("createListener: %v", err)
	}
	defer l.Close()

	// Simulate what net/http does: accept connections and explicitly complete
	// the TLS handshake before closing. The handshake triggers GetCertificate.
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				// net/http triggers the handshake on first read; do the same.
				if tc, ok := conn.(*tls.Conn); ok {
					_ = tc.Handshake()
				}
			}()
		}
	}()

	addr := l.Addr().String()
	dialCfg := &tls.Config{InsecureSkipVerify: true} //nolint:gosec

	// Before cert files exist: GetCertificate returns (nil, nil) which makes
	// the TLS library abort with "no certificates configured".
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", addr, dialCfg)
	if err == nil {
		conn.Close()
		t.Fatal("expected dial to fail before cert files exist; got successful connection")
	}

	// Write a self-signed cert to disk.
	cert, key, err := certutil.GenerateSelfSignedCertKey("localhost", []net.IP{{127, 0, 0, 1}}, nil)
	if err != nil {
		t.Fatalf("GenerateSelfSignedCertKey: %v", err)
	}
	if err := os.WriteFile(certPath, cert, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	// After cert files exist the lazy initializer picks them up on the next
	// handshake. Retry briefly to allow the goroutine scheduler to run.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err = tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", addr, dialCfg)
		if err == nil {
			conn.Close()
			return // success
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("cert files written but handshake still failing after 5s: %v", err)
}
