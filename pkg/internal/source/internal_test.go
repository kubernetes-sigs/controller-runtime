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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	internal "sigs.k8s.io/controller-runtime/pkg/internal/source"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllertest"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ = Describe("Internal", func() {
	ctx := context.Background()
	var instance *internal.EventHandler[*corev1.Pod]
	var funcs, setfuncs *handler.Funcs[*corev1.Pod]
	var set bool
	BeforeEach(func() {
		funcs = &handler.Funcs[*corev1.Pod]{
			CreateFunc: func(context.Context, event.CreateEvent[*corev1.Pod], workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Fail("Did not expect CreateEvent to be called.")
			},
			DeleteFunc: func(context.Context, event.DeleteEvent[*corev1.Pod], workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Fail("Did not expect DeleteEvent to be called.")
			},
			UpdateFunc: func(context.Context, event.UpdateEvent[*corev1.Pod], workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Fail("Did not expect UpdateEvent to be called.")
			},
			GenericFunc: func(context.Context, event.GenericEvent[*corev1.Pod], workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Fail("Did not expect GenericEvent to be called.")
			},
		}

		setfuncs = &handler.Funcs[*corev1.Pod]{
			CreateFunc: func(context.Context, event.CreateEvent[*corev1.Pod], workqueue.RateLimitingInterface) {
				set = true
			},
			DeleteFunc: func(context.Context, event.DeleteEvent[*corev1.Pod], workqueue.RateLimitingInterface) {
				set = true
			},
			UpdateFunc: func(context.Context, event.UpdateEvent[*corev1.Pod], workqueue.RateLimitingInterface) {
				set = true
			},
			GenericFunc: func(context.Context, event.GenericEvent[*corev1.Pod], workqueue.RateLimitingInterface) {
				set = true
			},
		}
		instance = internal.NewEventHandler[*corev1.Pod](ctx, controllertest.Queue{}, funcs, nil)
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

		It("should create a CreateEvent", func() {
			funcs.CreateFunc = func(ctx context.Context, evt event.CreateEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Expect(evt.Object).To(Equal(pod))
			}
			instance.OnAdd(pod)
		})

		It("should used Predicates to filter CreateEvents", func() {
			instance = internal.NewEventHandler[*corev1.Pod](ctx, controllertest.Queue{}, setfuncs, []predicate.Predicate[*corev1.Pod]{
				predicate.Funcs[*corev1.Pod]{CreateFunc: func(event.CreateEvent[*corev1.Pod]) bool { return false }},
			})
			set = false
			instance.OnAdd(pod)
			Expect(set).To(BeFalse())

			set = false
			instance = internal.NewEventHandler[*corev1.Pod](ctx, controllertest.Queue{}, setfuncs, []predicate.Predicate[*corev1.Pod]{
				predicate.Funcs[*corev1.Pod]{CreateFunc: func(event.CreateEvent[*corev1.Pod]) bool { return true }},
			})
			instance.OnAdd(pod)
			Expect(set).To(BeTrue())

			set = false
			instance = internal.NewEventHandler[*corev1.Pod](ctx, controllertest.Queue{}, setfuncs, []predicate.Predicate[*corev1.Pod]{
				predicate.Funcs[*corev1.Pod]{CreateFunc: func(event.CreateEvent[*corev1.Pod]) bool { return true }},
				predicate.Funcs[*corev1.Pod]{CreateFunc: func(event.CreateEvent[*corev1.Pod]) bool { return false }},
			})
			instance.OnAdd(pod)
			Expect(set).To(BeFalse())

			set = false
			instance = internal.NewEventHandler[*corev1.Pod](ctx, controllertest.Queue{}, setfuncs, []predicate.Predicate[*corev1.Pod]{
				predicate.Funcs[*corev1.Pod]{CreateFunc: func(event.CreateEvent[*corev1.Pod]) bool { return false }},
				predicate.Funcs[*corev1.Pod]{CreateFunc: func(event.CreateEvent[*corev1.Pod]) bool { return true }},
			})
			instance.OnAdd(pod)
			Expect(set).To(BeFalse())

			set = false
			instance = internal.NewEventHandler[*corev1.Pod](ctx, controllertest.Queue{}, setfuncs, []predicate.Predicate[*corev1.Pod]{
				predicate.Funcs[*corev1.Pod]{CreateFunc: func(event.CreateEvent[*corev1.Pod]) bool { return true }},
				predicate.Funcs[*corev1.Pod]{CreateFunc: func(event.CreateEvent[*corev1.Pod]) bool { return true }},
			})
			instance.OnAdd(pod)
			Expect(set).To(BeTrue())
		})

		It("should not call Create EventHandler if the object is not a runtime.Object", func() {
			instance.OnAdd(&metav1.ObjectMeta{})
		})

		It("should not call Create EventHandler if the object does not have metadata", func() {
			instance.OnAdd(FooRuntimeObject{})
		})

		It("should create an UpdateEvent", func() {
			funcs.UpdateFunc = func(ctx context.Context, evt event.UpdateEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Expect(evt.ObjectOld).To(Equal(pod))
				Expect(evt.ObjectNew).To(Equal(newPod))
			}
			instance.OnUpdate(pod, newPod)
		})

		It("should used Predicates to filter UpdateEvents", func() {
			set = false
			instance = internal.NewEventHandler[*corev1.Pod](ctx, controllertest.Queue{}, setfuncs, []predicate.Predicate[*corev1.Pod]{
				predicate.Funcs[*corev1.Pod]{UpdateFunc: func(updateEvent event.UpdateEvent[*corev1.Pod]) bool { return false }},
			})
			instance.OnUpdate(pod, newPod)
			Expect(set).To(BeFalse())

			set = false
			instance = internal.NewEventHandler[*corev1.Pod](ctx, controllertest.Queue{}, setfuncs, []predicate.Predicate[*corev1.Pod]{
				predicate.Funcs[*corev1.Pod]{UpdateFunc: func(event.UpdateEvent[*corev1.Pod]) bool { return true }},
			})
			instance.OnUpdate(pod, newPod)
			Expect(set).To(BeTrue())

			set = false
			instance = internal.NewEventHandler[*corev1.Pod](ctx, controllertest.Queue{}, setfuncs, []predicate.Predicate[*corev1.Pod]{
				predicate.Funcs[*corev1.Pod]{UpdateFunc: func(event.UpdateEvent[*corev1.Pod]) bool { return true }},
				predicate.Funcs[*corev1.Pod]{UpdateFunc: func(event.UpdateEvent[*corev1.Pod]) bool { return false }},
			})
			instance.OnUpdate(pod, newPod)
			Expect(set).To(BeFalse())

			set = false
			instance = internal.NewEventHandler[*corev1.Pod](ctx, controllertest.Queue{}, setfuncs, []predicate.Predicate[*corev1.Pod]{
				predicate.Funcs[*corev1.Pod]{UpdateFunc: func(event.UpdateEvent[*corev1.Pod]) bool { return false }},
				predicate.Funcs[*corev1.Pod]{UpdateFunc: func(event.UpdateEvent[*corev1.Pod]) bool { return true }},
			})
			instance.OnUpdate(pod, newPod)
			Expect(set).To(BeFalse())

			set = false
			instance = internal.NewEventHandler[*corev1.Pod](ctx, controllertest.Queue{}, setfuncs, []predicate.Predicate[*corev1.Pod]{
				predicate.Funcs[*corev1.Pod]{CreateFunc: func(event.CreateEvent[*corev1.Pod]) bool { return true }},
				predicate.Funcs[*corev1.Pod]{CreateFunc: func(event.CreateEvent[*corev1.Pod]) bool { return true }},
			})
			instance.OnUpdate(pod, newPod)
			Expect(set).To(BeTrue())
		})

		It("should not call Update EventHandler if the object is not a runtime.Object", func() {
			instance.OnUpdate(&metav1.ObjectMeta{}, &corev1.Pod{})
			instance.OnUpdate(&corev1.Pod{}, &metav1.ObjectMeta{})
		})

		It("should not call Update EventHandler if the object does not have metadata", func() {
			instance.OnUpdate(FooRuntimeObject{}, &corev1.Pod{})
			instance.OnUpdate(&corev1.Pod{}, FooRuntimeObject{})
		})

		It("should create a DeleteEvent", func() {
			funcs.DeleteFunc = func(ctx context.Context, evt event.DeleteEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Expect(evt.Object).To(Equal(pod))
			}
			instance.OnDelete(pod)
		})

		It("should used Predicates to filter DeleteEvents", func() {
			set = false
			instance = internal.NewEventHandler[*corev1.Pod](ctx, controllertest.Queue{}, setfuncs, []predicate.Predicate[*corev1.Pod]{
				predicate.Funcs[*corev1.Pod]{DeleteFunc: func(event.DeleteEvent[*corev1.Pod]) bool { return false }},
			})
			instance.OnDelete(pod)
			Expect(set).To(BeFalse())

			set = false
			instance = internal.NewEventHandler[*corev1.Pod](ctx, controllertest.Queue{}, setfuncs, []predicate.Predicate[*corev1.Pod]{
				predicate.Funcs[*corev1.Pod]{DeleteFunc: func(event.DeleteEvent[*corev1.Pod]) bool { return true }},
			})
			instance.OnDelete(pod)
			Expect(set).To(BeTrue())

			set = false
			instance = internal.NewEventHandler[*corev1.Pod](ctx, controllertest.Queue{}, setfuncs, []predicate.Predicate[*corev1.Pod]{
				predicate.Funcs[*corev1.Pod]{DeleteFunc: func(event.DeleteEvent[*corev1.Pod]) bool { return true }},
				predicate.Funcs[*corev1.Pod]{DeleteFunc: func(event.DeleteEvent[*corev1.Pod]) bool { return false }},
			})
			instance.OnDelete(pod)
			Expect(set).To(BeFalse())

			set = false
			instance = internal.NewEventHandler[*corev1.Pod](ctx, controllertest.Queue{}, setfuncs, []predicate.Predicate[*corev1.Pod]{
				predicate.Funcs[*corev1.Pod]{DeleteFunc: func(event.DeleteEvent[*corev1.Pod]) bool { return false }},
				predicate.Funcs[*corev1.Pod]{DeleteFunc: func(event.DeleteEvent[*corev1.Pod]) bool { return true }},
			})
			instance.OnDelete(pod)
			Expect(set).To(BeFalse())

			set = false
			instance = internal.NewEventHandler[*corev1.Pod](ctx, controllertest.Queue{}, setfuncs, []predicate.Predicate[*corev1.Pod]{
				predicate.Funcs[*corev1.Pod]{DeleteFunc: func(event.DeleteEvent[*corev1.Pod]) bool { return true }},
				predicate.Funcs[*corev1.Pod]{DeleteFunc: func(event.DeleteEvent[*corev1.Pod]) bool { return true }},
			})
			instance.OnDelete(pod)
			Expect(set).To(BeTrue())
		})

		It("should not call Delete EventHandler if the object is not a runtime.Object", func() {
			instance.OnDelete(&metav1.ObjectMeta{})
		})

		It("should not call Delete EventHandler if the object does not have metadata", func() {
			instance.OnDelete(FooRuntimeObject{})
		})

		It("should create a DeleteEvent from a tombstone", func() {
			tombstone := cache.DeletedFinalStateUnknown{
				Obj: pod,
			}
			funcs.DeleteFunc = func(ctx context.Context, evt event.DeleteEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Expect(evt.Object).To(Equal(pod))
				Expect(evt.DeleteStateUnknown).Should(BeTrue())
			}

			instance.OnDelete(tombstone)
		})

		It("should ignore tombstone objects without meta", func() {
			tombstone := cache.DeletedFinalStateUnknown{Obj: Foo{}}
			instance.OnDelete(tombstone)
		})
		It("should ignore objects without meta", func() {
			instance.OnAdd(Foo{})
			instance.OnUpdate(Foo{}, Foo{})
			instance.OnDelete(Foo{})
		})
	})
})

type Foo struct{}

var _ runtime.Object = FooRuntimeObject{}

type FooRuntimeObject struct{}

func (FooRuntimeObject) GetObjectKind() schema.ObjectKind { return nil }
func (FooRuntimeObject) DeepCopyObject() runtime.Object   { return nil }
