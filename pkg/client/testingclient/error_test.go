package testingclient_test

import (
	"errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
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

	// TODO: test more functions.
	// TODO: test prefers specific matches over general matches.
})
