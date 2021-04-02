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

		subject = testingclient.NewErrorInjector(fakeClient)
	})

	Describe("Get", func() {
		It("can inject errors on calls to Get", func() {
			subject.InjectError(testingclient.GetVerb, &corev1.Pod{}, client.ObjectKey{Name: "pod1", Namespace: "ns"}, exampleError)

			key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
			var obj corev1.Pod
			Expect(subject.Get(nil, key, &obj)).To(MatchError("injected error"))
		})

		It("calls the delegate client if no errors match", func() {
			subject.InjectError(testingclient.GetVerb, &corev1.Service{}, testingclient.AnyObject, exampleError)

			key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
			var obj corev1.Pod
			Expect(subject.Get(nil, key, &obj)).To(Succeed())

			var objInDelegate corev1.Pod
			Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())

			Expect(obj).To(Equal(objInDelegate), "obj should be the retrieved object")
		})

		It("also accepts actions as a string", func() {
			subject.InjectError("get", &corev1.Pod{}, client.ObjectKey{Name: "pod1", Namespace: "ns"}, exampleError)

			key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
			var obj corev1.Pod
			Expect(subject.Get(nil, key, &obj)).To(MatchError("injected error"))
		})

		It("does not return injected error for invalid actions", func() {
			subject.InjectError("invalid", &corev1.Pod{}, client.ObjectKey{Name: "pod1", Namespace: "ns"}, exampleError)

			key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
			var obj corev1.Pod
			Expect(subject.Get(nil, key, &obj)).To(Succeed())
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
			subject.InjectError(testingclient.CreateVerb, &corev1.Pod{}, client.ObjectKey{Name: "pod3", Namespace: "ns"}, exampleError)

			Expect(subject.Create(nil, &pod3)).To(MatchError("injected error"))

			key := types.NamespacedName{Namespace: "ns", Name: "pod3"}
			var objInDelegate corev1.Pod
			err := fakeClient.Get(nil, key, &objInDelegate)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "should not create the object if an error was injected")
		})

		It("calls the delegate client if no errors match", func() {
			subject.InjectError(testingclient.CreateVerb, &corev1.Service{}, testingclient.AnyObject, exampleError)

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
			subject.InjectError(testingclient.DeleteVerb, &corev1.Pod{}, client.ObjectKey{Name: "pod1", Namespace: "ns"}, exampleError)

			key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
			var obj corev1.Pod
			Expect(fakeClient.Get(nil, key, &obj)).To(Succeed())
			Expect(subject.Delete(nil, &obj)).To(MatchError("injected error"))

			var objInDelegate corev1.Pod
			Expect(fakeClient.Get(nil, key, &objInDelegate)).To(Succeed())
			Expect(obj).To(Equal(objInDelegate), "pod1 should still exist")
		})

		It("calls the delegate client if no errors match", func() {
			subject.InjectError(testingclient.DeleteVerb, &corev1.Service{}, testingclient.AnyObject, exampleError)

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
			subject.InjectError(testingclient.UpdateVerb, &corev1.Pod{}, client.ObjectKey{Name: "pod1", Namespace: "ns"}, exampleError)

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
			subject.InjectError(testingclient.UpdateVerb, &corev1.Service{}, testingclient.AnyObject, exampleError)

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
			subject.InjectError(testingclient.PatchVerb, &corev1.Pod{}, client.ObjectKey{Name: "pod1", Namespace: "ns"}, exampleError)

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
			subject.InjectError(testingclient.PatchVerb, &corev1.Service{}, testingclient.AnyObject, exampleError)

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

	It("works with all combinations of wildcards", func() {
		pod1Key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
		var pod1 corev1.Pod
		Expect(subject.Get(nil, pod1Key, &pod1)).To(Succeed())
		pod2Key := types.NamespacedName{Namespace: "ns", Name: "pod2"}
		var pod2 corev1.Pod
		Expect(subject.Get(nil, pod2Key, &pod2)).To(Succeed())

		subject.InjectError(testingclient.GetVerb, &corev1.Pod{}, pod1Key, errors.New("error 1"))
		subject.InjectError(testingclient.GetVerb, &corev1.Pod{}, testingclient.AnyObject, errors.New("error 2"))
		subject.InjectError(testingclient.AnyVerb, &corev1.Pod{}, pod1Key, errors.New("error 3"))
		subject.InjectError(testingclient.GetVerb, testingclient.AnyKind, pod1Key, errors.New("error 4"))
		subject.InjectError(testingclient.AnyVerb, &corev1.Pod{}, testingclient.AnyObject, errors.New("error 5"))
		subject.InjectError(testingclient.GetVerb, testingclient.AnyKind, testingclient.AnyObject, errors.New("error 6"))
		subject.InjectError(testingclient.AnyVerb, testingclient.AnyKind, pod1Key, errors.New("error 7"))
		subject.InjectError(testingclient.AnyVerb, testingclient.AnyKind, testingclient.AnyObject, errors.New("error 8"))

		var p corev1.Pod
		var c corev1.ConfigMap
		Expect(subject.Get(nil, pod2Key, &p)).To(MatchError("error 2"))
		Expect(subject.Get(nil, pod1Key, &p)).To(MatchError("error 1"))
		Expect(subject.Delete(nil, &pod1)).To(MatchError("error 3"))
		Expect(subject.Get(nil, pod1Key, &c)).To(MatchError("error 4"))
		Expect(subject.Delete(nil, &pod2)).To(MatchError("error 5"))
		Expect(subject.Get(nil, pod2Key, &c)).To(MatchError("error 6"))
		c.SetNamespace(pod1Key.Namespace)
		c.SetName(pod1Key.Name)
		Expect(subject.Delete(nil, &c)).To(MatchError("error 7"))
		c.SetNamespace(pod2Key.Namespace)
		c.SetName(pod2Key.Name)
		Expect(subject.Delete(nil, &c)).To(MatchError("error 8"))
	})

	Describe("preferences between wildcard injections", func() {
		type injection struct {
			verb          testingclient.Verb
			kind          client.Object
			objectKey     client.ObjectKey
			injectedError error
		}
		ItPrefers := func(desc string, preferred, demoted injection) {
			It("prefers "+desc, func() {
				pod1Key := types.NamespacedName{Namespace: "ns", Name: "pod1"}
				var pod1 corev1.Pod
				Expect(subject.Get(nil, pod1Key, &pod1)).To(Succeed())

				subject.InjectError(demoted.verb, demoted.kind, demoted.objectKey, demoted.injectedError)

				var p corev1.Pod
				Expect(subject.Get(nil, pod1Key, &p)).To(Equal(demoted.injectedError))

				subject.InjectError(preferred.verb, preferred.kind, preferred.objectKey, preferred.injectedError)

				Expect(subject.Get(nil, pod1Key, &p)).To(Equal(preferred.injectedError))
			})
		}

		pod1Key := types.NamespacedName{Namespace: "ns", Name: "pod1"}

		ItPrefers("AnyObject(2) matches over AnyVerb(3) matches",
			injection{testingclient.GetVerb, &corev1.Pod{}, testingclient.AnyObject, errors.New("error 2")},
			injection{testingclient.AnyVerb, &corev1.Pod{}, pod1Key, errors.New("error 3")},
		)

		ItPrefers("AnyObject(2) matches over AnyKind(4) matches",
			injection{testingclient.GetVerb, &corev1.Pod{}, testingclient.AnyObject, errors.New("error 2")},
			injection{testingclient.GetVerb, testingclient.AnyKind, pod1Key, errors.New("error 4")},
		)

		ItPrefers("AnyVerb(3) matches over AnyKind(4) matches",
			injection{testingclient.AnyVerb, &corev1.Pod{}, pod1Key, errors.New("error 3")},
			injection{testingclient.GetVerb, testingclient.AnyKind, pod1Key, errors.New("error 4")},
		)

		ItPrefers("AnyKind(4) matches over AnyVerb,AnyObject(5) matches",
			injection{testingclient.GetVerb, testingclient.AnyKind, pod1Key, errors.New("error 4")},
			injection{testingclient.AnyVerb, &corev1.Pod{}, testingclient.AnyObject, errors.New("error 5")},
		)

		ItPrefers("AnyVerb,AnyObject(5) matches over AnyKind,AnyObject(6)",
			injection{testingclient.AnyVerb, &corev1.Pod{}, testingclient.AnyObject, errors.New("error 5")},
			injection{testingclient.GetVerb, testingclient.AnyKind, testingclient.AnyObject, errors.New("error 6")},
		)

		ItPrefers("AnyVerb,AnyObject(5) matches over AnyVerb,AnyKind(7)",
			injection{testingclient.AnyVerb, &corev1.Pod{}, testingclient.AnyObject, errors.New("error 5")},
			injection{testingclient.AnyVerb, testingclient.AnyKind, pod1Key, errors.New("error 7")},
		)

		ItPrefers("AnyKind,AnyObject(6) matches over AnyVerb,AnyKind(7)",
			injection{testingclient.GetVerb, testingclient.AnyKind, testingclient.AnyObject, errors.New("error 6")},
			injection{testingclient.AnyVerb, testingclient.AnyKind, pod1Key, errors.New("error 7")},
		)
	})
})
