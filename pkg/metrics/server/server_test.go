/*
Copyright The Kubernetes Authors.

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
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/sys/unix"
	certutil "k8s.io/client-go/util/cert"

	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
)

func TestServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metrics Server Suite")
}

var _ = Describe("handlerOpts", func() {
	type testCase struct {
		server                    defaultServer
		expectedErrorHandling     promhttp.HandlerErrorHandling
		expectedEnableOpenMetrics bool
	}

	DescribeTable("returns the expected handler options",
		func(tc *testCase) {
			opts := tc.server.handlerOpts()

			Expect(opts.ErrorHandling).To(Equal(tc.expectedErrorHandling))
			Expect(opts.EnableOpenMetrics).To(Equal(tc.expectedEnableOpenMetrics))
		},
		Entry("with defaults", &testCase{
			server:                    defaultServer{},
			expectedErrorHandling:     promhttp.HTTPErrorOnError,
			expectedEnableOpenMetrics: true,
		}),
		Entry("with overrides", &testCase{
			server: defaultServer{
				options: Options{
					HandlerOpts: []func(*promhttp.HandlerOpts){
						func(opts *promhttp.HandlerOpts) {
							opts.ErrorHandling = promhttp.ContinueOnError
							opts.EnableOpenMetrics = false
						},
					},
				},
			},
			expectedErrorHandling:     promhttp.ContinueOnError,
			expectedEnableOpenMetrics: false,
		}),
	)
})

var _ = Describe("metrics server certificate watcher", func() {
	It("returns nil when the server context is cancelled", func(specCtx SpecContext) {
		ctx, cancel := context.WithCancel(specCtx)
		defer cancel()

		dir := GinkgoT().TempDir()
		certPEM, keyPEM, err := certutil.GenerateSelfSignedCertKey("localhost", nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(filepath.Join(dir, "tls.crt"), certPEM, 0o600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(dir, "tls.key"), keyPEM, 0o600)).To(Succeed())

		srv, err := NewServer(Options{
			SecureServing: true,
			BindAddress:   "127.0.0.1:0",
			CertDir:       dir,
		}, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		errCh := make(chan error, 1)
		go func() {
			defer GinkgoRecover()
			errCh <- srv.Start(ctx)
		}()

		Eventually(srv.(*defaultServer).GetBindAddr, 5*time.Second).ShouldNot(BeEmpty())
		cancel()
		Eventually(errCh, 5*time.Second).Should(Receive(BeNil()))
	})

	// Registration retries for 10s, so this outlives that poll.
	It("returns certwatcher.ErrFailed when certificate watches cannot be registered", SpecTimeout(20*time.Second), func(specCtx SpecContext) {
		ctx, cancel := context.WithCancel(specCtx)
		defer cancel()

		dir := GinkgoT().TempDir()
		certPEM, keyPEM, err := certutil.GenerateSelfSignedCertKey("localhost", nil, nil)
		Expect(err).NotTo(HaveOccurred())
		// FIFOs let New read the certificate bytes, then the paths are removed
		// before fsnotify can watch them.
		Expect(unix.Mkfifo(filepath.Join(dir, "tls.crt"), 0o600)).To(Succeed())
		Expect(unix.Mkfifo(filepath.Join(dir, "tls.key"), 0o600)).To(Succeed())

		srv, err := NewServer(Options{
			SecureServing: true,
			BindAddress:   "127.0.0.1:0",
			CertDir:       dir,
		}, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		errCh := make(chan error, 1)
		go func() {
			defer GinkgoRecover()
			errCh <- srv.Start(ctx)
		}()

		writeFIFO(filepath.Join(dir, "tls.crt"), certPEM)
		writeFIFO(filepath.Join(dir, "tls.key"), keyPEM)

		Eventually(errCh, 15*time.Second).Should(Receive(MatchError(certwatcher.ErrFailed)))
	})
})

// writeFIFO blocks until a reader opens path, removes the path, then writes data
// to the already-open descriptor.
func writeFIFO(path string, data []byte) {
	opened := make(chan *os.File, 1)
	openErr := make(chan error, 1)
	go func() {
		// blocks until first file read in `Start`
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			openErr <- err
			return
		}
		opened <- f
	}()

	var f *os.File
	select {
	case f = <-opened:
	case err := <-openErr:
		Expect(err).NotTo(HaveOccurred())
	case <-time.After(5 * time.Second):
		Fail(fmt.Sprintf("timed out waiting for a reader on %s", path))
	}

	// remove path here so fsnotify fails
	Expect(os.Remove(path)).To(Succeed())
	_, err := f.Write(data)
	Expect(err).NotTo(HaveOccurred())
	Expect(f.Close()).To(Succeed())
}
