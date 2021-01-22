package testingclient_test

import (
	"errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/testingclient"
)

var exampleError = errors.New("injected error")

var _ = Describe("ErrorInjector", func() {
	var (
		subject    *testingclient.ErrorInjector
		fakeClient client.Client
	)
	BeforeEach(func() {
		fakeClient = testingclient.NewFakeClientWithScheme(scheme.Scheme)

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

		subject = testingclient.NewErrorInjector(fakeClient)
	})

	Describe("Get", func() {
		It("can inject errors on calls to Get", func() {
			subject.InjectError("get", &corev1.Pod{}, client.ObjectKey{Name: "pod1", Namespace: "ns"}, exampleError)

			key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
			var obj corev1.Pod
			Expect(subject.Get(nil, key, &obj)).To(MatchError("injected error"))
		})

		It("calls the delegate client if no errors match", func() {
			subject.InjectError("get", &corev1.Service{}, testingclient.AnyObject, exampleError)

			key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
			var obj corev1.Pod
			Expect(subject.Get(nil, key, &obj)).To(Succeed())

			var objInDelegate corev1.Pod
			Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())

			Expect(obj).To(Equal(objInDelegate), "obj should be the retrieved object")
		})
	})

	Describe("Create", func() {
		pod3 := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod3",
				Namespace: "ns",
			},
		}
		It("can inject errors on calls to Create", func() {
			subject.InjectError("create", &corev1.Pod{}, client.ObjectKey{Name: "pod3", Namespace: "ns"}, exampleError)

			Expect(subject.Create(nil, &pod3)).To(MatchError("injected error"))

			key := types.NamespacedName{Namespace: "ns", Name: "pod3"}
			var objInDelegate corev1.Pod
			err := fakeClient.Get(nil, key, &objInDelegate)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "should not create the object if an error was injected")
		})

		It("calls the delegate client if no errors match", func() {
			subject.InjectError("create", &corev1.Service{}, testingclient.AnyObject, exampleError)

			Expect(subject.Create(nil, &pod3)).To(Succeed())

			key := types.NamespacedName{Namespace: "ns", Name: "pod3"}
			var objInDelegate corev1.Pod
			Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())

			objInDelegate.TypeMeta = metav1.TypeMeta{} // Clear TypeMeta for comparison. fakeClient.Get() fills this in.
			Expect(pod3).To(Equal(objInDelegate), "obj should be the retrieved object")
		})
	})

	Describe("Delete", func() {
		It("can inject errors on calls to Delete", func() {
			subject.InjectError("delete", &corev1.Pod{}, client.ObjectKey{Name: "pod1", Namespace: "ns"}, exampleError)

			key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
			var obj corev1.Pod
			Expect(fakeClient.Get(nil, key, &obj)).To(Succeed())
			Expect(subject.Delete(nil, &obj)).To(MatchError("injected error"))

			var objInDelegate corev1.Pod
			Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())
			Expect(obj).To(Equal(objInDelegate), "pod1 should still exist")
		})

		It("calls the delegate client if no errors match", func() {
			subject.InjectError("delete", &corev1.Service{}, testingclient.AnyObject, exampleError)

			key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
			obj := corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: key.Namespace, Name: key.Name,
				},
			}
			Expect(subject.Delete(nil, &obj)).To(Succeed())

			var objInDelegate corev1.Pod
			err := fakeClient.Get(nil, key, &objInDelegate)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "should delete the object if no error was injected")

		})
	})

	Describe("Update", func() {
		It("can inject errors on calls to Update", func() {
			subject.InjectError("update", &corev1.Pod{}, client.ObjectKey{Name: "pod1", Namespace: "ns"}, exampleError)

			key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
			var obj corev1.Pod
			Expect(fakeClient.Get(nil, key, &obj)).To(Succeed())
			origObj := obj.DeepCopy()
			obj.Spec.NodeName = "lymph"
			Expect(subject.Update(nil, &obj)).To(MatchError("injected error"))

			var objInDelegate corev1.Pod
			Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())
			Expect(objInDelegate).To(Equal(*origObj), "pod1 should be unmodified")
		})

		It("calls the delegate client if no errors match", func() {
			subject.InjectError("update", &corev1.Service{}, testingclient.AnyObject, exampleError)

			key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
			var obj corev1.Pod
			Expect(fakeClient.Get(nil, key, &obj)).To(Succeed())
			obj.Spec.NodeName = "lymph"
			Expect(subject.Update(nil, &obj)).To(Succeed())

			var objInDelegate corev1.Pod
			Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())
			Expect(objInDelegate).To(Equal(obj), "pod1 should be modified")
		})
	})

	Describe("Patch", func() {
		It("can inject errors on calls to Patch", func() {
			subject.InjectError("patch", &corev1.Pod{}, client.ObjectKey{Name: "pod1", Namespace: "ns"}, exampleError)

			key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
			var obj corev1.Pod
			Expect(fakeClient.Get(nil, key, &obj)).To(Succeed())
			origObj := obj.DeepCopy()
			obj.Spec.NodeName = "lymph"
			Expect(subject.Patch(nil, &obj, client.MergeFrom(origObj))).To(MatchError("injected error"))

			var objInDelegate corev1.Pod
			Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())
			Expect(objInDelegate).To(Equal(*origObj), "pod1 should be unmodified")
		})

		It("calls the delegate client if no errors match", func() {
			subject.InjectError("patch", &corev1.Service{}, testingclient.AnyObject, exampleError)

			key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
			var obj corev1.Pod
			Expect(fakeClient.Get(nil, key, &obj)).To(Succeed())
			origObj := obj.DeepCopy()
			obj.Spec.NodeName = "lymph"
			Expect(subject.Patch(nil, &obj, client.MergeFrom(origObj))).To(Succeed())

			var objInDelegate corev1.Pod
			Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())
			Expect(objInDelegate).To(Equal(obj), "pod1 should be modified")
		})
	})

	// TODO: test prefers specific matches over general matches.
})
