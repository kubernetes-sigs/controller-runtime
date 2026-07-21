/*
Copyright 2026 The Kubernetes Authors.

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

package cache

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	toolscache "k8s.io/client-go/tools/cache"
)

var _ = Describe("TypedInformer", func() {
	It("adapts typed event handlers", func() {
		informer := &recordingInformer{}
		typedInformer := NewTypedInformer[*corev1.Pod](informer)

		var added *corev1.Pod
		var initialList bool
		var oldPod, newPod *corev1.Pod
		var deleted toolscache.DeletedObject[*corev1.Pod]
		_, err := typedInformer.AddTypedEventHandler(toolscache.TypedResourceEventHandlerDetailedFuncs[*corev1.Pod]{
			AddFunc: func(obj *corev1.Pod, isInInitialList bool) {
				added = obj
				initialList = isInInitialList
			},
			UpdateFunc: func(oldObj, newObj *corev1.Pod) {
				oldPod = oldObj
				newPod = newObj
			},
			DeleteFunc: func(obj toolscache.DeletedObject[*corev1.Pod]) {
				deleted = obj
			},
		}, toolscache.HandlerOptions{})
		Expect(err).NotTo(HaveOccurred())

		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}}
		updatedPod := pod.DeepCopy()
		updatedPod.ResourceVersion = "2"
		informer.handler.OnAdd(pod, true)
		informer.handler.OnUpdate(pod, updatedPod)
		informer.handler.OnDelete(pod)

		Expect(added).To(BeIdenticalTo(pod))
		Expect(initialList).To(BeTrue())
		Expect(oldPod).To(BeIdenticalTo(pod))
		Expect(newPod).To(BeIdenticalTo(updatedPod))
		Expect(deleted.OptionalObj).To(BeIdenticalTo(pod))
		Expect(deleted.FinalStateUnknown).To(BeNil())

		tombstone := toolscache.DeletedFinalStateUnknown{Key: "default/pod", Obj: pod}
		informer.handler.OnDelete(tombstone)
		Expect(deleted.OptionalObj).To(BeIdenticalTo(pod))
		Expect(deleted.FinalStateUnknown).NotTo(BeNil())
		Expect(*deleted.FinalStateUnknown).To(Equal(tombstone))

		tombstone.Obj = nil
		informer.handler.OnDelete(tombstone)
		Expect(deleted.OptionalObj).To(BeNil())
		Expect(deleted.FinalStateUnknown).NotTo(BeNil())
		Expect(*deleted.FinalStateUnknown).To(Equal(tombstone))
	})

	It("adapts typed event handler options", func() {
		informer := &recordingInformer{}
		typedInformer := NewTypedInformer[*corev1.Pod](informer)

		_, err := typedInformer.AddTypedEventHandler(toolscache.TypedResourceEventHandlerFuncs[*corev1.Pod]{})
		Expect(err).NotTo(HaveOccurred())
		Expect(informer.options).To(Equal(toolscache.HandlerOptions{}))

		resyncPeriod := time.Minute
		resync := toolscache.HandlerOptions{ResyncPeriod: &resyncPeriod}
		_, err = typedInformer.AddTypedEventHandler(toolscache.TypedResourceEventHandlerFuncs[*corev1.Pod]{}, resync)
		Expect(err).NotTo(HaveOccurred())
		Expect(informer.options).To(Equal(resync))

		_, err = typedInformer.AddTypedEventHandler(toolscache.TypedResourceEventHandlerFuncs[*corev1.Pod]{}, toolscache.HandlerOptions{}, toolscache.HandlerOptions{})
		Expect(err).To(MatchError("at most one HandlerOptions may be passed, got 2"))
	})

	It("adapts typed indexers", func() {
		informer := &recordingInformer{}
		typedInformer := NewTypedInformer[*corev1.Pod](informer)

		err := typedInformer.AddTypedIndexers(toolscache.TypedIndexers[*corev1.Pod]{
			"node": func(pod *corev1.Pod) ([]string, error) {
				return []string{pod.Spec.NodeName}, nil
			},
		})
		Expect(err).NotTo(HaveOccurred())

		values, err := informer.indexers["node"](&corev1.Pod{Spec: corev1.PodSpec{NodeName: "node-a"}})
		Expect(err).NotTo(HaveOccurred())
		Expect(values).To(Equal([]string{"node-a"}))
	})
})

type recordingInformer struct {
	Informer
	handler  toolscache.ResourceEventHandler
	options  toolscache.HandlerOptions
	indexers toolscache.Indexers
}

func (i *recordingInformer) AddEventHandlerWithOptions(handler toolscache.ResourceEventHandler, options toolscache.HandlerOptions) (toolscache.ResourceEventHandlerRegistration, error) {
	i.handler = handler
	i.options = options
	return nil, nil
}

func (i *recordingInformer) AddIndexers(indexers toolscache.Indexers) error {
	i.indexers = indexers
	return nil
}
