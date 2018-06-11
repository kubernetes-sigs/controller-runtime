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

package source_test

import (
	"fmt"

	"github.com/kubernetes-sigs/controller-runtime/pkg/cache/informertest"
	"github.com/kubernetes-sigs/controller-runtime/pkg/controller/event"
	"github.com/kubernetes-sigs/controller-runtime/pkg/controller/eventhandler"
	"github.com/kubernetes-sigs/controller-runtime/pkg/controller/source"
	"github.com/kubernetes-sigs/controller-runtime/pkg/runtime/inject"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/util/workqueue"

	"github.com/kubernetes-sigs/controller-runtime/pkg/controller/predicate"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("Source", func() {
	Describe("KindSource", func() {
		var c chan struct{}
		var p *corev1.Pod
		var ic *informertest.FakeInformers

		BeforeEach(func(done Done) {
			ic = &informertest.FakeInformers{}
			c = make(chan struct{})
			p = &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "test", Image: "test"},
					},
				},
			}
			close(done)
		})

		Context("for a Pod resource", func() {
			It("should provide a Pod CreateEvent", func(done Done) {
				c := make(chan struct{})
				p := &corev1.Pod{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "test", Image: "test"},
						},
					},
				}

				q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
				instance := &source.KindSource{
					Type: &corev1.Pod{},
				}
				inject.CacheInto(ic, instance)
				err := instance.Start(eventhandler.Funcs{
					CreateFunc: func(q2 workqueue.RateLimitingInterface, evt event.CreateEvent) {
						defer GinkgoRecover()
						Expect(q2).To(Equal(q))
						Expect(evt.Meta).To(Equal(p))
						Expect(evt.Object).To(Equal(p))
						close(c)
					},
					UpdateFunc: func(workqueue.RateLimitingInterface, event.UpdateEvent) {
						defer GinkgoRecover()
						Fail("Unexpected UpdateEvent")
					},
					DeleteFunc: func(workqueue.RateLimitingInterface, event.DeleteEvent) {
						defer GinkgoRecover()
						Fail("Unexpected DeleteEvent")
					},
					GenericFunc: func(workqueue.RateLimitingInterface, event.GenericEvent) {
						defer GinkgoRecover()
						Fail("Unexpected GenericEvent")
					},
				}, q)
				Expect(err).NotTo(HaveOccurred())

				i, err := ic.FakeInformerFor(&corev1.Pod{})
				Expect(err).NotTo(HaveOccurred())

				i.Add(p)
				<-c
				close(done)
			})

			It("should provide a Pod UpdateEvent", func(done Done) {
				p2 := p.DeepCopy()
				p2.SetLabels(map[string]string{"biz": "baz"})

				ic := &informertest.FakeInformers{}
				q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
				instance := &source.KindSource{
					Type: &corev1.Pod{},
				}
				instance.InjectCache(ic)
				err := instance.Start(eventhandler.Funcs{
					CreateFunc: func(q2 workqueue.RateLimitingInterface, evt event.CreateEvent) {
						defer GinkgoRecover()
						Fail("Unexpected CreateEvent")
					},
					UpdateFunc: func(q2 workqueue.RateLimitingInterface, evt event.UpdateEvent) {
						defer GinkgoRecover()
						Expect(q2).To(Equal(q))
						Expect(evt.MetaOld).To(Equal(p))
						Expect(evt.ObjectOld).To(Equal(p))

						Expect(evt.MetaNew).To(Equal(p2))
						Expect(evt.ObjectNew).To(Equal(p2))

						close(c)
					},
					DeleteFunc: func(workqueue.RateLimitingInterface, event.DeleteEvent) {
						defer GinkgoRecover()
						Fail("Unexpected DeleteEvent")
					},
					GenericFunc: func(workqueue.RateLimitingInterface, event.GenericEvent) {
						defer GinkgoRecover()
						Fail("Unexpected GenericEvent")
					},
				}, q)
				Expect(err).NotTo(HaveOccurred())

				i, err := ic.FakeInformerFor(&corev1.Pod{})
				Expect(err).NotTo(HaveOccurred())

				i.Update(p, p2)
				<-c
				close(done)
			})

			It("should provide a Pod DeletedEvent", func(done Done) {
				c := make(chan struct{})
				p := &corev1.Pod{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "test", Image: "test"},
						},
					},
				}

				q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
				instance := &source.KindSource{
					Type: &corev1.Pod{},
				}
				inject.CacheInto(ic, instance)
				err := instance.Start(eventhandler.Funcs{
					CreateFunc: func(workqueue.RateLimitingInterface, event.CreateEvent) {
						defer GinkgoRecover()
						Fail("Unexpected DeleteEvent")
					},
					UpdateFunc: func(workqueue.RateLimitingInterface, event.UpdateEvent) {
						defer GinkgoRecover()
						Fail("Unexpected UpdateEvent")
					},
					DeleteFunc: func(q2 workqueue.RateLimitingInterface, evt event.DeleteEvent) {
						defer GinkgoRecover()
						Expect(q2).To(Equal(q))
						Expect(evt.Meta).To(Equal(p))
						Expect(evt.Object).To(Equal(p))
						close(c)
					},
					GenericFunc: func(workqueue.RateLimitingInterface, event.GenericEvent) {
						defer GinkgoRecover()
						Fail("Unexpected GenericEvent")
					},
				}, q)
				Expect(err).NotTo(HaveOccurred())

				i, err := ic.FakeInformerFor(&corev1.Pod{})
				Expect(err).NotTo(HaveOccurred())

				i.Delete(p)
				<-c
				close(done)
			})
		})

		It("should return an error from Start if informers were not injected", func(done Done) {
			instance := source.KindSource{Type: &corev1.Pod{}}
			err := instance.Start(nil, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must call CacheInto on KindSource before calling Start"))

			close(done)
		})

		It("should return an error from Start if a type was not provided", func(done Done) {
			instance := source.KindSource{}
			instance.InjectCache(&informertest.FakeInformers{})
			err := instance.Start(nil, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must specify KindSource.Type"))

			close(done)
		})

		Context("for a Kind not in the cache", func() {
			It("should return an error when Start is called", func(done Done) {
				ic.Error = fmt.Errorf("test error")
				q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")

				instance := &source.KindSource{
					Type: &corev1.Pod{},
				}
				instance.InjectCache(ic)
				err := instance.Start(eventhandler.Funcs{}, q)
				Expect(err).To(HaveOccurred())

				close(done)
			})
		})
	})

	Describe("Func", func() {
		It("should be called from Start", func(done Done) {
			run := false
			instance := source.Func(func(
				eventhandler.EventHandler,
				workqueue.RateLimitingInterface, ...predicate.Predicate) error {
				run = true
				return nil
			})
			Expect(instance.Start(nil, nil)).NotTo(HaveOccurred())
			Expect(run).To(BeTrue())

			expected := fmt.Errorf("expected error: Func")
			instance = source.Func(func(
				eventhandler.EventHandler,
				workqueue.RateLimitingInterface, ...predicate.Predicate) error {
				return expected
			})
			Expect(instance.Start(nil, nil)).To(Equal(expected))

			close(done)
		})
	})

	Describe("ChannelSource", func() {
		It("TODO(community): implement this", func(done Done) {
			instance := source.ChannelSource(make(chan event.GenericEvent))
			instance.Start(nil, nil)

			close(done)
		})
	})
})
