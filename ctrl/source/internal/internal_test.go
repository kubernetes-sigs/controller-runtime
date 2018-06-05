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

package internal_test

import (
	"time"

	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/event"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/eventhandler"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/source/internal"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("Internal", func() {

	var instance internal.EventHandler
	var funcs *eventhandler.EventHandlerFuncs
	BeforeEach(func() {
		funcs = &eventhandler.EventHandlerFuncs{
			CreateFunc: func(workqueue.RateLimitingInterface, event.CreateEvent) {
				defer GinkgoRecover()
				Fail("Did not expect CreateEvent to be called.")
			},
			DeleteFunc: func(q workqueue.RateLimitingInterface, e event.DeleteEvent) {
				defer GinkgoRecover()
				Fail("Did not expect DeleteEvent to be called.")
			},
			UpdateFunc: func(workqueue.RateLimitingInterface, event.UpdateEvent) {
				defer GinkgoRecover()
				Fail("Did not expect UpdateEvent to be called.")
			},
			GenericFunc: func(workqueue.RateLimitingInterface, event.GenericEvent) {
				defer GinkgoRecover()
				Fail("Did not expect GenericEvent to be called.")
			},
		}
		instance = internal.EventHandler{
			Queue:        Queue{},
			EventHandler: funcs,
		}
	})

	Describe("EventHandler", func() {
		var pod, newPod *corev1.Pod

		BeforeEach(func() {
			pod = &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "test", Image: "test"}},
				},
			}
			newPod = pod.DeepCopy()
			newPod.Labels = map[string]string{"foo": "bar"}
		})

		It("should create a CreateEvent", func(done Done) {
			funcs.CreateFunc = func(q workqueue.RateLimitingInterface, evt event.CreateEvent) {
				defer GinkgoRecover()
				Expect(q).To(Equal(instance.Queue))
				m, err := meta.Accessor(pod)
				Expect(err).NotTo(HaveOccurred())
				Expect(evt.Meta).To(Equal(m))
				Expect(evt.Object).To(Equal(pod))
			}
			instance.OnAdd(pod)
			close(done)
		})

		It("should create an UpdateEvent", func(done Done) {
			funcs.UpdateFunc = func(q workqueue.RateLimitingInterface, evt event.UpdateEvent) {
				defer GinkgoRecover()
				Expect(q).To(Equal(instance.Queue))

				m, err := meta.Accessor(pod)
				Expect(err).NotTo(HaveOccurred())
				Expect(evt.MetaOld).To(Equal(m))
				Expect(evt.ObjectOld).To(Equal(pod))

				m, err = meta.Accessor(newPod)
				Expect(err).NotTo(HaveOccurred())
				Expect(evt.MetaNew).To(Equal(m))
				Expect(evt.ObjectNew).To(Equal(newPod))
			}
			instance.OnUpdate(pod, newPod)
			close(done)
		})

		It("should create a DeleteEvent", func(done Done) {
			funcs.DeleteFunc = func(q workqueue.RateLimitingInterface, evt event.DeleteEvent) {
				defer GinkgoRecover()
				Expect(q).To(Equal(instance.Queue))

				m, err := meta.Accessor(pod)
				Expect(err).NotTo(HaveOccurred())
				Expect(evt.Meta).To(Equal(m))
				Expect(evt.Object).To(Equal(pod))
			}
			instance.OnDelete(pod)
			close(done)
		})

		It("should create a DeleteEvent from a tombstone", func(done Done) {

			tombstone := cache.DeletedFinalStateUnknown{
				Obj: pod,
			}
			funcs.DeleteFunc = func(q workqueue.RateLimitingInterface, evt event.DeleteEvent) {
				defer GinkgoRecover()
				Expect(q).To(Equal(instance.Queue))
				m, err := meta.Accessor(pod)
				Expect(err).NotTo(HaveOccurred())
				Expect(evt.Meta).To(Equal(m))
				Expect(evt.Object).To(Equal(pod))
			}

			instance.OnDelete(tombstone)
			close(done)
		})

		It("should ignore tombstone objects without meta", func(done Done) {
			tombstone := cache.DeletedFinalStateUnknown{Obj: Foo{}}
			instance.OnDelete(tombstone)
			close(done)
		})
		It("should ignore objects without meta", func(done Done) {
			instance.OnAdd(Foo{})
			instance.OnUpdate(Foo{}, Foo{})
			instance.OnDelete(Foo{})
			close(done)
		})
	})
})

type Foo struct{}

var _ workqueue.RateLimitingInterface = Queue{}

type Queue struct {
	workqueue.Interface
}

// AddAfter adds an item to the workqueue after the indicated duration has passed
func (q Queue) AddAfter(item interface{}, duration time.Duration) {
	q.Add(item)
}

func (q Queue) AddRateLimited(item interface{}) {
	q.Add(item)
}

func (q Queue) Forget(item interface{}) {
	// Do nothing
}

func (q Queue) NumRequeues(item interface{}) int {
	return 0
}
