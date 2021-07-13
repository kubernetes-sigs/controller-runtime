package testingclient_test

import (
	"errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/testingclient"
)

var _ = Describe("Reactive", func() {
	var (
		subject    *testingclient.Reactive
		fakeClient client.Client
	)
	BeforeEach(func() {
		fakeClient = testingclient.NewFakeClientBuilder().WithScheme(scheme.Scheme).Build()

		examplePod := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod",
				Namespace: "ns",
			},
		}

		pod1 := examplePod.DeepCopy()
		pod1.Name = "pod1"
		pod2 := examplePod.DeepCopy()
		pod2.Name = "pod2"

		Expect(fakeClient.Create(nil, pod1)).To(Succeed())
		Expect(fakeClient.Create(nil, pod2)).To(Succeed())

		subject = testingclient.NewReactiveClient(fakeClient)
	})

	Describe("Get", func() {
		key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
		var (
			obj    *corev1.Pod
			apiErr error
		)
		JustBeforeEach(func() {
			obj = new(corev1.Pod)
			apiErr = subject.Get(nil, key, obj)
		})

		When("the reactor 'handles' the action", func() {
			var reactorCalled bool
			BeforeEach(func() {
				reactorCalled = false
				subject.PrependReactor(testingclient.GetVerb, &corev1.Pod{}, func(action testing.Action) (handled bool, ret runtime.Object, err error) {
					reactorCalled = true
					return true, nil, errors.New("reactor error")
				})
			})

			It("calls the reactor", func() {
				Expect(reactorCalled).To(BeTrue())
				Expect(apiErr).To(MatchError("reactor error"))
			})

			It("doesn't call the delegate", func() {
				Expect(obj.GetName()).To(BeEmpty())
			})
		})

		When("the reactor doesn't 'handle' the action", func() {
			var reactorCalled bool
			BeforeEach(func() {
				reactorCalled = false
				subject.PrependReactor(testingclient.GetVerb, &corev1.Pod{}, func(action testing.Action) (handled bool, ret runtime.Object, err error) {
					reactorCalled = true
					return false, nil, nil
				})
			})

			It("calls the reactor", func() {
				Expect(reactorCalled).To(BeTrue())
			})

			It("calls the delegate client", func() {
				Expect(apiErr).NotTo(HaveOccurred())

				var objInDelegate corev1.Pod
				Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())

				Expect(obj).To(gstruct.PointTo(Equal(objInDelegate)), "obj should be the retrieved object")
			})
		})
	})

	Describe("List", func() {
		var (
			list   *corev1.PodList
			apiErr error
		)
		JustBeforeEach(func() {
			list = new(corev1.PodList)
			apiErr = subject.List(nil, list)
		})

		When("the reactor 'handles' the action", func() {
			var reactorCalled bool
			BeforeEach(func() {
				reactorCalled = false
				subject.PrependReactor(testingclient.ListVerb, &corev1.Pod{}, func(action testing.Action) (handled bool, ret runtime.Object, err error) {
					reactorCalled = true
					return true, nil, errors.New("reactor error")
				})
			})

			It("calls the reactor", func() {
				Expect(reactorCalled).To(BeTrue())
				Expect(apiErr).To(MatchError("reactor error"))
			})

			It("doesn't call the delegate", func() {
				Expect(list.Items).To(BeEmpty())
			})
		})

		When("the reactor doesn't 'handle' the action", func() {
			var reactorCalled bool
			BeforeEach(func() {
				reactorCalled = false
				subject.PrependReactor(testingclient.ListVerb, &corev1.Pod{}, func(action testing.Action) (handled bool, ret runtime.Object, err error) {
					reactorCalled = true
					return false, nil, nil
				})
			})

			It("calls the reactor", func() {
				Expect(reactorCalled).To(BeTrue())
			})

			It("calls the delegate client", func() {
				Expect(apiErr).NotTo(HaveOccurred())

				var listInDelegate corev1.PodList
				Expect(fakeClient.List(nil, &listInDelegate)).To(Succeed())

				Expect(list).To(gstruct.PointTo(Equal(listInDelegate)), "list should be the retrieved list")
			})
		})
	})

	Describe("Create", func() {
		key := types.NamespacedName{Namespace: "ns", Name: "pod3"}
		var (
			obj    *corev1.Pod
			apiErr error
		)
		JustBeforeEach(func() {
			obj = &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
			}
			apiErr = subject.Create(nil, obj)
		})

		When("the reactor 'handles' the action", func() {
			var reactorCalled bool
			BeforeEach(func() {
				reactorCalled = false
				subject.PrependReactor(testingclient.CreateVerb, &corev1.Pod{}, func(action testing.Action) (handled bool, ret runtime.Object, err error) {
					reactorCalled = true
					return true, nil, errors.New("reactor error")
				})
			})

			It("calls the reactor", func() {
				Expect(reactorCalled).To(BeTrue())
				Expect(apiErr).To(MatchError("reactor error"))
			})

			It("doesn't call the delegate", func() {
				var objInDelegate corev1.Pod
				err := fakeClient.Get(nil, key, &objInDelegate)
				Expect(apierrors.IsNotFound(err)).To(BeTrue(), "should not create the object")
			})
		})

		When("the reactor doesn't 'handle' the action", func() {
			var reactorCalled bool
			BeforeEach(func() {
				reactorCalled = false
				subject.PrependReactor(testingclient.CreateVerb, &corev1.Pod{}, func(action testing.Action) (handled bool, ret runtime.Object, err error) {
					reactorCalled = true
					return false, nil, nil
				})
			})

			It("calls the reactor", func() {
				Expect(reactorCalled).To(BeTrue())
			})

			It("calls the delegate client", func() {
				Expect(apiErr).NotTo(HaveOccurred())

				var objInDelegate corev1.Pod
				Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())
				objInDelegate.ResourceVersion = "" // Clear this for comparison. It is set by fake.Client.Create()

				Expect(obj).To(gstruct.PointTo(Equal(objInDelegate)), "obj should be the retrieved object")
			})
		})
	})

	Describe("Delete", func() {
		key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
		var (
			obj    corev1.Pod
			apiErr error
		)
		JustBeforeEach(func() {
			Expect(fakeClient.Get(nil, key, &obj)).To(Succeed())
			apiErr = subject.Delete(nil, &obj)
		})

		When("the reactor 'handles' the action", func() {
			var reactorCalled bool
			BeforeEach(func() {
				reactorCalled = false
				subject.PrependReactor(testingclient.DeleteVerb, &corev1.Pod{}, func(action testing.Action) (handled bool, ret runtime.Object, err error) {
					reactorCalled = true
					return true, nil, errors.New("reactor error")
				})
			})

			It("calls the reactor", func() {
				Expect(reactorCalled).To(BeTrue())
				Expect(apiErr).To(MatchError("reactor error"))
			})

			It("doesn't call the delegate", func() {
				var objInDelegate corev1.Pod
				Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())

				Expect(obj).To(Equal(objInDelegate), "obj should still exist in the delegate")
			})
		})

		When("the reactor doesn't 'handle' the action", func() {
			var reactorCalled bool
			BeforeEach(func() {
				reactorCalled = false
				subject.PrependReactor(testingclient.DeleteVerb, &corev1.Pod{}, func(action testing.Action) (handled bool, ret runtime.Object, err error) {
					reactorCalled = true
					return false, nil, nil
				})
			})

			It("calls the reactor", func() {
				Expect(reactorCalled).To(BeTrue())
			})

			It("calls the delegate client", func() {
				Expect(apiErr).NotTo(HaveOccurred())

				var objInDelegate corev1.Pod
				err := fakeClient.Get(nil, key, &objInDelegate)
				Expect(apierrors.IsNotFound(err)).To(BeTrue(), "object should have been deleted")
			})
		})
	})

	Describe("Update", func() {
		key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
		var (
			obj    corev1.Pod
			apiErr error
		)
		JustBeforeEach(func() {
			Expect(fakeClient.Get(nil, key, &obj)).To(Succeed())
			Expect(obj.Spec.Hostname).To(BeEmpty(), "consistency check")
			updatedObj := obj.DeepCopy()
			updatedObj.Spec.Hostname = "new-hostname"
			apiErr = subject.Update(nil, updatedObj)
		})

		When("the reactor 'handles' the action", func() {
			var reactorCalled bool
			BeforeEach(func() {
				reactorCalled = false
				subject.PrependReactor(testingclient.UpdateVerb, &corev1.Pod{}, func(action testing.Action) (handled bool, ret runtime.Object, err error) {
					reactorCalled = true
					return true, nil, errors.New("reactor error")
				})
			})

			It("calls the reactor", func() {
				Expect(reactorCalled).To(BeTrue())
				Expect(apiErr).To(MatchError("reactor error"))
			})

			It("doesn't call the delegate", func() {
				var objInDelegate corev1.Pod
				Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())
				Expect(objInDelegate.Spec.Hostname).To(BeEmpty(), "obj should not have been updated")
			})
		})

		When("the reactor doesn't 'handle' the action", func() {
			var reactorCalled bool
			BeforeEach(func() {
				reactorCalled = false
				subject.PrependReactor(testingclient.UpdateVerb, &corev1.Pod{}, func(action testing.Action) (handled bool, ret runtime.Object, err error) {
					reactorCalled = true
					return false, nil, nil
				})
			})

			It("calls the reactor", func() {
				Expect(reactorCalled).To(BeTrue())
			})

			It("calls the delegate client", func() {
				Expect(apiErr).NotTo(HaveOccurred())

				var objInDelegate corev1.Pod
				Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())
				Expect(objInDelegate.Spec.Hostname).To(Equal("new-hostname"), "obj should have been updated")
			})
		})
	})

	Describe("Patch", func() {
		key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
		var (
			obj    corev1.Pod
			apiErr error
		)
		JustBeforeEach(func() {
			Expect(fakeClient.Get(nil, key, &obj)).To(Succeed())
			Expect(obj.Spec.Hostname).To(BeEmpty(), "consistency check")
			updatedObj := obj.DeepCopy()
			updatedObj.Spec.Hostname = "new-hostname"
			apiErr = subject.Patch(nil, updatedObj, client.MergeFrom(&obj))
		})

		When("the reactor 'handles' the action", func() {
			var reactorCalled bool
			BeforeEach(func() {
				reactorCalled = false
				subject.PrependReactor(testingclient.PatchVerb, &corev1.Pod{}, func(action testing.Action) (handled bool, ret runtime.Object, err error) {
					reactorCalled = true
					return true, nil, errors.New("reactor error")
				})
			})

			It("calls the reactor", func() {
				Expect(reactorCalled).To(BeTrue())
				Expect(apiErr).To(MatchError("reactor error"))
			})

			It("doesn't call the delegate", func() {
				var objInDelegate corev1.Pod
				Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())
				Expect(objInDelegate.Spec.Hostname).To(BeEmpty(), "obj should not have been updated")
			})
		})

		When("the reactor doesn't 'handle' the action", func() {
			var reactorCalled bool
			BeforeEach(func() {
				reactorCalled = false
				subject.PrependReactor(testingclient.PatchVerb, &corev1.Pod{}, func(action testing.Action) (handled bool, ret runtime.Object, err error) {
					reactorCalled = true
					return false, nil, nil
				})
			})

			It("calls the reactor", func() {
				Expect(reactorCalled).To(BeTrue())
			})

			It("calls the delegate client", func() {
				Expect(apiErr).NotTo(HaveOccurred())

				var objInDelegate corev1.Pod
				Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())
				Expect(objInDelegate.Spec.Hostname).To(Equal("new-hostname"), "obj should have been updated")
			})
		})
	})
})

// TODO: test for assertion failure if List isn't passed something that implements client.ObjectList / metav1.ListInterface
// TODO: test for assertion failure if Get isn't passed something that implements client.Object / metav1.Object
