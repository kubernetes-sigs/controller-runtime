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

	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/event"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/eventhandler"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/inject"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/source"
	"github.com/kubernetes-sigs/kubebuilder/pkg/informer/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/util/workqueue"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Source", func() {
	Describe("KindSource", func() {
		var c chan struct{}
		var p *corev1.Pod
		var d *appsv1.Deployment
		var ic *test.FakeIndexCache

		BeforeEach(func() {
			ic = &test.FakeIndexCache{}
			c = make(chan struct{})
			p = &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "test", Image: "test"},
					},
				},
			}
			d = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "deployment-name"},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"foo": "bar"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "bar"}},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx",
								},
							},
						},
					},
				},
			}
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
				inject.InjectIndexInformerCache(ic, instance)
				err := instance.Start(eventhandler.EventHandlerFuncs{
					CreateFunc: func(q2 workqueue.RateLimitingInterface, evt event.CreateEvent) {
						defer GinkgoRecover()
						Expect(q2).To(Equal(q))
						m := evt.Meta.(*metav1.ObjectMeta)
						Expect(m).To(Equal(&p.ObjectMeta))
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

				ic := &test.FakeIndexCache{}
				q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
				instance := &source.KindSource{
					Type: &corev1.Pod{},
				}
				instance.InjectIndexInformerCache(ic)
				err := instance.Start(eventhandler.EventHandlerFuncs{
					CreateFunc: func(q2 workqueue.RateLimitingInterface, evt event.CreateEvent) {
						defer GinkgoRecover()
						Fail("Unexpected CreateEvent")
					},
					UpdateFunc: func(q2 workqueue.RateLimitingInterface, evt event.UpdateEvent) {
						defer GinkgoRecover()
						Expect(q2).To(Equal(q))
						m := evt.MetaOld.(*metav1.ObjectMeta)
						Expect(m).To(Equal(&p.ObjectMeta))
						Expect(evt.ObjectOld).To(Equal(p))

						m = evt.MetaNew.(*metav1.ObjectMeta)
						Expect(m).To(Equal(&p2.ObjectMeta))
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
				inject.InjectIndexInformerCache(ic, instance)
				err := instance.Start(eventhandler.EventHandlerFuncs{
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
						m := evt.Meta.(*metav1.ObjectMeta)
						Expect(m).To(Equal(&p.ObjectMeta))
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

			It("should provide a Pod DeletedEvent for a tombstone", func(done Done) {
				// TODO: Write this
				close(done)
			})
		})
		Context("for a Kind not in the cache", func() {
			It("should return an error when Start is called", func(done Done) {
				ic.Error = fmt.Errorf("test error")
				q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")

				instance := &source.KindSource{
					Type: &corev1.Pod{},
				}
				instance.InjectIndexInformerCache(ic)
				err := instance.Start(eventhandler.EventHandlerFuncs{}, q)
				Expect(err).To(HaveOccurred())

				close(done)
			})
		})
	})
})
