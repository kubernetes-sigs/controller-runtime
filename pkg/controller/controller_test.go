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

package controller

import (
	"fmt"

	"time"

	"github.com/kubernetes-sigs/controller-runtime/pkg/cache/informertest"
	"github.com/kubernetes-sigs/controller-runtime/pkg/controller/controllertest"
	"github.com/kubernetes-sigs/controller-runtime/pkg/controller/eventhandler"
	"github.com/kubernetes-sigs/controller-runtime/pkg/controller/predicate"
	"github.com/kubernetes-sigs/controller-runtime/pkg/controller/reconcile"
	"github.com/kubernetes-sigs/controller-runtime/pkg/controller/reconcile/reconciletest"
	"github.com/kubernetes-sigs/controller-runtime/pkg/controller/source"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

var _ = Describe("controller", func() {
	var fakeReconcile *reconciletest.FakeReconcile
	var ctrl *controller
	var queue *controllertest.Queue
	var informers *informertest.FakeInformers
	var stop chan struct{}
	var reconciled chan reconcile.Request
	var request = reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: "foo", Name: "bar"},
	}

	BeforeEach(func() {
		stop = make(chan struct{})
		reconciled = make(chan reconcile.Request)
		fakeReconcile = &reconciletest.FakeReconcile{
			Chan: reconciled,
		}
		queue = &controllertest.Queue{
			Interface: workqueue.New(),
		}
		informers = &informertest.FakeInformers{}
		ctrl = &controller{
			maxConcurrentReconciles: 1,
			reconcile:               fakeReconcile,
			queue:                   queue,
			cache:                   informers,
			inject:                  func(interface{}) error { return nil },
		}
	})

	AfterEach(func() {
		close(stop)
	})

	Describe("Calling Watch on a Controller", func() {
		It("should inject dependencies into the Source", func() {
			src := &source.KindSource{Type: &corev1.Pod{}}
			src.InjectCache(ctrl.cache)
			evthdl := &eventhandler.EnqueueHandler{}
			found := false
			ctrl.inject = func(i interface{}) error {
				defer GinkgoRecover()
				if i == src {
					found = true
				}
				return nil
			}
			Expect(ctrl.Watch(src, evthdl)).NotTo(HaveOccurred())
			Expect(found).To(BeTrue(), "Source not injected")
		})

		It("should return an error if there is an error injecting into the Source", func() {
			src := &source.KindSource{Type: &corev1.Pod{}}
			src.InjectCache(ctrl.cache)
			evthdl := &eventhandler.EnqueueHandler{}
			expected := fmt.Errorf("expect fail source")
			ctrl.inject = func(i interface{}) error {
				defer GinkgoRecover()
				if i == src {
					return expected
				}
				return nil
			}
			Expect(ctrl.Watch(src, evthdl)).To(Equal(expected))
		})

		It("should inject dependencies into the EventHandler", func() {
			src := &source.KindSource{Type: &corev1.Pod{}}
			src.InjectCache(ctrl.cache)
			evthdl := &eventhandler.EnqueueHandler{}
			found := false
			ctrl.inject = func(i interface{}) error {
				defer GinkgoRecover()
				if i == evthdl {
					found = true
				}
				return nil
			}
			Expect(ctrl.Watch(src, evthdl)).NotTo(HaveOccurred())
			Expect(found).To(BeTrue(), "EventHandler not injected")
		})

		It("should return an error if there is an error injecting into the EventHandler", func() {
			src := &source.KindSource{Type: &corev1.Pod{}}
			evthdl := &eventhandler.EnqueueHandler{}
			expected := fmt.Errorf("expect fail eventhandler")
			ctrl.inject = func(i interface{}) error {
				defer GinkgoRecover()
				if i == evthdl {
					return expected
				}
				return nil
			}
			Expect(ctrl.Watch(src, evthdl)).To(Equal(expected))
		})

		It("should inject dependencies into the Reconcile", func() {
		})

		It("should return an error if there is an error injecting into the Reconcile", func() {
		})

		It("should inject dependencies into all of the Predicates", func() {
			src := &source.KindSource{Type: &corev1.Pod{}}
			src.InjectCache(ctrl.cache)
			evthdl := &eventhandler.EnqueueHandler{}
			pr1 := &predicate.Funcs{}
			pr2 := &predicate.Funcs{}
			found1 := false
			found2 := false
			ctrl.inject = func(i interface{}) error {
				defer GinkgoRecover()
				if i == pr1 {
					found1 = true
				}
				if i == pr2 {
					found2 = true
				}
				return nil
			}
			Expect(ctrl.Watch(src, evthdl, pr1, pr2)).NotTo(HaveOccurred())
			Expect(found1).To(BeTrue(), "First Predicated not injected")
			Expect(found2).To(BeTrue(), "Second Predicated not injected")
		})

		It("should return an error if there is an error injecting into any of the Predicates", func() {
			src := &source.KindSource{Type: &corev1.Pod{}}
			src.InjectCache(ctrl.cache)
			evthdl := &eventhandler.EnqueueHandler{}
			pr1 := &predicate.Funcs{}
			pr2 := &predicate.Funcs{}
			expected := fmt.Errorf("expect fail predicate")
			ctrl.inject = func(i interface{}) error {
				defer GinkgoRecover()
				if i == pr1 {
					return expected
				}
				return nil
			}
			Expect(ctrl.Watch(src, evthdl, pr1, pr2)).To(Equal(expected))

			ctrl.inject = func(i interface{}) error {
				defer GinkgoRecover()
				if i == pr2 {
					return expected
				}
				return nil
			}
			Expect(ctrl.Watch(src, evthdl, pr1, pr2)).To(Equal(expected))
		})

		It("should call Start the Source with the EventHandler, Queue, and Predicates", func() {
			pr1 := &predicate.Funcs{}
			pr2 := &predicate.Funcs{}
			evthdl := &eventhandler.EnqueueHandler{}
			src := source.Func(func(e eventhandler.EventHandler, q workqueue.RateLimitingInterface, p ...predicate.Predicate) error {
				defer GinkgoRecover()
				Expect(e).To(Equal(evthdl))
				Expect(q).To(Equal(ctrl.queue))
				Expect(p).To(ConsistOf(pr1, pr2))
				return nil
			})
			Expect(ctrl.Watch(src, evthdl, pr1, pr2)).NotTo(HaveOccurred())

		})

		It("should return an error if there is an error starting the Source", func() {
			err := fmt.Errorf("Expected Error: could not start source")
			src := source.Func(func(eventhandler.EventHandler,
				workqueue.RateLimitingInterface,
				...predicate.Predicate) error {
				defer GinkgoRecover()
				return err
			})
			Expect(ctrl.Watch(src, &eventhandler.EnqueueHandler{})).To(Equal(err))
		})
	})

	Describe("Processing queue items from a Controller", func() {
		It("should call Reconcile if an item is enqueued", func(done Done) {
			go func() {
				defer GinkgoRecover()
				Expect(ctrl.Start(stop)).NotTo(HaveOccurred())
			}()
			ctrl.queue.Add(request)

			By("Invoking Reconcile")
			Expect(<-reconciled).To(Equal(request))

			By("Removing the item from the queue")
			Eventually(ctrl.queue.Len).Should(Equal(0))
			Eventually(func() int { return ctrl.queue.NumRequeues(request) }).Should(Equal(0))

			close(done)
		})

		It("should continue to process additional queue items after the first", func(done Done) {
			ctrl.reconcile = reconcile.Func(func(reconcile.Request) (reconcile.Result, error) {
				defer GinkgoRecover()
				Fail("Reconcile should not have been called")
				return reconcile.Result{}, nil
			})
			go func() {
				defer GinkgoRecover()
				Expect(ctrl.Start(stop)).NotTo(HaveOccurred())
			}()
			ctrl.queue.Add("foo/bar")

			// Don't expect the string to reconciled
			Expect(ctrl.processNextWorkItem()).To(BeTrue())

			Eventually(ctrl.queue.Len).Should(Equal(0))
			Eventually(func() int { return ctrl.queue.NumRequeues(request) }).Should(Equal(0))

			close(done)
		})

		It("should forget an item if it is not a Request and continue processing items", func() {
			// TODO(community): write this test
		})

		It("should requeue a Request if there is an error and continue processing items", func(done Done) {
			fakeReconcile.Err = fmt.Errorf("expected error: reconcile")
			go func() {
				defer GinkgoRecover()
				Expect(ctrl.Start(stop)).NotTo(HaveOccurred())
			}()
			ctrl.queue.Add(request)

			// Reduce the jitterperiod so we don't have to wait a second before the reconcile function is rerun.
			ctrl.jitterPeriod = time.Millisecond

			By("Invoking Reconcile which will give an error")
			Expect(<-reconciled).To(Equal(request))

			By("Invoking Reconcile a second time without error")
			fakeReconcile.Err = nil
			Expect(<-reconciled).To(Equal(request))

			By("Removing the item from the queue")
			Eventually(ctrl.queue.Len).Should(Equal(0))
			Eventually(func() int { return ctrl.queue.NumRequeues(request) }).Should(Equal(0))

			close(done)
		}, 1.0)

		It("should requeue a Request if the Result sets Requeue:true and continue processing items", func() {
			fakeReconcile.Result.Requeue = true
			go func() {
				defer GinkgoRecover()
				Expect(ctrl.Start(stop)).NotTo(HaveOccurred())
			}()
			ctrl.queue.Add(request)

			By("Invoking Reconcile which will ask for requeue")
			Expect(<-reconciled).To(Equal(request))

			By("Invoking Reconcile a second time without asking for requeue")
			fakeReconcile.Result.Requeue = false
			Expect(<-reconciled).To(Equal(request))

			By("Removing the item from the queue")
			Eventually(ctrl.queue.Len).Should(Equal(0))
			Eventually(func() int { return ctrl.queue.NumRequeues(request) }).Should(Equal(0))
		})

		It("should return an error if there is an error waiting for the informers", func(done Done) {
			ctrl.waitForCache = func(<-chan struct{}, ...cache.InformerSynced) bool { return false }
			err := ctrl.Start(stop)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to wait for  caches to sync"))

			close(done)
		})

		It("should forget the Request if Reconcile is successful", func() {
			// TODO(community): write this test
		})

		It("should return if the queue is shutdown", func() {
			// TODO(community): write this test
		})

		It("should wait for informers to be synced before processing items", func() {
			// TODO(community): write this test
		})

		It("should create a new go routine for MaxConcurrentReconciles", func() {
			// TODO(community): write this test
		})
	})
})
