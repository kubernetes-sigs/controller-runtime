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

	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/source"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
)

var _ = Describe("Source", func() {
	var instance1, instance2 *source.Kind
	var obj runtime.Object
	var q workqueue.RateLimitingInterface
	var c1, c2 chan interface{}
	var ns string
	count := 0

	BeforeEach(func(done Done) {
		// Create the namespace for the test
		ns = fmt.Sprintf("controller-source-kindsource-%v", count)
		count++
		_, err := clientset.CoreV1().Namespaces().Create(&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		})
		Expect(err).NotTo(HaveOccurred())

		q = workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
		c1 = make(chan interface{})
		c2 = make(chan interface{})

		close(done)
	})

	JustBeforeEach(func() {
		instance1 = &source.Kind{Type: obj}
		inject.CacheInto(icache, instance1)

		instance2 = &source.Kind{Type: obj}
		inject.CacheInto(icache, instance2)
	})

	AfterEach(func(done Done) {
		err := clientset.CoreV1().Namespaces().Delete(ns, &metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())
		close(c1)
		close(c2)

		close(done)
	})

	Describe("Kind", func() {
		Context("for a Deployment resource", func() {
			obj = &appsv1.Deployment{}

			It("should provide Deployment Events", func(done Done) {
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
				newHandler := func(c chan interface{}) handler.Funcs {
					return handler.Funcs{
						CreateFunc: func(evt event.CreateEvent, rli workqueue.RateLimitingInterface) {
							defer GinkgoRecover()
							Expect(rli).To(Equal(q))
							c <- evt
						},
						UpdateFunc: func(evt event.UpdateEvent, rli workqueue.RateLimitingInterface) {
							defer GinkgoRecover()
							Expect(rli).To(Equal(q))
							c <- evt
						},
						DeleteFunc: func(evt event.DeleteEvent, rli workqueue.RateLimitingInterface) {
							defer GinkgoRecover()
							Expect(rli).To(Equal(q))
							c <- evt
						},
					}
				}
				handler1 := newHandler(c1)
				handler2 := newHandler(c2)

				// Create 2 instances
				instance1.Start(handler1, q)
				instance2.Start(handler2, q)

				By("Creating a Deployment and expecting the CreateEvent.")
				created, err = client.Create(deployment)
				Expect(err).NotTo(HaveOccurred())
				Expect(created).NotTo(BeNil())

				// Check first CreateEvent
				evt := <-c1
				createEvt, ok := evt.(event.CreateEvent)
				Expect(ok).To(BeTrue(), fmt.Sprintf("expect %T to be %T", evt, event.CreateEvent{}))
				Expect(createEvt.Meta).To(Equal(created))
				Expect(createEvt.Object).To(Equal(created))

				// Check second CreateEvent
				evt = <-c2
				createEvt, ok = evt.(event.CreateEvent)
				Expect(ok).To(BeTrue(), fmt.Sprintf("expect %T to be %T", evt, event.CreateEvent{}))
				Expect(createEvt.Meta).To(Equal(created))
				Expect(createEvt.Object).To(Equal(created))

				By("Updating a Deployment and expecting the UpdateEvent.")
				updated = created.DeepCopy()
				updated.Labels = map[string]string{"biz": "buz"}
				updated, err = client.Update(updated)
				Expect(err).NotTo(HaveOccurred())

				// Check first UpdateEvent
				evt = <-c1
				updateEvt, ok := evt.(event.UpdateEvent)
				Expect(ok).To(BeTrue(), fmt.Sprintf("expect %T to be %T", evt, event.UpdateEvent{}))

				Expect(updateEvt.MetaNew).To(Equal(updated))
				Expect(updateEvt.ObjectNew).To(Equal(updated))

				Expect(updateEvt.MetaOld).To(Equal(created))
				Expect(updateEvt.ObjectOld).To(Equal(created))

				// Check second UpdateEvent
				evt = <-c2
				updateEvt, ok = evt.(event.UpdateEvent)
				Expect(ok).To(BeTrue(), fmt.Sprintf("expect %T to be %T", evt, event.UpdateEvent{}))

				Expect(updateEvt.MetaNew).To(Equal(updated))
				Expect(updateEvt.ObjectNew).To(Equal(updated))

				Expect(updateEvt.MetaOld).To(Equal(created))
				Expect(updateEvt.ObjectOld).To(Equal(created))

				By("Deleting a Deployment and expecting the Delete.")
				deleted = updated.DeepCopy()
				err = client.Delete(created.Name, &metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

				deleted.SetResourceVersion("")
				evt = <-c1
				deleteEvt, ok := evt.(event.DeleteEvent)
				Expect(ok).To(BeTrue(), fmt.Sprintf("expect %T to be %T", evt, event.DeleteEvent{}))
				deleteEvt.Meta.SetResourceVersion("")
				Expect(deleteEvt.Meta).To(Equal(deleted))
				Expect(deleteEvt.Object).To(Equal(deleted))

				evt = <-c2
				deleteEvt, ok = evt.(event.DeleteEvent)
				Expect(ok).To(BeTrue(), fmt.Sprintf("expect %T to be %T", evt, event.DeleteEvent{}))
				deleteEvt.Meta.SetResourceVersion("")
				Expect(deleteEvt.Meta).To(Equal(deleted))
				Expect(deleteEvt.Object).To(Equal(deleted))

				close(done)
			}, 5)
		})

		// TODO(pwittrock): Write this test
		Context("for a Foo CRD resource", func() {
			It("should provide Foo Events", func() {

			})
		})
	})
})
