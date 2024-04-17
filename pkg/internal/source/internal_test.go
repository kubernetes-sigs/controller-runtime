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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	internal "sigs.k8s.io/controller-runtime/pkg/internal/source"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllertest"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func newFunc[T any](_ T) *handler.ObjectFuncs[T] {
	return &handler.ObjectFuncs[T]{
		CreateFunc: func(context.Context, T, workqueue.RateLimitingInterface) {
			defer GinkgoRecover()
			Fail("Did not expect CreateEvent to be called.")
		},
		DeleteFunc: func(context.Context, T, workqueue.RateLimitingInterface) {
			defer GinkgoRecover()
			Fail("Did not expect DeleteEvent to be called.")
		},
		UpdateFunc: func(context.Context, T, T, workqueue.RateLimitingInterface) {
			defer GinkgoRecover()
			Fail("Did not expect UpdateEvent to be called.")
		},
		GenericFunc: func(context.Context, T, workqueue.RateLimitingInterface) {
			defer GinkgoRecover()
			Fail("Did not expect GenericEvent to be called.")
		},
	}
}

func newSetFunc[T any](_ T, set *bool) *handler.ObjectFuncs[T] {
	return &handler.ObjectFuncs[T]{
		CreateFunc: func(context.Context, T, workqueue.RateLimitingInterface) {
			*set = true
		},
		DeleteFunc: func(context.Context, T, workqueue.RateLimitingInterface) {
			*set = true
		},
		UpdateFunc: func(context.Context, T, T, workqueue.RateLimitingInterface) {
			*set = true
		},
		GenericFunc: func(context.Context, T, workqueue.RateLimitingInterface) {
			*set = true
		},
	}
}

var _ = Describe("Internal", func() {
	var ctx = context.Background()
	var instance *internal.EventHandler[*corev1.Pod]
	var partialMetadataInstance *internal.EventHandler[*metav1.PartialObjectMetadata]
	var funcs, setfuncs *handler.ObjectFuncs[*corev1.Pod]
	var metafuncs, metasetfuncs *handler.ObjectFuncs[*metav1.PartialObjectMetadata]
	var set bool
	BeforeEach(func() {
		funcs = newFunc(&corev1.Pod{})
		setfuncs = newSetFunc(&corev1.Pod{}, &set)
		metafuncs = newFunc(&metav1.PartialObjectMetadata{})
		metasetfuncs = newSetFunc(&metav1.PartialObjectMetadata{}, &set)

		instance = internal.NewEventHandler(ctx, &controllertest.Queue{}, funcs, nil)
		partialMetadataInstance = internal.NewEventHandler(ctx, &controllertest.Queue{}, metafuncs, nil)
	})

	Describe("EventHandler", func() {
		var pod, newPod *corev1.Pod
		var pom, newPom *metav1.PartialObjectMetadata

		BeforeEach(func() {
			pod = &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "test", Image: "test"}},
				},
			}
			newPod = pod.DeepCopy()
			newPod.Labels = map[string]string{"foo": "bar"}
			pom = &metav1.PartialObjectMetadata{}
			newPom = pom.DeepCopy()
			newPom.Labels = map[string]string{"foo": "bar"}
		})

		It("should create a CreateEvent", func() {
			funcs.CreateFunc = func(ctx context.Context, obj *corev1.Pod, q workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Expect(obj).To(Equal(pod))
			}
			instance.OnAdd(pod)
		})

		It("should used Predicates to filter CreateEvents", func() {
			instance = internal.NewEventHandler(ctx, &controllertest.Queue{}, setfuncs, []predicate.ObjectPredicate[*corev1.Pod]{
				predicate.ObjectFuncs[*corev1.Pod]{CreateFunc: func(*corev1.Pod) bool { return false }},
			})
			set = false
			instance.OnAdd(pod)
			Expect(set).To(BeFalse())

			set = false
			instance = internal.NewEventHandler(ctx, &controllertest.Queue{}, setfuncs, []predicate.ObjectPredicate[*corev1.Pod]{
				predicate.ObjectFuncs[*corev1.Pod]{CreateFunc: func(*corev1.Pod) bool {
					return true
				}},
			})
			instance.OnAdd(pod)
			Expect(set).To(BeTrue())

			set = false
			instance = internal.NewEventHandler(ctx, &controllertest.Queue{}, setfuncs, []predicate.ObjectPredicate[*corev1.Pod]{
				predicate.ObjectFuncs[*corev1.Pod]{CreateFunc: func(*corev1.Pod) bool { return true }},
				predicate.ObjectFuncs[*corev1.Pod]{CreateFunc: func(*corev1.Pod) bool { return false }},
			})
			instance.OnAdd(pod)
			Expect(set).To(BeFalse())

			set = false
			instance = internal.NewEventHandler(ctx, &controllertest.Queue{}, setfuncs, []predicate.ObjectPredicate[*corev1.Pod]{
				predicate.ObjectFuncs[*corev1.Pod]{CreateFunc: func(*corev1.Pod) bool { return false }},
				predicate.ObjectFuncs[*corev1.Pod]{CreateFunc: func(*corev1.Pod) bool { return true }},
			})
			instance.OnAdd(pod)
			Expect(set).To(BeFalse())

			set = false
			instance = internal.NewEventHandler(ctx, &controllertest.Queue{}, setfuncs, []predicate.ObjectPredicate[*corev1.Pod]{
				predicate.ObjectFuncs[*corev1.Pod]{CreateFunc: func(*corev1.Pod) bool { return true }},
				predicate.ObjectFuncs[*corev1.Pod]{CreateFunc: func(*corev1.Pod) bool { return true }},
			})
			instance.OnAdd(pod)
			Expect(set).To(BeTrue())
		})

		It("should use Predicates to filter CreateEvents on PartialObjectMetadata", func() {
			partialMetadataInstance = internal.NewEventHandler(ctx, &controllertest.Queue{}, metasetfuncs, []predicate.ObjectPredicate[*metav1.PartialObjectMetadata]{
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{CreateFunc: func(obj *metav1.PartialObjectMetadata) bool { return false }},
			})
			set = false
			partialMetadataInstance.OnAdd(pom)
			Expect(set).To(BeFalse())

			set = false
			partialMetadataInstance = internal.NewEventHandler(ctx, &controllertest.Queue{}, metasetfuncs, []predicate.ObjectPredicate[*metav1.PartialObjectMetadata]{
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{CreateFunc: func(*metav1.PartialObjectMetadata) bool { return true }},
			})
			partialMetadataInstance.OnAdd(pom)
			Expect(set).To(BeTrue())

			set = false
			partialMetadataInstance = internal.NewEventHandler(ctx, &controllertest.Queue{}, metasetfuncs, []predicate.ObjectPredicate[*metav1.PartialObjectMetadata]{
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{CreateFunc: func(*metav1.PartialObjectMetadata) bool { return true }},
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{CreateFunc: func(*metav1.PartialObjectMetadata) bool { return false }},
			})
			partialMetadataInstance.OnAdd(pom)
			Expect(set).To(BeFalse())

			set = false
			partialMetadataInstance = internal.NewEventHandler(ctx, &controllertest.Queue{}, metasetfuncs, []predicate.ObjectPredicate[*metav1.PartialObjectMetadata]{
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{CreateFunc: func(*metav1.PartialObjectMetadata) bool { return false }},
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{CreateFunc: func(*metav1.PartialObjectMetadata) bool { return true }},
			})
			partialMetadataInstance.OnAdd(pom)
			Expect(set).To(BeFalse())

			set = false
			partialMetadataInstance = internal.NewEventHandler(ctx, &controllertest.Queue{}, metasetfuncs, []predicate.ObjectPredicate[*metav1.PartialObjectMetadata]{
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{CreateFunc: func(*metav1.PartialObjectMetadata) bool { return true }},
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{CreateFunc: func(*metav1.PartialObjectMetadata) bool { return true }},
			})
			partialMetadataInstance.OnAdd(pom)
			Expect(set).To(BeTrue())
		})

		It("should not call Create EventHandler if the object is not a runtime.Object", func() {
			instance.OnAdd(&metav1.ObjectMeta{})
		})

		It("should not call Create EventHandler if an object is not 'that' object", func() {
			instance.OnAdd(&corev1.Secret{})
		})

		It("should not call Create EventHandler if the object does not have metadata", func() {
			instance.OnAdd(FooRuntimeObject{})
		})

		It("should not call Create EventHandler if an object is not a partial object metadata object", func() {
			partialMetadataInstance.OnAdd(&corev1.Secret{})
		})

		It("should create an UpdateEvent", func() {
			funcs.UpdateFunc = func(ctx context.Context, old *corev1.Pod, new *corev1.Pod, q workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Expect(old).To(Equal(pod))
				Expect(new).To(Equal(newPod))
			}
			instance.OnUpdate(pod, newPod)
		})

		It("should used Predicates to filter UpdateEvents", func() {
			set = false
			instance = internal.NewEventHandler(ctx, &controllertest.Queue{}, setfuncs, []predicate.ObjectPredicate[*corev1.Pod]{
				predicate.ObjectFuncs[*corev1.Pod]{UpdateFunc: func(old, new *corev1.Pod) bool { return false }},
			})
			instance.OnUpdate(pod, newPod)
			Expect(set).To(BeFalse())

			set = false
			instance = internal.NewEventHandler(ctx, &controllertest.Queue{}, setfuncs, []predicate.ObjectPredicate[*corev1.Pod]{
				predicate.ObjectFuncs[*corev1.Pod]{UpdateFunc: func(old, new *corev1.Pod) bool { return true }},
			})
			instance.OnUpdate(pod, newPod)
			Expect(set).To(BeTrue())

			set = false
			instance = internal.NewEventHandler(ctx, &controllertest.Queue{}, setfuncs, []predicate.ObjectPredicate[*corev1.Pod]{
				predicate.ObjectFuncs[*corev1.Pod]{UpdateFunc: func(old, new *corev1.Pod) bool { return true }},
				predicate.ObjectFuncs[*corev1.Pod]{UpdateFunc: func(old, new *corev1.Pod) bool { return false }},
			})
			instance.OnUpdate(pod, newPod)
			Expect(set).To(BeFalse())

			set = false
			instance = internal.NewEventHandler(ctx, &controllertest.Queue{}, setfuncs, []predicate.ObjectPredicate[*corev1.Pod]{
				predicate.ObjectFuncs[*corev1.Pod]{UpdateFunc: func(old, new *corev1.Pod) bool { return false }},
				predicate.ObjectFuncs[*corev1.Pod]{UpdateFunc: func(old, new *corev1.Pod) bool { return true }},
			})
			instance.OnUpdate(pod, newPod)
			Expect(set).To(BeFalse())

			set = false
			instance = internal.NewEventHandler(ctx, &controllertest.Queue{}, setfuncs, []predicate.ObjectPredicate[*corev1.Pod]{
				predicate.ObjectFuncs[*corev1.Pod]{CreateFunc: func(*corev1.Pod) bool { return true }},
				predicate.ObjectFuncs[*corev1.Pod]{CreateFunc: func(*corev1.Pod) bool { return true }},
			})
			instance.OnUpdate(pod, newPod)
			Expect(set).To(BeTrue())
		})

		It("should use Predicates to filter UpdateEvents on PartialObjectMetadata", func() {
			set = false
			partialMetadataInstance = internal.NewEventHandler(ctx, &controllertest.Queue{}, metasetfuncs, []predicate.ObjectPredicate[*metav1.PartialObjectMetadata]{
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{UpdateFunc: func(old, new *metav1.PartialObjectMetadata) bool { return false }},
			})
			partialMetadataInstance.OnUpdate(pom, newPom)
			Expect(set).To(BeFalse())

			set = false
			partialMetadataInstance = internal.NewEventHandler(ctx, &controllertest.Queue{}, metasetfuncs, []predicate.ObjectPredicate[*metav1.PartialObjectMetadata]{
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{UpdateFunc: func(old, new *metav1.PartialObjectMetadata) bool { return true }},
			})
			partialMetadataInstance.OnUpdate(pom, newPom)
			Expect(set).To(BeTrue())

			set = false
			partialMetadataInstance = internal.NewEventHandler(ctx, &controllertest.Queue{}, metasetfuncs, []predicate.ObjectPredicate[*metav1.PartialObjectMetadata]{
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{UpdateFunc: func(old, new *metav1.PartialObjectMetadata) bool { return true }},
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{UpdateFunc: func(old, new *metav1.PartialObjectMetadata) bool { return false }},
			})
			partialMetadataInstance.OnUpdate(pom, newPom)
			Expect(set).To(BeFalse())

			set = false
			partialMetadataInstance = internal.NewEventHandler(ctx, &controllertest.Queue{}, metasetfuncs, []predicate.ObjectPredicate[*metav1.PartialObjectMetadata]{
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{UpdateFunc: func(old, new *metav1.PartialObjectMetadata) bool { return false }},
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{UpdateFunc: func(old, new *metav1.PartialObjectMetadata) bool { return true }},
			})
			partialMetadataInstance.OnUpdate(pom, newPom)
			Expect(set).To(BeFalse())

			set = false
			partialMetadataInstance = internal.NewEventHandler(ctx, &controllertest.Queue{}, metasetfuncs, []predicate.ObjectPredicate[*metav1.PartialObjectMetadata]{
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{CreateFunc: func(obj *metav1.PartialObjectMetadata) bool { return true }},
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{CreateFunc: func(obj *metav1.PartialObjectMetadata) bool { return true }},
			})
			partialMetadataInstance.OnUpdate(pom, newPom)
			Expect(set).To(BeTrue())
		})

		It("should not call Update EventHandler if the object is not a runtime.Object", func() {
			instance.OnUpdate(&metav1.ObjectMeta{}, &corev1.Pod{})
			instance.OnUpdate(&corev1.Pod{}, &metav1.ObjectMeta{})
		})

		It("should not call Update EventHandler if an object is not 'that' object", func() {
			instance.OnUpdate(&corev1.Secret{}, &corev1.Pod{})
			instance.OnUpdate(&corev1.Pod{}, &corev1.ConfigMap{})
		})

		It("should not call Update EventHandler if an object is not a partial object metadata object", func() {
			partialMetadataInstance.OnUpdate(&corev1.Secret{}, &corev1.Pod{})
			partialMetadataInstance.OnUpdate(&metav1.PartialObjectMetadata{}, &corev1.ConfigMap{})
			partialMetadataInstance.OnUpdate(&corev1.ConfigMap{}, &metav1.PartialObjectMetadata{})
		})

		It("should not call Update EventHandler if the object does not have metadata", func() {
			instance.OnUpdate(&corev1.Pod{}, FooRuntimeObject{})
			instance.OnUpdate(FooRuntimeObject{}, &corev1.Pod{})
			instance.OnUpdate(FooRuntimeObject{}, FooRuntimeObject{})
		})

		It("should create a DeleteEvent", func() {
			funcs.DeleteFunc = func(ctx context.Context, obj *corev1.Pod, q workqueue.RateLimitingInterface) {
				defer GinkgoRecover()
				Expect(obj).To(Equal(pod))
			}
			instance.OnDelete(pod)
		})

		It("should used Predicates to filter DeleteEvents", func() {
			set = false
			instance = internal.NewEventHandler(ctx, &controllertest.Queue{}, setfuncs, []predicate.ObjectPredicate[*corev1.Pod]{
				predicate.ObjectFuncs[*corev1.Pod]{DeleteFunc: func(*corev1.Pod) bool { return false }},
			})
			instance.OnDelete(pod)
			Expect(set).To(BeFalse())

			set = false
			instance = internal.NewEventHandler(ctx, &controllertest.Queue{}, setfuncs, []predicate.ObjectPredicate[*corev1.Pod]{
				predicate.ObjectFuncs[*corev1.Pod]{DeleteFunc: func(*corev1.Pod) bool { return true }},
			})
			instance.OnDelete(pod)
			Expect(set).To(BeTrue())

			set = false
			instance = internal.NewEventHandler(ctx, &controllertest.Queue{}, setfuncs, []predicate.ObjectPredicate[*corev1.Pod]{
				predicate.ObjectFuncs[*corev1.Pod]{DeleteFunc: func(*corev1.Pod) bool { return true }},
				predicate.ObjectFuncs[*corev1.Pod]{DeleteFunc: func(*corev1.Pod) bool { return false }},
			})
			instance.OnDelete(pod)
			Expect(set).To(BeFalse())

			set = false
			instance = internal.NewEventHandler(ctx, &controllertest.Queue{}, setfuncs, []predicate.ObjectPredicate[*corev1.Pod]{
				predicate.ObjectFuncs[*corev1.Pod]{DeleteFunc: func(*corev1.Pod) bool { return false }},
				predicate.ObjectFuncs[*corev1.Pod]{DeleteFunc: func(*corev1.Pod) bool { return true }},
			})
			instance.OnDelete(pod)
			Expect(set).To(BeFalse())

			set = false
			instance = internal.NewEventHandler(ctx, &controllertest.Queue{}, setfuncs, []predicate.ObjectPredicate[*corev1.Pod]{
				predicate.ObjectFuncs[*corev1.Pod]{DeleteFunc: func(*corev1.Pod) bool { return true }},
				predicate.ObjectFuncs[*corev1.Pod]{DeleteFunc: func(*corev1.Pod) bool { return true }},
			})
			instance.OnDelete(pod)
			Expect(set).To(BeTrue())
		})

		It("should use Predicates to filter DeleteEvents", func() {
			set = false
			partialMetadataInstance = internal.NewEventHandler(ctx, &controllertest.Queue{}, metasetfuncs, []predicate.ObjectPredicate[*metav1.PartialObjectMetadata]{
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{DeleteFunc: func(*metav1.PartialObjectMetadata) bool { return false }},
			})
			partialMetadataInstance.OnDelete(pom)
			Expect(set).To(BeFalse())

			set = false
			partialMetadataInstance = internal.NewEventHandler(ctx, &controllertest.Queue{}, metasetfuncs, []predicate.ObjectPredicate[*metav1.PartialObjectMetadata]{
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{DeleteFunc: func(*metav1.PartialObjectMetadata) bool { return true }},
			})
			partialMetadataInstance.OnDelete(pom)
			Expect(set).To(BeTrue())

			set = false
			partialMetadataInstance = internal.NewEventHandler(ctx, &controllertest.Queue{}, metasetfuncs, []predicate.ObjectPredicate[*metav1.PartialObjectMetadata]{
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{DeleteFunc: func(*metav1.PartialObjectMetadata) bool { return true }},
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{DeleteFunc: func(*metav1.PartialObjectMetadata) bool { return false }},
			})
			partialMetadataInstance.OnDelete(pom)
			Expect(set).To(BeFalse())

			set = false
			partialMetadataInstance = internal.NewEventHandler(ctx, &controllertest.Queue{}, metasetfuncs, []predicate.ObjectPredicate[*metav1.PartialObjectMetadata]{
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{DeleteFunc: func(*metav1.PartialObjectMetadata) bool { return false }},
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{DeleteFunc: func(*metav1.PartialObjectMetadata) bool { return true }},
			})
			partialMetadataInstance.OnDelete(pom)
			Expect(set).To(BeFalse())

			set = false
			partialMetadataInstance = internal.NewEventHandler(ctx, &controllertest.Queue{}, metasetfuncs, []predicate.ObjectPredicate[*metav1.PartialObjectMetadata]{
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{DeleteFunc: func(*metav1.PartialObjectMetadata) bool { return true }},
				predicate.ObjectFuncs[*metav1.PartialObjectMetadata]{DeleteFunc: func(*metav1.PartialObjectMetadata) bool { return true }},
			})
			partialMetadataInstance.OnDelete(pom)
			Expect(set).To(BeTrue())
		})

		It("should not call Delete EventHandler if the object is not a runtime.Object", func() {
			instance.OnDelete(&metav1.ObjectMeta{})
		})

		It("should not call Delete EventHandler if an object is not 'that' object", func() {
			instance.OnDelete(&corev1.Secret{})
		})

		It("should not call Delete EventHandler if the object does not have metadata", func() {
			instance.OnDelete(FooRuntimeObject{})
		})

		It("should not call Delete EventHandler if an object is not a partial object metadata object", func() {
			partialMetadataInstance.OnDelete(&corev1.Secret{})
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
