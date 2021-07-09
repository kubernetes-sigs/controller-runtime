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

package webhook_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var _ = Describe("Webhook Server", func() {
	var (
		ctx          context.Context
		ctxCancel    context.CancelFunc
		testHostPort string
		client       *http.Client
		server       *webhook.Server
		servingOpts  envtest.WebhookInstallOptions
	)

	BeforeEach(func() {
		ctx, ctxCancel = context.WithCancel(context.Background())
		// closed in individual tests differently

		servingOpts = envtest.WebhookInstallOptions{}
		Expect(servingOpts.PrepWithoutInstalling()).To(Succeed())

		testHostPort = net.JoinHostPort(servingOpts.LocalServingHost, fmt.Sprintf("%d", servingOpts.LocalServingPort))

		// bypass needing to set up the x509 cert pool, etc ourselves
		clientTransport, err := rest.TransportFor(&rest.Config{
			TLSClientConfig: rest.TLSClientConfig{CAData: servingOpts.LocalServingCAData},
		})
		Expect(err).NotTo(HaveOccurred())
		client = &http.Client{
			Transport: clientTransport,
		}

		server = &webhook.Server{
			Host:    servingOpts.LocalServingHost,
			Port:    servingOpts.LocalServingPort,
			CertDir: servingOpts.LocalServingCertDir,
		}
	})
	AfterEach(func() {
		Expect(servingOpts.Cleanup()).To(Succeed())
	})

	genericStartServer := func(f func(ctx context.Context)) (done <-chan struct{}) {
		doneCh := make(chan struct{})
		go func() {
			defer GinkgoRecover()
			defer close(doneCh)
			f(ctx)
		}()
		// wait till we can ping the server to start the test
		Eventually(func() error {
			_, err := client.Get(fmt.Sprintf("https://%s/unservedpath", testHostPort))
			return err
		}).Should(Succeed())

		// this is normally called before Start by the manager
		Expect(server.InjectFunc(func(i interface{}) error {
			boolInj, canInj := i.(interface{ InjectBool(bool) error })
			if !canInj {
				return nil
			}
			return boolInj.InjectBool(true)
		})).To(Succeed())

		return doneCh
	}

	startServer := func() (done <-chan struct{}) {
		return genericStartServer(func(ctx context.Context) {
			Expect(server.Start(ctx)).To(Succeed())
		})
	}

	// TODO(directxman12): figure out a good way to test all the serving setup
	// with httptest.Server to get all the niceness from that.

	Context("when serving", func() {
		PIt("should verify the client CA name when asked to", func() {

		})
		PIt("should support HTTP/2", func() {

		})

		// TODO(directxman12): figure out a good way to test the port default, etc
	})

	It("should panic if a duplicate path is registered", func() {
		server.Register("/somepath", &testHandler{})
		doneCh := startServer()

		Expect(func() { server.Register("/somepath", &testHandler{}) }).To(Panic())

		ctxCancel()
		Eventually(doneCh, "4s").Should(BeClosed())
	})

	Context("when registering new webhooks before starting", func() {
		It("should serve a webhook on the requested path", func() {
			server.Register("/somepath", &testHandler{})

			Expect(server.StartedChecker()(nil)).ToNot(Succeed())

			doneCh := startServer()

			Eventually(func() ([]byte, error) {
				resp, err := client.Get(fmt.Sprintf("https://%s/somepath", testHostPort))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()
				return ioutil.ReadAll(resp.Body)
			}).Should(Equal([]byte("gadzooks!")))

			Expect(server.StartedChecker()(nil)).To(Succeed())

			ctxCancel()
			Eventually(doneCh, "4s").Should(BeClosed())
		})

		It("should inject dependencies eventually, given an inject func is eventually provided", func() {
			handler := &testHandler{}
			server.Register("/somepath", handler)
			doneCh := startServer()

			Eventually(func() bool { return handler.injectedField }).Should(BeTrue())

			ctxCancel()
			Eventually(doneCh, "4s").Should(BeClosed())
		})
	})

	Context("when registering webhooks after starting", func() {
		var (
			doneCh <-chan struct{}
		)
		BeforeEach(func() {
			doneCh = startServer()
		})
		AfterEach(func() {
			// wait for cleanup to happen
			ctxCancel()
			Eventually(doneCh, "4s").Should(BeClosed())
		})

		It("should serve a webhook on the requested path", func() {
			server.Register("/somepath", &testHandler{})
			resp, err := client.Get(fmt.Sprintf("https://%s/somepath", testHostPort))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(ioutil.ReadAll(resp.Body)).To(Equal([]byte("gadzooks!")))
		})

		It("should inject dependencies, if an inject func has been provided already", func() {
			handler := &testHandler{}
			server.Register("/somepath", handler)
			Expect(handler.injectedField).To(BeTrue())
		})
	})

	It("should serve be able to serve in unmanaged mode", func() {
		server = &webhook.Server{
			Host:    servingOpts.LocalServingHost,
			Port:    servingOpts.LocalServingPort,
			CertDir: servingOpts.LocalServingCertDir,
		}
		server.Register("/somepath", &testHandler{})
		doneCh := genericStartServer(func(ctx context.Context) {
			Expect(server.StartStandalone(ctx, scheme.Scheme))
		})

		Eventually(func() ([]byte, error) {
			resp, err := client.Get(fmt.Sprintf("https://%s/somepath", testHostPort))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			return ioutil.ReadAll(resp.Body)
		}).Should(Equal([]byte("gadzooks!")))

		ctxCancel()
		Eventually(doneCh, "4s").Should(BeClosed())
	})
})

type testHandler struct {
	injectedField bool
}

func (t *testHandler) InjectBool(val bool) error {
	t.injectedField = val
	return nil
}
func (t *testHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	if _, err := resp.Write([]byte("gadzooks!")); err != nil {
		panic("unable to write http response!")
	}
}
