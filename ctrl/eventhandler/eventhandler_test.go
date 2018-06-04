package eventhandler_test

import (
	"time"

	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/event"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/eventhandler"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/reconcile"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
)

var _ = Describe("Eventhandler", func() {
	var q workqueue.RateLimitingInterface
	var instance eventhandler.EnqueueHandler
	var pod *corev1.Pod
	BeforeEach(func() {
		q = Queue{workqueue.New()}
		instance = eventhandler.EnqueueHandler{}
		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: "biz", Name: "baz"},
		}
	})

	Describe("EnqueueHandler", func() {
		It("should enqueue a ReconcileRequest with the Name / Namespace of the object in the CreateEvent.", func(done Done) {
			evt := event.CreateEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			instance.Create(q, evt)
			Expect(q.Len()).To(Equal(1))

			i, _ := q.Get()
			Expect(i).NotTo(BeNil())
			req, ok := i.(reconcile.ReconcileRequest)
			Expect(ok).To(BeTrue())
			Expect(req.NamespacedName).To(Equal(types.NamespacedName{Namespace: "biz", Name: "baz"}))

			close(done)
		})

		It("should enqueue a ReconcileRequest with the Name / Namespace of the object in the DeleteEvent.", func(done Done) {
			evt := event.DeleteEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			instance.Delete(q, evt)
			Expect(q.Len()).To(Equal(1))

			i, _ := q.Get()
			Expect(i).NotTo(BeNil())
			req, ok := i.(reconcile.ReconcileRequest)
			Expect(ok).To(BeTrue())
			Expect(req.NamespacedName).To(Equal(types.NamespacedName{Namespace: "biz", Name: "baz"}))

			close(done)
		})

		It("should enqueue a ReconcileRequest with the Name / Namespace of both objects in the UpdateEvent.",
			func(done Done) {
				newPod := pod.DeepCopy()
				newPod.Name = "baz2"
				newPod.Namespace = "biz2"

				evt := event.UpdateEvent{
					ObjectOld: pod,
					MetaOld:   pod.GetObjectMeta(),
					ObjectNew: newPod,
					MetaNew:   newPod.GetObjectMeta(),
				}
				instance.Update(q, evt)
				Expect(q.Len()).To(Equal(2))

				i, _ := q.Get()
				Expect(i).NotTo(BeNil())
				req, ok := i.(reconcile.ReconcileRequest)
				Expect(ok).To(BeTrue())
				Expect(req.NamespacedName).To(Equal(types.NamespacedName{Namespace: "biz", Name: "baz"}))

				i, _ = q.Get()
				Expect(i).NotTo(BeNil())
				req, ok = i.(reconcile.ReconcileRequest)
				Expect(ok).To(BeTrue())
				Expect(req.NamespacedName).To(Equal(types.NamespacedName{Namespace: "biz2", Name: "baz2"}))

				close(done)
			})

		It("should enqueue a ReconcileRequest with the Name / Namespace of the object in the GenericEvent.", func(done Done) {
			evt := event.GenericEvent{
				Object: pod,
				Meta:   pod.GetObjectMeta(),
			}
			instance.Generic(q, evt)
			Expect(q.Len()).To(Equal(1))
			i, _ := q.Get()
			Expect(i).NotTo(BeNil())
			req, ok := i.(reconcile.ReconcileRequest)
			Expect(ok).To(BeTrue())
			Expect(req.NamespacedName).To(Equal(types.NamespacedName{Namespace: "biz", Name: "baz"}))

			close(done)
		})

		Context("for a runtime.Object without Metadata", func() {
			It("should do nothing if the Metadata is missing for a CreateEvent.", func(done Done) {
				evt := event.CreateEvent{
					Object: pod,
				}
				instance.Create(q, evt)
				Expect(q.Len()).To(Equal(0))
				close(done)
			})

			It("should do nothing if the Metadata is missing for a UpdateEvent.", func(done Done) {
				newPod := pod.DeepCopy()
				newPod.Name = "baz2"
				newPod.Namespace = "biz2"

				evt := event.UpdateEvent{
					ObjectNew: newPod,
					MetaNew:   newPod.GetObjectMeta(),
					ObjectOld: pod,
				}
				instance.Update(q, evt)
				Expect(q.Len()).To(Equal(1))
				i, _ := q.Get()
				Expect(i).NotTo(BeNil())
				req, ok := i.(reconcile.ReconcileRequest)
				Expect(ok).To(BeTrue())
				Expect(req.NamespacedName).To(Equal(types.NamespacedName{Namespace: "biz2", Name: "baz2"}))

				evt.MetaNew = nil
				evt.MetaOld = pod.GetObjectMeta()
				instance.Update(q, evt)
				Expect(q.Len()).To(Equal(1))
				i, _ = q.Get()
				Expect(i).NotTo(BeNil())
				req, ok = i.(reconcile.ReconcileRequest)
				Expect(ok).To(BeTrue())
				Expect(req.NamespacedName).To(Equal(types.NamespacedName{Namespace: "biz", Name: "baz"}))

				close(done)
			})

			It("should do nothing if the Metadata is missing for a DeleteEvent.", func(done Done) {
				evt := event.DeleteEvent{
					Object: pod,
				}
				instance.Delete(q, evt)
				Expect(q.Len()).To(Equal(0))
				close(done)
			})

			It("should do nothing if the Metadata is missing for a GenericEvent.", func(done Done) {
				evt := event.GenericEvent{
					Object: pod,
				}
				instance.Generic(q, evt)
				Expect(q.Len()).To(Equal(0))
				close(done)
			})
		})
	})

	//Describe("EnqueueMappedHandler", func() {
	//	It("should enqueue a ReconcileRequest with the function applied to the CreateEvent.", func() {
	//		instance := eventhandler.EnqueueMappedHandler{}
	//
	//	})
	//
	//	It("should enqueue a ReconcileRequest with the function applied to the DeleteEvent.", func() {
	//		instance := eventhandler.EnqueueMappedHandler{}
	//
	//	})
	//
	//	It("should enqueue a ReconcileRequest with the function applied to both objects in the UpdateEvent.",
	//		func() {
	//			instance := eventhandler.EnqueueMappedHandler{}
	//
	//		})
	//
	//	It("should enqueue a ReconcileRequest with the function applied to the GenericEvent.", func() {
	//		instance := eventhandler.EnqueueMappedHandler{}
	//
	//	})
	//})
	//
	//Describe("EnqueueOwnerHandler", func() {
	//	It("should enqueue a ReconcileRequest with the Owner of the object in the CreateEvent.", func() {
	//		instance := eventhandler.EnqueueMappedHandler{}
	//
	//	})
	//
	//	It("should enqueue a ReconcileRequest with the Owner of the object in the DeleteEvent.", func() {
	//		instance := eventhandler.EnqueueMappedHandler{}
	//
	//	})
	//
	//	It("should enqueue a ReconcileRequest with the Owners of both objects in the UpdateEvent.", func() {
	//		instance := eventhandler.EnqueueMappedHandler{}
	//
	//	})
	//
	//	It("should enqueue a ReconcileRequest with the Owner of the object in the GenericEvent.", func() {
	//		instance := eventhandler.EnqueueMappedHandler{}
	//	})
	//
	//	It("should not enqueue a ReconcileRequest if there are no matching owners.", func() {
	//		instance := eventhandler.EnqueueMappedHandler{}
	//	})
	//
	//	It("should not enqueue a ReconcileRequest if there are no owners.", func() {
	//		instance := eventhandler.EnqueueMappedHandler{}
	//	})
	//
	//	Context("with the Controller field set to true", func() {
	//		It("should only enqueue ReconcileRequests for the Controller if there are multiple owners.", func() {
	//			instance := eventhandler.EnqueueMappedHandler{}
	//		})
	//
	//		It("should not enqueue ReconcileRequests if there are no Controller owners.", func() {
	//			instance := eventhandler.EnqueueMappedHandler{}
	//		})
	//
	//		It("should not enqueue ReconcileRequests if there are no owners.", func() {
	//			instance := eventhandler.EnqueueMappedHandler{}
	//		})
	//	})
	//
	//	Context("with the Controller field set to false", func() {
	//		It("should enqueue ReconcileRequests for all owners.", func() {
	//			instance := eventhandler.EnqueueMappedHandler{}
	//		})
	//	})
	//
	//})
})

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
