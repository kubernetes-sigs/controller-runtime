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

package manager

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	rt "runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/leaderelection"
	fakeleaderelection "sigs.k8s.io/controller-runtime/pkg/leaderelection/fake"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/recorder"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
)

var _ = Describe("manger.Manager", func() {
	var stop chan struct{}

	BeforeEach(func() {
		stop = make(chan struct{})
	})

	AfterEach(func() {
		close(stop)
	})

	Describe("New", func() {
		It("should return an error if there is no Config", func() {
			m, err := New(nil, Options{})
			Expect(m).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("must specify Config"))

		})

		It("should return an error if it can't create a RestMapper", func() {
			expected := fmt.Errorf("expected error: RestMapper")
			m, err := New(cfg, Options{
				MapperProvider: func(c *rest.Config) (meta.RESTMapper, error) { return nil, expected },
			})
			Expect(m).To(BeNil())
			Expect(err).To(Equal(expected))

		})

		It("should return an error it can't create a client.Client", func(done Done) {
			m, err := New(cfg, Options{
				NewClient: func(cache cache.Cache, config *rest.Config, options client.Options) (client.Client, error) {
					return nil, fmt.Errorf("expected error")
				},
			})
			Expect(m).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected error"))

			close(done)
		})

		It("should return an error it can't create a cache.Cache", func(done Done) {
			m, err := New(cfg, Options{
				NewCache: func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
					return nil, fmt.Errorf("expected error")
				},
			})
			Expect(m).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected error"))

			close(done)
		})

		It("should create a client defined in by the new client function", func(done Done) {
			m, err := New(cfg, Options{
				NewClient: func(cache cache.Cache, config *rest.Config, options client.Options) (client.Client, error) {
					return nil, nil
				},
			})
			Expect(m).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())
			Expect(m.GetClient()).To(BeNil())

			close(done)
		})

		It("should return an error it can't create a recorder.Provider", func(done Done) {
			m, err := New(cfg, Options{
				newRecorderProvider: func(config *rest.Config, scheme *runtime.Scheme, logger logr.Logger, broadcaster record.EventBroadcaster) (recorder.Provider, error) {
					return nil, fmt.Errorf("expected error")
				},
			})
			Expect(m).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected error"))

			close(done)
		})

		It("should lazily initialize a webhook server if needed", func(done Done) {
			By("creating a manager with options")
			m, err := New(cfg, Options{Port: 9443, Host: "foo.com"})
			Expect(err).NotTo(HaveOccurred())
			Expect(m).NotTo(BeNil())

			By("checking options are passed to the webhook server")
			svr := m.GetWebhookServer()
			Expect(svr).NotTo(BeNil())
			Expect(svr.Port).To(Equal(9443))
			Expect(svr.Host).To(Equal("foo.com"))

			close(done)
		})

		Context("with leader election enabled", func() {
			It("should only cancel the leader election after all runnables are done", func() {
				m, err := New(cfg, Options{
					LeaderElection:          true,
					LeaderElectionNamespace: "default",
					LeaderElectionID:        "test-leader-election-id-2",
					HealthProbeBindAddress:  "0",
					MetricsBindAddress:      "0",
				})
				Expect(err).To(BeNil())

				runnableDone := make(chan struct{})
				slowRunnable := RunnableFunc(func(s <-chan struct{}) error {
					<-s
					time.Sleep(100 * time.Millisecond)
					close(runnableDone)
					return nil
				})
				Expect(m.Add(slowRunnable)).To(BeNil())

				cm := m.(*controllerManager)
				cm.gracefulShutdownTimeout = time.Second
				leaderElectionDone := make(chan struct{})
				cm.onStoppedLeading = func() {
					close(leaderElectionDone)
				}

				mgrStopChan := make(chan struct{})
				mgrDone := make(chan struct{})
				go func() {
					defer GinkgoRecover()
					Expect(m.Start(mgrStopChan)).To(BeNil())
					close(mgrDone)
				}()
				<-cm.elected
				close(mgrStopChan)
				select {
				case <-leaderElectionDone:
					Expect(errors.New("leader election was cancelled before runnables were done")).ToNot(HaveOccurred())
				case <-runnableDone:
					// Success
				}
				// Don't leak routines
				<-mgrDone

			})
			It("should disable gracefulShutdown when stopping to lead", func() {
				m, err := New(cfg, Options{
					LeaderElection:          true,
					LeaderElectionNamespace: "default",
					LeaderElectionID:        "test-leader-election-id-3",
					HealthProbeBindAddress:  "0",
					MetricsBindAddress:      "0",
				})
				Expect(err).To(BeNil())

				mgrDone := make(chan struct{})
				go func() {
					defer GinkgoRecover()
					err := m.Start(make(chan struct{}))
					Expect(err).ToNot(BeNil())
					Expect(err.Error()).To(Equal("leader election lost"))
					close(mgrDone)
				}()
				cm := m.(*controllerManager)
				<-cm.elected

				cm.leaderElectionCancel()
				<-mgrDone

				Expect(cm.gracefulShutdownTimeout.Nanoseconds()).To(Equal(int64(0)))
			})
			It("should default ID to controller-runtime if ID is not set", func() {
				var rl resourcelock.Interface
				m1, err := New(cfg, Options{
					LeaderElection:          true,
					LeaderElectionNamespace: "default",
					LeaderElectionID:        "test-leader-election-id",
					newResourceLock: func(config *rest.Config, recorderProvider recorder.Provider, options leaderelection.Options) (resourcelock.Interface, error) {
						var err error
						rl, err = leaderelection.NewResourceLock(config, recorderProvider, options)
						return rl, err
					},
					HealthProbeBindAddress: "0",
					MetricsBindAddress:     "0",
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(m1).ToNot(BeNil())
				Expect(rl.Describe()).To(Equal("default/test-leader-election-id"))

				m1cm, ok := m1.(*controllerManager)
				Expect(ok).To(BeTrue())
				m1cm.onStoppedLeading = func() {}

				m2, err := New(cfg, Options{
					LeaderElection:          true,
					LeaderElectionNamespace: "default",
					LeaderElectionID:        "test-leader-election-id",
					newResourceLock: func(config *rest.Config, recorderProvider recorder.Provider, options leaderelection.Options) (resourcelock.Interface, error) {
						var err error
						rl, err = leaderelection.NewResourceLock(config, recorderProvider, options)
						return rl, err
					},
					HealthProbeBindAddress: "0",
					MetricsBindAddress:     "0",
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(m2).ToNot(BeNil())
				Expect(rl.Describe()).To(Equal("default/test-leader-election-id"))

				m2cm, ok := m2.(*controllerManager)
				Expect(ok).To(BeTrue())
				m2cm.onStoppedLeading = func() {}

				c1 := make(chan struct{})
				Expect(m1.Add(RunnableFunc(func(s <-chan struct{}) error {
					defer GinkgoRecover()
					close(c1)
					return nil
				}))).To(Succeed())

				go func() {
					defer GinkgoRecover()
					Expect(m1.Elected()).ShouldNot(BeClosed())
					Expect(m1.Start(stop)).NotTo(HaveOccurred())
					Expect(m1.Elected()).Should(BeClosed())
				}()
				<-c1

				c2 := make(chan struct{})
				Expect(m2.Add(RunnableFunc(func(s <-chan struct{}) error {
					defer GinkgoRecover()
					close(c2)
					return nil
				}))).To(Succeed())

				m2Stop := make(chan struct{})
				m2done := make(chan struct{})
				go func() {
					defer GinkgoRecover()
					Expect(m2.Start(m2Stop)).NotTo(HaveOccurred())
					close(m2done)
				}()
				Consistently(m2.Elected()).ShouldNot(Receive())

				Consistently(c2).ShouldNot(Receive())
				close(m2Stop)
				<-m2done
			})

			It("should return an error if namespace not set and not running in cluster", func() {
				m, err := New(cfg, Options{LeaderElection: true, LeaderElectionID: "controller-runtime"})
				Expect(m).To(BeNil())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unable to find leader election namespace: not running in-cluster, please specify LeaderElectionNamespace"))
			})
		})

		It("should create a listener for the metrics if a valid address is provided", func() {
			var listener net.Listener
			m, err := New(cfg, Options{
				MetricsBindAddress: ":0",
				newMetricsListener: func(addr string) (net.Listener, error) {
					var err error
					listener, err = metrics.NewListener(addr)
					return listener, err
				},
			})
			Expect(m).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())
			Expect(listener).ToNot(BeNil())
			Expect(listener.Close()).ToNot(HaveOccurred())
		})

		It("should return an error if the metrics bind address is already in use", func() {
			ln, err := metrics.NewListener(":0")
			Expect(err).ShouldNot(HaveOccurred())

			var listener net.Listener
			m, err := New(cfg, Options{
				MetricsBindAddress: ln.Addr().String(),
				newMetricsListener: func(addr string) (net.Listener, error) {
					var err error
					listener, err = metrics.NewListener(addr)
					return listener, err
				},
			})
			Expect(m).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(listener).To(BeNil())

			Expect(ln.Close()).ToNot(HaveOccurred())
		})

		It("should create a listener for the health probes if a valid address is provided", func() {
			var listener net.Listener
			m, err := New(cfg, Options{
				HealthProbeBindAddress: ":0",
				newHealthProbeListener: func(addr string) (net.Listener, error) {
					var err error
					listener, err = defaultHealthProbeListener(addr)
					return listener, err
				},
			})
			Expect(m).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())
			Expect(listener).ToNot(BeNil())
			Expect(listener.Close()).ToNot(HaveOccurred())
		})

		It("should return an error if the health probes bind address is already in use", func() {
			ln, err := defaultHealthProbeListener(":0")
			Expect(err).ShouldNot(HaveOccurred())

			var listener net.Listener
			m, err := New(cfg, Options{
				HealthProbeBindAddress: ln.Addr().String(),
				newHealthProbeListener: func(addr string) (net.Listener, error) {
					var err error
					listener, err = defaultHealthProbeListener(addr)
					return listener, err
				},
			})
			Expect(m).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(listener).To(BeNil())

			Expect(ln.Close()).ToNot(HaveOccurred())
		})
	})

	Describe("Start", func() {
		var startSuite = func(options Options, callbacks ...func(Manager)) {
			It("should Start each Component", func(done Done) {
				m, err := New(cfg, options)
				Expect(err).NotTo(HaveOccurred())
				for _, cb := range callbacks {
					cb(m)
				}
				var wgRunnableStarted sync.WaitGroup
				wgRunnableStarted.Add(2)
				Expect(m.Add(RunnableFunc(func(s <-chan struct{}) error {
					defer GinkgoRecover()
					wgRunnableStarted.Done()
					return nil
				}))).To(Succeed())

				Expect(m.Add(RunnableFunc(func(s <-chan struct{}) error {
					defer GinkgoRecover()
					wgRunnableStarted.Done()
					return nil
				}))).To(Succeed())

				go func() {
					defer GinkgoRecover()
					Expect(m.Elected()).ShouldNot(BeClosed())
					Expect(m.Start(stop)).NotTo(HaveOccurred())
					Expect(m.Elected()).Should(BeClosed())
				}()

				wgRunnableStarted.Wait()
				close(done)
			})

			It("should stop when stop is called", func(done Done) {
				m, err := New(cfg, options)
				Expect(err).NotTo(HaveOccurred())
				for _, cb := range callbacks {
					cb(m)
				}
				s := make(chan struct{})
				close(s)
				Expect(m.Start(s)).NotTo(HaveOccurred())

				close(done)
			})

			It("should return an error if it can't start the cache", func(done Done) {
				m, err := New(cfg, options)
				Expect(err).NotTo(HaveOccurred())
				for _, cb := range callbacks {
					cb(m)
				}
				mgr, ok := m.(*controllerManager)
				Expect(ok).To(BeTrue())
				mgr.startCache = func(stop <-chan struct{}) error {
					return fmt.Errorf("expected error")
				}
				Expect(m.Start(stop)).To(MatchError(ContainSubstring("expected error")))

				close(done)
			})

			It("should return an error if any Components fail to Start", func(done Done) {
				m, err := New(cfg, options)
				Expect(err).NotTo(HaveOccurred())
				for _, cb := range callbacks {
					cb(m)
				}

				Expect(m.Add(RunnableFunc(func(s <-chan struct{}) error {
					defer GinkgoRecover()
					<-s
					return nil
				}))).To(Succeed())

				Expect(m.Add(RunnableFunc(func(s <-chan struct{}) error {
					defer GinkgoRecover()
					return fmt.Errorf("expected error")
				}))).To(Succeed())

				Expect(m.Add(RunnableFunc(func(s <-chan struct{}) error {
					defer GinkgoRecover()
					return nil
				}))).To(Succeed())

				defer GinkgoRecover()
				err = m.Start(stop)
				Expect(err).ToNot(BeNil())
				Expect(err.Error()).To(Equal("expected error"))

				close(done)
			})

			It("should wait for runnables to stop", func(done Done) {
				m, err := New(cfg, options)
				Expect(err).NotTo(HaveOccurred())
				for _, cb := range callbacks {
					cb(m)
				}

				var lock sync.Mutex
				runnableDoneCount := 0
				runnableDoneFunc := func() {
					lock.Lock()
					defer lock.Unlock()
					runnableDoneCount++
				}
				var wgRunnableRunning sync.WaitGroup
				wgRunnableRunning.Add(2)
				Expect(m.Add(RunnableFunc(func(s <-chan struct{}) error {
					wgRunnableRunning.Done()
					defer GinkgoRecover()
					defer runnableDoneFunc()
					<-s
					return nil
				}))).To(Succeed())

				Expect(m.Add(RunnableFunc(func(s <-chan struct{}) error {
					wgRunnableRunning.Done()
					defer GinkgoRecover()
					defer runnableDoneFunc()
					<-s
					time.Sleep(300 * time.Millisecond) //slow closure simulation
					return nil
				}))).To(Succeed())

				defer GinkgoRecover()
				s := make(chan struct{})

				var wgManagerRunning sync.WaitGroup
				wgManagerRunning.Add(1)
				go func() {
					defer wgManagerRunning.Done()
					Expect(m.Start(s)).NotTo(HaveOccurred())
					Expect(runnableDoneCount).To(Equal(2))
				}()
				wgRunnableRunning.Wait()
				close(s)

				wgManagerRunning.Wait()
				close(done)
			})

			It("should return an error if any Components fail to Start and wait for runnables to stop", func(done Done) {
				m, err := New(cfg, options)
				Expect(err).NotTo(HaveOccurred())
				for _, cb := range callbacks {
					cb(m)
				}
				defer GinkgoRecover()
				var lock sync.Mutex
				runnableDoneCount := 0
				runnableDoneFunc := func() {
					lock.Lock()
					defer lock.Unlock()
					runnableDoneCount++
				}

				Expect(m.Add(RunnableFunc(func(s <-chan struct{}) error {
					defer GinkgoRecover()
					defer runnableDoneFunc()
					return fmt.Errorf("expected error")
				}))).To(Succeed())

				Expect(m.Add(RunnableFunc(func(s <-chan struct{}) error {
					defer GinkgoRecover()
					defer runnableDoneFunc()
					<-s
					return nil
				}))).To(Succeed())

				Expect(m.Start(stop)).To(HaveOccurred())
				Expect(runnableDoneCount).To(Equal(2))

				close(done)
			})

			It("should refuse to add runnable if stop procedure is already engaged", func(done Done) {
				m, err := New(cfg, options)
				Expect(err).NotTo(HaveOccurred())
				for _, cb := range callbacks {
					cb(m)
				}
				defer GinkgoRecover()

				var wgRunnableRunning sync.WaitGroup
				wgRunnableRunning.Add(1)
				Expect(m.Add(RunnableFunc(func(s <-chan struct{}) error {
					wgRunnableRunning.Done()
					defer GinkgoRecover()
					<-s
					return nil
				}))).To(Succeed())

				s := make(chan struct{})
				go func() {
					Expect(m.Start(s)).NotTo(HaveOccurred())
				}()
				wgRunnableRunning.Wait()
				close(s)
				time.Sleep(100 * time.Millisecond) // give some time for the stop chan closure to be caught by the manager
				Expect(m.Add(RunnableFunc(func(s <-chan struct{}) error {
					defer GinkgoRecover()
					return nil
				}))).NotTo(Succeed())

				close(done)
			})

			It("should return both runnables and stop errors when both error", func(done Done) {
				m, err := New(cfg, options)
				Expect(err).NotTo(HaveOccurred())
				for _, cb := range callbacks {
					cb(m)
				}
				m.(*controllerManager).gracefulShutdownTimeout = 1 * time.Nanosecond
				Expect(m.Add(RunnableFunc(func(_ <-chan struct{}) error {
					return runnableError{}
				})))
				testDone := make(chan struct{})
				defer close(testDone)
				Expect(m.Add(RunnableFunc(func(s <-chan struct{}) error {
					<-s
					timer := time.NewTimer(30 * time.Second)
					defer timer.Stop()
					select {
					case <-testDone:
						return nil
					case <-timer.C:
						return nil
					}
				})))
				err = m.Start(make(chan struct{}))
				Expect(err).ToNot(BeNil())
				eMsg := "[not feeling like that, failed waiting for all runnables to end within grace period of 1ns: context deadline exceeded]"
				Expect(err.Error()).To(Equal(eMsg))
				Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue())
				Expect(errors.Is(err, runnableError{})).To(BeTrue())

				close(done)
			})

			It("should return only stop errors if runnables dont error", func(done Done) {
				m, err := New(cfg, options)
				Expect(err).NotTo(HaveOccurred())
				for _, cb := range callbacks {
					cb(m)
				}
				m.(*controllerManager).gracefulShutdownTimeout = 1 * time.Nanosecond
				Expect(m.Add(RunnableFunc(func(s <-chan struct{}) error {
					<-s
					return nil
				})))
				testDone := make(chan struct{})
				defer close(testDone)
				Expect(m.Add(RunnableFunc(func(s <-chan struct{}) error {
					<-s
					timer := time.NewTimer(30 * time.Second)
					defer timer.Stop()
					select {
					case <-testDone:
						return nil
					case <-timer.C:
						return nil
					}
				}))).NotTo(HaveOccurred())
				stop := make(chan struct{})
				managerStopDone := make(chan struct{})
				go func() { err = m.Start(stop); close(managerStopDone) }()
				// Use the 'elected' channel to find out if startup was done, otherwise we stop
				// before we started the Runnable and see flakes, mostly in low-CPU envs like CI
				<-m.(*controllerManager).elected
				close(stop)
				<-managerStopDone
				Expect(err).ToNot(BeNil())
				Expect(err.Error()).To(Equal("failed waiting for all runnables to end within grace period of 1ns: context deadline exceeded"))
				Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue())
				Expect(errors.Is(err, runnableError{})).ToNot(BeTrue())

				close(done)
			})

			It("should return only runnables error if stop doesn't error", func(done Done) {
				m, err := New(cfg, options)
				Expect(err).NotTo(HaveOccurred())
				for _, cb := range callbacks {
					cb(m)
				}
				Expect(m.Add(RunnableFunc(func(_ <-chan struct{}) error {
					return runnableError{}
				})))
				err = m.Start(make(chan struct{}))
				Expect(err).ToNot(BeNil())
				Expect(err.Error()).To(Equal("not feeling like that"))
				Expect(errors.Is(err, context.DeadlineExceeded)).ToNot(BeTrue())
				Expect(errors.Is(err, runnableError{})).To(BeTrue())

				close(done)
			})

			It("should not wait for runnables if gracefulShutdownTimeout is 0", func(done Done) {
				m, err := New(cfg, options)
				Expect(err).NotTo(HaveOccurred())
				for _, cb := range callbacks {
					cb(m)
				}
				m.(*controllerManager).gracefulShutdownTimeout = time.Duration(0)

				runnableStopped := make(chan struct{})
				Expect(m.Add(RunnableFunc(func(s <-chan struct{}) error {
					<-s
					time.Sleep(100 * time.Millisecond)
					close(runnableStopped)
					return nil
				}))).ToNot(HaveOccurred())

				managerStop := make(chan struct{})
				managerStopDone := make(chan struct{})
				go func() {
					Expect(m.Start(managerStop)).NotTo(HaveOccurred())
					close(managerStopDone)
				}()
				<-m.(*controllerManager).elected
				close(managerStop)

				<-managerStopDone
				<-runnableStopped
				close(done)
			})

		}

		Context("with defaults", func() {
			startSuite(Options{})
		})

		Context("with leaderelection enabled", func() {
			startSuite(
				Options{
					LeaderElection:          true,
					LeaderElectionID:        "controller-runtime",
					LeaderElectionNamespace: "default",
					newResourceLock:         fakeleaderelection.NewResourceLock,
				},
				func(m Manager) {
					cm, ok := m.(*controllerManager)
					Expect(ok).To(BeTrue())
					cm.onStoppedLeading = func() {}
				},
			)
		})

		Context("should start serving metrics", func() {
			var listener net.Listener
			var opts Options

			BeforeEach(func() {
				listener = nil
				opts = Options{
					newMetricsListener: func(addr string) (net.Listener, error) {
						var err error
						listener, err = metrics.NewListener(addr)
						return listener, err
					},
				}
			})

			AfterEach(func() {
				if listener != nil {
					listener.Close()
				}
			})

			It("should stop serving metrics when stop is called", func(done Done) {
				opts.MetricsBindAddress = ":0"
				m, err := New(cfg, opts)
				Expect(err).NotTo(HaveOccurred())

				s := make(chan struct{})
				go func() {
					defer GinkgoRecover()
					Expect(m.Start(s)).NotTo(HaveOccurred())
					close(done)
				}()

				// Check the metrics started
				endpoint := fmt.Sprintf("http://%s", listener.Addr().String())
				_, err = http.Get(endpoint)
				Expect(err).NotTo(HaveOccurred())

				// Shutdown the server
				close(s)

				// Expect the metrics server to shutdown
				Eventually(func() error {
					_, err = http.Get(endpoint)
					return err
				}).ShouldNot(Succeed())
			})

			It("should serve metrics endpoint", func(done Done) {
				opts.MetricsBindAddress = ":0"
				m, err := New(cfg, opts)
				Expect(err).NotTo(HaveOccurred())

				s := make(chan struct{})
				defer close(s)
				go func() {
					defer GinkgoRecover()
					Expect(m.Start(s)).NotTo(HaveOccurred())
					close(done)
				}()

				metricsEndpoint := fmt.Sprintf("http://%s/metrics", listener.Addr().String())
				resp, err := http.Get(metricsEndpoint)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))
			})

			It("should not serve anything other than metrics endpoint by default", func(done Done) {
				opts.MetricsBindAddress = ":0"
				m, err := New(cfg, opts)
				Expect(err).NotTo(HaveOccurred())

				s := make(chan struct{})
				defer close(s)
				go func() {
					defer GinkgoRecover()
					Expect(m.Start(s)).NotTo(HaveOccurred())
					close(done)
				}()

				endpoint := fmt.Sprintf("http://%s/should-not-exist", listener.Addr().String())
				resp, err := http.Get(endpoint)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})

			It("should serve metrics in its registry", func(done Done) {
				one := prometheus.NewCounter(prometheus.CounterOpts{
					Name: "test_one",
					Help: "test metric for testing",
				})
				one.Inc()
				err := metrics.Registry.Register(one)
				Expect(err).NotTo(HaveOccurred())

				opts.MetricsBindAddress = ":0"
				m, err := New(cfg, opts)
				Expect(err).NotTo(HaveOccurred())

				s := make(chan struct{})
				defer close(s)
				go func() {
					defer GinkgoRecover()
					Expect(m.Start(s)).NotTo(HaveOccurred())
					close(done)
				}()

				metricsEndpoint := fmt.Sprintf("http://%s/metrics", listener.Addr().String())
				resp, err := http.Get(metricsEndpoint)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				data, err := ioutil.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(data)).To(ContainSubstring("%s\n%s\n%s\n",
					`# HELP test_one test metric for testing`,
					`# TYPE test_one counter`,
					`test_one 1`,
				))

				// Unregister will return false if the metric was never registered
				ok := metrics.Registry.Unregister(one)
				Expect(ok).To(BeTrue())
			})

			It("should serve extra endpoints", func(done Done) {
				opts.MetricsBindAddress = ":0"
				m, err := New(cfg, opts)
				Expect(err).NotTo(HaveOccurred())

				err = m.AddMetricsExtraHandler("/debug", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					_, _ = w.Write([]byte("Some debug info"))
				}))
				Expect(err).NotTo(HaveOccurred())

				// Should error when we add another extra endpoint on the already registered path.
				err = m.AddMetricsExtraHandler("/debug", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					_, _ = w.Write([]byte("Another debug info"))
				}))
				Expect(err).To(HaveOccurred())

				s := make(chan struct{})
				defer close(s)
				go func() {
					defer GinkgoRecover()
					Expect(m.Start(s)).NotTo(HaveOccurred())
					close(done)
				}()

				endpoint := fmt.Sprintf("http://%s/debug", listener.Addr().String())
				resp, err := http.Get(endpoint)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				body, err := ioutil.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(body)).To(Equal("Some debug info"))
			})
		})
	})

	Context("should start serving health probes", func() {
		var listener net.Listener
		var opts Options

		BeforeEach(func() {
			listener = nil
			opts = Options{
				newHealthProbeListener: func(addr string) (net.Listener, error) {
					var err error
					listener, err = defaultHealthProbeListener(addr)
					return listener, err
				},
			}
		})

		AfterEach(func() {
			if listener != nil {
				listener.Close()
			}
		})

		It("should stop serving health probes when stop is called", func(done Done) {
			opts.HealthProbeBindAddress = ":0"
			m, err := New(cfg, opts)
			Expect(err).NotTo(HaveOccurred())

			s := make(chan struct{})
			go func() {
				defer GinkgoRecover()
				Expect(m.Start(s)).NotTo(HaveOccurred())
				close(done)
			}()

			// Check the health probes started
			endpoint := fmt.Sprintf("http://%s", listener.Addr().String())
			_, err = http.Get(endpoint)
			Expect(err).NotTo(HaveOccurred())

			// Shutdown the server
			close(s)

			// Expect the health probes server to shutdown
			Eventually(func() error {
				_, err = http.Get(endpoint)
				return err
			}).ShouldNot(Succeed())
		})

		It("should serve readiness endpoint", func(done Done) {
			opts.HealthProbeBindAddress = ":0"
			m, err := New(cfg, opts)
			Expect(err).NotTo(HaveOccurred())

			res := fmt.Errorf("not ready yet")
			namedCheck := "check"
			err = m.AddReadyzCheck(namedCheck, func(_ *http.Request) error { return res })
			Expect(err).NotTo(HaveOccurred())

			s := make(chan struct{})
			defer close(s)
			go func() {
				defer GinkgoRecover()
				Expect(m.Start(s)).NotTo(HaveOccurred())
				close(done)
			}()

			readinessEndpoint := fmt.Sprint("http://", listener.Addr().String(), defaultReadinessEndpoint)

			// Controller is not ready
			resp, err := http.Get(readinessEndpoint)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))

			// Controller is ready
			res = nil
			resp, err = http.Get(readinessEndpoint)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// Check readiness path without trailing slash
			readinessEndpoint = fmt.Sprint("http://", listener.Addr().String(), strings.TrimSuffix(defaultReadinessEndpoint, "/"))
			res = nil
			resp, err = http.Get(readinessEndpoint)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// Check readiness path for individual check
			readinessEndpoint = fmt.Sprint("http://", listener.Addr().String(), path.Join(defaultReadinessEndpoint, namedCheck))
			res = nil
			resp, err = http.Get(readinessEndpoint)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("should serve liveness endpoint", func(done Done) {
			opts.HealthProbeBindAddress = ":0"
			m, err := New(cfg, opts)
			Expect(err).NotTo(HaveOccurred())

			res := fmt.Errorf("not alive")
			namedCheck := "check"
			err = m.AddHealthzCheck(namedCheck, func(_ *http.Request) error { return res })
			Expect(err).NotTo(HaveOccurred())

			s := make(chan struct{})
			defer close(s)
			go func() {
				defer GinkgoRecover()
				Expect(m.Start(s)).NotTo(HaveOccurred())
				close(done)
			}()

			livenessEndpoint := fmt.Sprint("http://", listener.Addr().String(), defaultLivenessEndpoint)

			// Controller is not ready
			resp, err := http.Get(livenessEndpoint)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))

			// Controller is ready
			res = nil
			resp, err = http.Get(livenessEndpoint)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// Check liveness path without trailing slash
			livenessEndpoint = fmt.Sprint("http://", listener.Addr().String(), strings.TrimSuffix(defaultLivenessEndpoint, "/"))
			res = nil
			resp, err = http.Get(livenessEndpoint)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// Check readiness path for individual check
			livenessEndpoint = fmt.Sprint("http://", listener.Addr().String(), path.Join(defaultLivenessEndpoint, namedCheck))
			res = nil
			resp, err = http.Get(livenessEndpoint)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})

	Describe("Add", func() {
		It("should immediately start the Component if the Manager has already Started another Component",
			func(done Done) {
				m, err := New(cfg, Options{})
				Expect(err).NotTo(HaveOccurred())
				mgr, ok := m.(*controllerManager)
				Expect(ok).To(BeTrue())

				// Add one component before starting
				c1 := make(chan struct{})
				Expect(m.Add(RunnableFunc(func(s <-chan struct{}) error {
					defer GinkgoRecover()
					close(c1)
					return nil
				}))).To(Succeed())

				go func() {
					defer GinkgoRecover()
					Expect(m.Start(stop)).NotTo(HaveOccurred())
				}()

				// Wait for the Manager to start
				Eventually(func() bool {
					mgr.mu.Lock()
					defer mgr.mu.Unlock()
					return mgr.started
				}).Should(BeTrue())

				// Add another component after starting
				c2 := make(chan struct{})
				Expect(m.Add(RunnableFunc(func(s <-chan struct{}) error {
					defer GinkgoRecover()
					close(c2)
					return nil
				}))).To(Succeed())
				<-c1
				<-c2

				close(done)
			})

		It("should immediately start the Component if the Manager has already Started", func(done Done) {
			m, err := New(cfg, Options{})
			Expect(err).NotTo(HaveOccurred())
			mgr, ok := m.(*controllerManager)
			Expect(ok).To(BeTrue())

			go func() {
				defer GinkgoRecover()
				Expect(m.Start(stop)).NotTo(HaveOccurred())
			}()

			// Wait for the Manager to start
			Eventually(func() bool {
				mgr.mu.Lock()
				defer mgr.mu.Unlock()
				return mgr.started
			}).Should(BeTrue())

			c1 := make(chan struct{})
			Expect(m.Add(RunnableFunc(func(s <-chan struct{}) error {
				defer GinkgoRecover()
				close(c1)
				return nil
			}))).To(Succeed())
			<-c1

			close(done)
		})

		It("should fail if SetFields fails", func() {
			m, err := New(cfg, Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(m.Add(&failRec{})).To(HaveOccurred())
		})
	})
	Describe("SetFields", func() {
		It("should inject field values", func(done Done) {
			m, err := New(cfg, Options{})
			Expect(err).NotTo(HaveOccurred())
			mgr, ok := m.(*controllerManager)
			Expect(ok).To(BeTrue())

			mgr.cache = &informertest.FakeInformers{}

			By("Injecting the dependencies")
			err = m.SetFields(&injectable{
				scheme: func(scheme *runtime.Scheme) error {
					defer GinkgoRecover()
					Expect(scheme).To(Equal(m.GetScheme()))
					return nil
				},
				config: func(config *rest.Config) error {
					defer GinkgoRecover()
					Expect(config).To(Equal(m.GetConfig()))
					return nil
				},
				client: func(client client.Client) error {
					defer GinkgoRecover()
					Expect(client).To(Equal(m.GetClient()))
					return nil
				},
				cache: func(c cache.Cache) error {
					defer GinkgoRecover()
					Expect(c).To(Equal(m.GetCache()))
					return nil
				},
				stop: func(stop <-chan struct{}) error {
					defer GinkgoRecover()
					Expect(stop).NotTo(BeNil())
					return nil
				},
				f: func(f inject.Func) error {
					defer GinkgoRecover()
					Expect(f).NotTo(BeNil())
					return nil
				},
				log: func(logger logr.Logger) error {
					defer GinkgoRecover()
					Expect(logger).To(Equal(log))
					return nil
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Returning an error if dependency injection fails")

			expected := fmt.Errorf("expected error")
			err = m.SetFields(&injectable{
				client: func(client client.Client) error {
					return expected
				},
			})
			Expect(err).To(Equal(expected))

			err = m.SetFields(&injectable{
				scheme: func(scheme *runtime.Scheme) error {
					return expected
				},
			})
			Expect(err).To(Equal(expected))

			err = m.SetFields(&injectable{
				config: func(config *rest.Config) error {
					return expected
				},
			})
			Expect(err).To(Equal(expected))

			err = m.SetFields(&injectable{
				cache: func(c cache.Cache) error {
					return expected
				},
			})
			Expect(err).To(Equal(expected))

			err = m.SetFields(&injectable{
				f: func(c inject.Func) error {
					return expected
				},
			})
			Expect(err).To(Equal(expected))
			err = m.SetFields(&injectable{
				stop: func(<-chan struct{}) error {
					return expected
				},
			})
			Expect(err).To(Equal(expected))
			close(done)
		})
	})

	// This test has been marked as pending because it has been causing lots of flakes in CI.
	// It should be rewritten at some point later in the future.
	XIt("should not leak goroutines when stop", func(done Done) {
		// TODO(directxman12): After closing the proper leaks on watch this must be reduced to 0
		// The leaks currently come from the event-related code (as in corev1.Event).
		threshold := 3

		m, err := New(cfg, Options{})
		Expect(err).NotTo(HaveOccurred())
		startGoruntime := rt.NumGoroutine()
		_ = pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)

		s := make(chan struct{})
		close(s)
		Expect(m.Start(s)).NotTo(HaveOccurred())

		_ = pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
		Expect(rt.NumGoroutine() - startGoruntime).To(BeNumerically("<=", threshold))
		close(done)
	})

	It("should provide a function to get the Config", func() {
		m, err := New(cfg, Options{})
		Expect(err).NotTo(HaveOccurred())
		mgr, ok := m.(*controllerManager)
		Expect(ok).To(BeTrue())
		Expect(m.GetConfig()).To(Equal(mgr.config))
	})

	It("should provide a function to get the Client", func() {
		m, err := New(cfg, Options{})
		Expect(err).NotTo(HaveOccurred())
		mgr, ok := m.(*controllerManager)
		Expect(ok).To(BeTrue())
		Expect(m.GetClient()).To(Equal(mgr.client))
	})

	It("should provide a function to get the Scheme", func() {
		m, err := New(cfg, Options{})
		Expect(err).NotTo(HaveOccurred())
		mgr, ok := m.(*controllerManager)
		Expect(ok).To(BeTrue())
		Expect(m.GetScheme()).To(Equal(mgr.scheme))
	})

	It("should provide a function to get the FieldIndexer", func() {
		m, err := New(cfg, Options{})
		Expect(err).NotTo(HaveOccurred())
		mgr, ok := m.(*controllerManager)
		Expect(ok).To(BeTrue())
		Expect(m.GetFieldIndexer()).To(Equal(mgr.fieldIndexes))
	})

	It("should provide a function to get the EventRecorder", func() {
		m, err := New(cfg, Options{})
		Expect(err).NotTo(HaveOccurred())
		Expect(m.GetEventRecorderFor("test")).NotTo(BeNil())
	})
	It("should provide a function to get the APIReader", func() {
		m, err := New(cfg, Options{})
		Expect(err).NotTo(HaveOccurred())
		Expect(m.GetAPIReader()).NotTo(BeNil())
	})
})

var _ reconcile.Reconciler = &failRec{}
var _ inject.Client = &failRec{}

type failRec struct{}

func (*failRec) Reconcile(reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (*failRec) Start(<-chan struct{}) error {
	return nil
}

func (*failRec) InjectClient(client.Client) error {
	return fmt.Errorf("expected error")
}

var _ inject.Injector = &injectable{}
var _ inject.Cache = &injectable{}
var _ inject.Client = &injectable{}
var _ inject.Scheme = &injectable{}
var _ inject.Config = &injectable{}
var _ inject.Stoppable = &injectable{}
var _ inject.Logger = &injectable{}

type injectable struct {
	scheme func(scheme *runtime.Scheme) error
	client func(client.Client) error
	config func(config *rest.Config) error
	cache  func(cache.Cache) error
	f      func(inject.Func) error
	stop   func(<-chan struct{}) error
	log    func(logger logr.Logger) error
}

func (i *injectable) InjectCache(c cache.Cache) error {
	if i.cache == nil {
		return nil
	}
	return i.cache(c)
}

func (i *injectable) InjectConfig(config *rest.Config) error {
	if i.config == nil {
		return nil
	}
	return i.config(config)
}

func (i *injectable) InjectClient(c client.Client) error {
	if i.client == nil {
		return nil
	}
	return i.client(c)
}

func (i *injectable) InjectScheme(scheme *runtime.Scheme) error {
	if i.scheme == nil {
		return nil
	}
	return i.scheme(scheme)
}

func (i *injectable) InjectFunc(f inject.Func) error {
	if i.f == nil {
		return nil
	}
	return i.f(f)
}

func (i *injectable) InjectStopChannel(stop <-chan struct{}) error {
	if i.stop == nil {
		return nil
	}
	return i.stop(stop)
}

func (i *injectable) InjectLogger(log logr.Logger) error {
	if i.log == nil {
		return nil
	}
	return i.log(log)
}

func (i *injectable) Start(<-chan struct{}) error {
	return nil
}

type runnableError struct {
}

func (runnableError) Error() string {
	return "not feeling like that"
}
