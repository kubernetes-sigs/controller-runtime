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
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type eventChannels[T client.ObjectConstraint] struct {
	create chan event.CreateEvent[T]
	update chan event.UpdateEvent[T]
	delete chan event.DeleteEvent[T]
}

func (ec *eventChannels[T]) close() {
	close(ec.create)
	close(ec.update)
	close(ec.delete)
}

func newEventChannels[T client.ObjectConstraint]() *eventChannels[T] {
	return &eventChannels[T]{
		create: make(chan event.CreateEvent[T]),
		update: make(chan event.UpdateEvent[T]),
		delete: make(chan event.DeleteEvent[T]),
	}
}

var _ = Describe("Source", func() {
	var q workqueue.RateLimitingInterface
	var ns string
	count := 0

	BeforeEach(func() {
		// Create the namespace for the test
		ns = fmt.Sprintf("controller-source-kindsource-%v", count)
		count++
		_, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		q = workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
	})

	AfterEach(func() {
		err := clientset.CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Kind", func() {
		Context("for a Deployment resource", func() {
			obj := &appsv1.Deployment{}
			var instance1, instance2 source.Source[*appsv1.Deployment]
			var c1, c2 *eventChannels[*appsv1.Deployment]

			BeforeEach(func() {
				c1 = newEventChannels[*appsv1.Deployment]()
				c2 = newEventChannels[*appsv1.Deployment]()
			})

			JustBeforeEach(func() {
				instance1 = source.Kind(icache, obj)
				instance2 = source.Kind(icache, obj)
			})

			AfterEach(func() {
				c1.close()
				c2.close()
			})

			It("should provide Deployment Events", func() {
				var created, updated, deleted *appsv1.Deployment
				var err error

				// Get the client and Deployment used to create events
				client := clientset.AppsV1().Deployments(ns)
				deployment := &appsv1.Deployment{
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

				// Create an event handler to verify the events
				newHandler := func(evtChannels *eventChannels[*appsv1.Deployment]) handler.Funcs[*appsv1.Deployment] {
					return handler.Funcs[*appsv1.Deployment]{
						CreateFunc: func(ctx context.Context, evt event.CreateEvent[*appsv1.Deployment], rli workqueue.RateLimitingInterface) {
							defer GinkgoRecover()
							Expect(rli).To(Equal(q))
							evtChannels.create <- evt
						},
						UpdateFunc: func(ctx context.Context, evt event.UpdateEvent[*appsv1.Deployment], rli workqueue.RateLimitingInterface) {
							defer GinkgoRecover()
							Expect(rli).To(Equal(q))
							evtChannels.update <- evt
						},
						DeleteFunc: func(ctx context.Context, evt event.DeleteEvent[*appsv1.Deployment], rli workqueue.RateLimitingInterface) {
							defer GinkgoRecover()
							Expect(rli).To(Equal(q))
							evtChannels.delete <- evt
						},
					}
				}
				handler1 := newHandler(c1)
				handler2 := newHandler(c2)

				// Create 2 instances
				Expect(instance1.Start(ctx, handler1, q)).To(Succeed())
				Expect(instance2.Start(ctx, handler2, q)).To(Succeed())

				By("Creating a Deployment and expecting the CreateEvent.")
				created, err = client.Create(ctx, deployment, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(created).NotTo(BeNil())

				// Check first CreateEvent
				createEvt := <-c1.create
				Expect(createEvt.Object).To(Equal(created))

				// Check second CreateEvent
				createEvt = <-c2.create
				Expect(createEvt.Object).To(Equal(created))

				By("Updating a Deployment and expecting the UpdateEvent.")
				updated = created.DeepCopy()
				updated.Labels = map[string]string{"biz": "buz"}
				updated, err = client.Update(ctx, updated, metav1.UpdateOptions{})
				Expect(err).NotTo(HaveOccurred())

				// Check first UpdateEvent
				updateEvt := <-c1.update
				Expect(updateEvt.ObjectNew).To(Equal(updated))

				Expect(updateEvt.ObjectOld).To(Equal(created))

				// Check second UpdateEvent
				updateEvt = <-c2.update
				Expect(updateEvt.ObjectNew).To(Equal(updated))

				Expect(updateEvt.ObjectOld).To(Equal(created))

				By("Deleting a Deployment and expecting the Delete.")
				deleted = updated.DeepCopy()
				err = client.Delete(ctx, created.Name, metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

				deleted.SetResourceVersion("")
				deleteEvt := <-c1.delete
				deleteEvt.Object.SetResourceVersion("")
				Expect(deleteEvt.Object).To(Equal(deleted))

				deleteEvt = <-c2.delete
				deleteEvt.Object.SetResourceVersion("")
				Expect(deleteEvt.Object).To(Equal(deleted))
			})
		})

		// TODO(pwittrock): Write this test
		PContext("for a Foo CRD resource", func() {
			It("should provide Foo Events", func() {
			})
		})
	})

	Describe("Informer", func() {
		var c chan struct{}
		var rs *appsv1.ReplicaSet
		var depInformer toolscache.SharedIndexInformer
		var informerFactory kubeinformers.SharedInformerFactory
		var stopTest chan struct{}

		BeforeEach(func() {
			stopTest = make(chan struct{})
			informerFactory = kubeinformers.NewSharedInformerFactory(clientset, time.Second*30)
			depInformer = informerFactory.Apps().V1().ReplicaSets().Informer()
			informerFactory.Start(stopTest)
			Eventually(depInformer.HasSynced).Should(BeTrue())

			c = make(chan struct{})
			rs = &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{Name: "informer-rs-name"},
				Spec: appsv1.ReplicaSetSpec{
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

		AfterEach(func() {
			close(stopTest)
		})

		Context("for a ReplicaSet resource", func() {
			It("should provide a ReplicaSet CreateEvent", func() {
				c := make(chan struct{})

				q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
				instance := &source.Informer{Informer: depInformer}
				err := instance.Start(ctx, handler.Funcs{
					CreateFunc: func(ctx context.Context, evt event.CreateEvent, q2 workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						var err error
						rs, err := clientset.AppsV1().ReplicaSets("default").Get(ctx, rs.Name, metav1.GetOptions{})
						Expect(err).NotTo(HaveOccurred())

						Expect(q2).To(BeIdenticalTo(q))
						Expect(evt.Object).To(Equal(rs))
						close(c)
					},
					UpdateFunc: func(context.Context, event.UpdateEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected UpdateEvent")
					},
					DeleteFunc: func(context.Context, event.DeleteEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected DeleteEvent")
					},
					GenericFunc: func(context.Context, event.GenericEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected GenericEvent")
					},
				}, q)
				Expect(err).NotTo(HaveOccurred())

				_, err = clientset.AppsV1().ReplicaSets("default").Create(ctx, rs, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				<-c
			})

			It("should provide a ReplicaSet UpdateEvent", func() {
				var err error
				rs, err = clientset.AppsV1().ReplicaSets("default").Get(ctx, rs.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())

				rs2 := rs.DeepCopy()
				rs2.SetLabels(map[string]string{"biz": "baz"})

				q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
				instance := &source.Informer{Informer: depInformer}
				err = instance.Start(ctx, handler.Funcs{
					CreateFunc: func(ctx context.Context, evt event.CreateEvent, q2 workqueue.RateLimitingInterface) {
					},
					UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q2 workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						var err error
						rs2, err := clientset.AppsV1().ReplicaSets("default").Get(ctx, rs.Name, metav1.GetOptions{})
						Expect(err).NotTo(HaveOccurred())

						Expect(q2).To(Equal(q))
						Expect(evt.ObjectOld).To(Equal(rs))

						Expect(evt.ObjectNew).To(Equal(rs2))

						close(c)
					},
					DeleteFunc: func(context.Context, event.DeleteEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected DeleteEvent")
					},
					GenericFunc: func(context.Context, event.GenericEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected GenericEvent")
					},
				}, q)
				Expect(err).NotTo(HaveOccurred())

				_, err = clientset.AppsV1().ReplicaSets("default").Update(ctx, rs2, metav1.UpdateOptions{})
				Expect(err).NotTo(HaveOccurred())
				<-c
			})

			It("should provide a ReplicaSet DeletedEvent", func() {
				c := make(chan struct{})

				q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
				instance := &source.Informer{Informer: depInformer}
				err := instance.Start(ctx, handler.Funcs{
					CreateFunc: func(context.Context, event.CreateEvent, workqueue.RateLimitingInterface) {
					},
					UpdateFunc: func(context.Context, event.UpdateEvent, workqueue.RateLimitingInterface) {
					},
					DeleteFunc: func(ctx context.Context, evt event.DeleteEvent, q2 workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Expect(q2).To(Equal(q))
						Expect(evt.Object.GetName()).To(Equal(rs.Name))
						close(c)
					},
					GenericFunc: func(context.Context, event.GenericEvent, workqueue.RateLimitingInterface) {
						defer GinkgoRecover()
						Fail("Unexpected GenericEvent")
					},
				}, q)
				Expect(err).NotTo(HaveOccurred())

				err = clientset.AppsV1().ReplicaSets("default").Delete(ctx, rs.Name, metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())
				<-c
			})
		})
	})
})
