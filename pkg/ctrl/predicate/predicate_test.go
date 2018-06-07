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

package predicate_test

import (
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/event"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/predicate"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Predicate", func() {
	var pod *corev1.Pod
	BeforeEach(func() {
		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: "biz", Name: "baz"},
		}
	})

	Describe("PredicateFuncs", func() {
		failingFuncs := predicate.PredicateFuncs{
			CreateFunc: func(event.CreateEvent) bool {
				defer GinkgoRecover()
				Fail("Did not expect CreateFunc to be called.")
				return false
			},
			DeleteFunc: func(event.DeleteEvent) bool {
				defer GinkgoRecover()
				Fail("Did not expect DeleteFunc to be called.")
				return false
			},
			UpdateFunc: func(event.UpdateEvent) bool {
				defer GinkgoRecover()
				Fail("Did not expect UpdateFunc to be called.")
				return false
			},
			GenericFunc: func(event.GenericEvent) bool {
				defer GinkgoRecover()
				Fail("Did not expect GenericFunc to be called.")
				return false
			},
		}

		It("should call Create", func(done Done) {
			instance := failingFuncs
			instance.CreateFunc = func(evt event.CreateEvent) bool {
				defer GinkgoRecover()
				Expect(evt.Meta).To(Equal(pod.GetObjectMeta()))
				Expect(evt.Object).To(Equal(pod))
				return false
			}
			evt := event.CreateEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			Expect(instance.Create(evt)).To(BeFalse())

			instance.CreateFunc = func(evt event.CreateEvent) bool {
				defer GinkgoRecover()
				Expect(evt.Meta).To(Equal(pod.GetObjectMeta()))
				Expect(evt.Object).To(Equal(pod))
				return true
			}
			Expect(instance.Create(evt)).To(BeTrue())

			instance.CreateFunc = nil
			Expect(instance.Create(evt)).To(BeTrue())
			close(done)
		})

		It("should call Update", func(done Done) {
			newPod := pod.DeepCopy()
			newPod.Name = "baz2"
			newPod.Namespace = "biz2"

			instance := failingFuncs
			instance.UpdateFunc = func(evt event.UpdateEvent) bool {
				defer GinkgoRecover()
				Expect(evt.MetaOld).To(Equal(pod.GetObjectMeta()))
				Expect(evt.ObjectOld).To(Equal(pod))
				Expect(evt.MetaNew).To(Equal(newPod.GetObjectMeta()))
				Expect(evt.ObjectNew).To(Equal(newPod))
				return false
			}
			evt := event.UpdateEvent{
				ObjectOld: pod,
				MetaOld:   pod.GetObjectMeta(),
				ObjectNew: newPod,
				MetaNew:   newPod.GetObjectMeta(),
			}
			Expect(instance.Update(evt)).To(BeFalse())

			instance.UpdateFunc = func(evt event.UpdateEvent) bool {
				defer GinkgoRecover()
				Expect(evt.MetaOld).To(Equal(pod.GetObjectMeta()))
				Expect(evt.ObjectOld).To(Equal(pod))
				Expect(evt.MetaNew).To(Equal(newPod.GetObjectMeta()))
				Expect(evt.ObjectNew).To(Equal(newPod))
				return true
			}
			Expect(instance.Update(evt)).To(BeTrue())

			instance.UpdateFunc = nil
			Expect(instance.Update(evt)).To(BeTrue())
			close(done)
		})

		It("should call Delete", func(done Done) {
			instance := failingFuncs
			instance.DeleteFunc = func(evt event.DeleteEvent) bool {
				defer GinkgoRecover()
				Expect(evt.Meta).To(Equal(pod.GetObjectMeta()))
				Expect(evt.Object).To(Equal(pod))
				return false
			}
			evt := event.DeleteEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			Expect(instance.Delete(evt)).To(BeFalse())

			instance.DeleteFunc = func(evt event.DeleteEvent) bool {
				defer GinkgoRecover()
				Expect(evt.Meta).To(Equal(pod.GetObjectMeta()))
				Expect(evt.Object).To(Equal(pod))
				return true
			}
			Expect(instance.Delete(evt)).To(BeTrue())

			instance.DeleteFunc = nil
			Expect(instance.Delete(evt)).To(BeTrue())
			close(done)
		})

		It("should call Generic", func(done Done) {
			instance := failingFuncs
			instance.GenericFunc = func(evt event.GenericEvent) bool {
				defer GinkgoRecover()
				Expect(evt.Meta).To(Equal(pod.GetObjectMeta()))
				Expect(evt.Object).To(Equal(pod))
				return false
			}
			evt := event.GenericEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			Expect(instance.Generic(evt)).To(BeFalse())

			instance.GenericFunc = func(evt event.GenericEvent) bool {
				defer GinkgoRecover()
				Expect(evt.Meta).To(Equal(pod.GetObjectMeta()))
				Expect(evt.Object).To(Equal(pod))
				return true
			}
			Expect(instance.Generic(evt)).To(BeTrue())

			instance.GenericFunc = nil
			Expect(instance.Generic(evt)).To(BeTrue())
			close(done)
		})
	})
})
