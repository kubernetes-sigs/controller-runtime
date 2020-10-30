package testingclient_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/testingclient"
)

var _ = Describe("Spy", func() {
	var (
		subject    testingclient.Spy
		fakeClient client.Client
		calls      chan testingclient.SpyCall
	)
	BeforeEach(func() {
		calls = make(chan testingclient.SpyCall, 1)
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

		subject = testingclient.Spy{
			Delegate: fakeClient,
			Calls:    calls,
		}
	})

	It("spies on calls to Get", func() {
		var obj corev1.Pod
		key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
		Expect(subject.Get(nil, key, &obj)).To(Succeed())
		var call testingclient.SpyCall
		Expect(calls).To(Receive(&call))
		Expect(call.GVK.String()).To(Equal("/v1, Kind=Pod"))
		Expect(call.Verb).To(Equal("get"))
		Expect(call.Key).To(Equal(key))

		var objInDelegate corev1.Pod
		Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())

		Expect(call.Obj).To(Equal(client.Object(&objInDelegate)), "call.Obj should be the retrieved object")
	})

	It("spies on calls to List", func() {
		var list corev1.PodList
		Expect(subject.List(nil, &list)).To(Succeed())
		var call testingclient.SpyCall
		Expect(calls).To(Receive(&call))
		Expect(call.GVK.String()).To(Equal("/v1, Kind=Pod"))
		Expect(call.Verb).To(Equal("list"))

		var listInDelegate corev1.PodList
		Expect(fakeClient.List(nil, &listInDelegate)).To(Succeed())
		Expect(listInDelegate.Items).To(HaveLen(2), "consistency check")

		Expect(call.List).To(Equal(client.ObjectList(&listInDelegate)), "call.Obj should be the retrieved list")
	})

	It("spies on calls to Create", func() {
		key := types.NamespacedName{Namespace: "ns", Name: "app1"}
		obj := appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: key.Namespace, Name: key.Name,
			},
		}
		Expect(subject.Create(nil, &obj)).To(Succeed())
		var call testingclient.SpyCall
		Expect(calls).To(Receive(&call))
		Expect(call.GVK.String()).To(Equal("apps/v1, Kind=Deployment"))
		Expect(call.Verb).To(Equal("create"))

		var objInDelegate appsv1.Deployment
		Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())

		objInDelegate.TypeMeta = metav1.TypeMeta{} // Clear TypeMeta for comparison. fakeClient.Get() fills this in.
		Expect(call.Obj).To(Equal(client.Object(&objInDelegate)), "call.Obj should be the created object")
	})

	It("spies on calls to Delete", func() {
		key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
		obj := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: key.Namespace, Name: key.Name,
			},
		}
		Expect(subject.Delete(nil, &obj)).To(Succeed())
		var call testingclient.SpyCall
		Expect(calls).To(Receive(&call))
		Expect(call.GVK.String()).To(Equal("/v1, Kind=Pod"))
		Expect(call.Verb).To(Equal("delete"))
		Expect(call.Obj).To(Equal(client.Object(&obj)), "call.Obj should be the deleted object")

		var objInDelegate corev1.Pod
		err := fakeClient.Get(nil, key, &objInDelegate)
		Expect(errors.IsNotFound(err)).To(BeTrue(), "The object should have been deleted in the delegate")
	})

	It("spies on calls to Update", func() {
		key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
		obj := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: key.Namespace, Name: key.Name,
				Annotations: map[string]string{
					"annotation1": "added-annotation",
				},
			},
		}
		Expect(subject.Update(nil, &obj)).To(Succeed())
		var call testingclient.SpyCall
		Expect(calls).To(Receive(&call))
		Expect(call.GVK.String()).To(Equal("/v1, Kind=Pod"))
		Expect(call.Verb).To(Equal("update"))

		var objInDelegate corev1.Pod
		Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())

		Expect(call.Obj.GetAnnotations()).To(HaveKeyWithValue("annotation1", "added-annotation"))
		objInDelegate.TypeMeta = metav1.TypeMeta{} // Clear TypeMeta for comparison. fakeClient.Get() fills this in.
		Expect(call.Obj).To(Equal(client.Object(&objInDelegate)), "call.Obj should be the updated object")
	})

	It("spies on calls to Patch", func() {
		key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
		obj := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: key.Namespace, Name: key.Name,
			},
		}
		Expect(subject.Patch(nil, &obj, client.RawPatch(types.MergePatchType,
			[]byte(`{"metadata":{"annotations":{"annotation1":"patched-annotation"}}}`))),
		).To(Succeed())
		var call testingclient.SpyCall
		Expect(calls).To(Receive(&call))
		Expect(call.GVK.String()).To(Equal("/v1, Kind=Pod"))
		Expect(call.Verb).To(Equal("patch"))

		var objInDelegate corev1.Pod
		Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())

		Expect(call.Obj.GetAnnotations()).To(HaveKeyWithValue("annotation1", "patched-annotation"))
		Expect(call.Obj).To(Equal(client.Object(&objInDelegate)), "call.Obj should be the updated object")
	})

	It("spies on calls to DeleteAllOf", func() {
		Expect(subject.DeleteAllOf(nil, &corev1.Pod{})).To(Succeed())
		var call testingclient.SpyCall
		Expect(calls).To(Receive(&call))
		Expect(call.GVK.String()).To(Equal("/v1, Kind=Pod"))
		Expect(call.Verb).To(Equal("deleteallof"))

		for _, key := range []types.NamespacedName{
			{Namespace: "ns", Name: "pod1"},
			{Namespace: "ns", Name: "pod2"},
		} {
			var objInDelegate corev1.Pod
			err := fakeClient.Get(nil, key, &objInDelegate)
			Expect(errors.IsNotFound(err)).To(BeTrue(), "The object %s should have been deleted in the delegate", key)
		}
	})

	Context("StatusWriter", func() {
		It("spies on calls to Status().Update()", func() {
			key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
			obj := corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: key.Namespace, Name: key.Name,
				},
				Status: corev1.PodStatus{
					Message: "The status is good",
				},
			}
			Expect(subject.Status().Update(nil, &obj)).To(Succeed())
			var call testingclient.SpyCall
			Expect(calls).To(Receive(&call))
			Expect(call.GVK.String()).To(Equal("/v1, Kind=Pod"))
			Expect(call.Verb).To(Equal("update"))
			Expect(call.IsStatus).To(BeTrue())

			var objInDelegate corev1.Pod
			Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())

			Expect(call.Obj.(*corev1.Pod).Status.Message).To(Equal("The status is good"))
			objInDelegate.TypeMeta = metav1.TypeMeta{} // Clear TypeMeta for comparison. fakeClient.Get() fills this in.
			Expect(call.Obj).To(Equal(client.Object(&objInDelegate)), "call.Obj should be the updated object")
		})

		It("spies on calls to Status().Patch()", func() {
			key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
			obj := corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: key.Namespace, Name: key.Name,
				},
			}
			Expect(subject.Status().Patch(nil, &obj, client.RawPatch(types.MergePatchType,
				[]byte(`{"status":{"message":"The status has been patched"}}`))),
			).To(Succeed())
			var call testingclient.SpyCall
			Expect(calls).To(Receive(&call))
			Expect(call.GVK.String()).To(Equal("/v1, Kind=Pod"))
			Expect(call.Verb).To(Equal("patch"))
			Expect(call.IsStatus).To(BeTrue())

			var objInDelegate corev1.Pod
			Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())

			Expect(call.Obj.(*corev1.Pod).Status.Message).To(Equal("The status has been patched"))
			Expect(call.Obj).To(Equal(client.Object(&objInDelegate)), "call.Obj should be the updated object")
		})
	})
})
