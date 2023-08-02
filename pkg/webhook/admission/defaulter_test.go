package admission

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ = Describe("Defaulter Handler", func() {

	It("should return ok if received delete verb in defaulter handler", func() {
		obj := &TestDefaulter{}
		handler := DefaultingWebhookFor(admissionScheme, obj)

		resp := handler.Handle(context.TODO(), Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Delete,
				OldObject: runtime.RawExtension{
					Raw: []byte("{}"),
				},
			},
		})
		Expect(resp.Allowed).Should(BeTrue())
		Expect(resp.Result.Code).Should(Equal(int32(http.StatusOK)))
	})

})

// TestDefaulter.
var _ runtime.Object = &TestDefaulter{}

type TestDefaulter struct {
	Replica int `json:"replica,omitempty"`
}

var testDefaulterGVK = schema.GroupVersionKind{Group: "foo.test.org", Version: "v1", Kind: "TestDefaulter"}

func (d *TestDefaulter) GetObjectKind() schema.ObjectKind { return d }
func (d *TestDefaulter) DeepCopyObject() runtime.Object {
	return &TestDefaulter{
		Replica: d.Replica,
	}
}

func (d *TestDefaulter) GroupVersionKind() schema.GroupVersionKind {
	return testDefaulterGVK
}

func (d *TestDefaulter) SetGroupVersionKind(gvk schema.GroupVersionKind) {}

var _ runtime.Object = &TestDefaulterList{}

type TestDefaulterList struct{}

func (*TestDefaulterList) GetObjectKind() schema.ObjectKind { return nil }
func (*TestDefaulterList) DeepCopyObject() runtime.Object   { return nil }

func (d *TestDefaulter) Default() {
	if d.Replica < 2 {
		d.Replica = 2
	}
}

/*
*
BEFORE (without isByteArrayEqual)
goos: darwin
goarch: arm64
pkg: sigs.k8s.io/controller-runtime/pkg/webhook/admission
BenchmarkHandle
BenchmarkHandle/benchmark_handle_function
BenchmarkHandle/benchmark_handle_function-10         	   22878	     50394 ns/op

AFTER (with isByteArrayEqual)
goos: darwin
goarch: arm64
pkg: sigs.k8s.io/controller-runtime/pkg/webhook/admission
BenchmarkHandle
BenchmarkHandle/benchmark_handle_function
BenchmarkHandle/benchmark_handle_function-10         	   44620	     26178 ns/op
*/
func BenchmarkHandle(b *testing.B) {
	pod := createPodForBenchmark()

	byteArray, err := json.Marshal(pod)
	if err != nil {
		fmt.Println("error marshalling pod")
	}

	pa := &podAnnotator{}
	handler := WithCustomDefaulter(admissionScheme, pod, pa)

	b.Run("benchmark handle function", func(b *testing.B) {

		b.ResetTimer()
		for i := 0; i < b.N; i++ {

			// handler.Handle invokes PatchResponseFromRaw function, which is the func to be benchmarked.
			handler.Handle(context.TODO(), Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					Object: runtime.RawExtension{
						Raw: byteArray,
					},
				},
			})
		}
	})
}

// podAnnotator does nothing to a pod.
// it tries to imitate the behavior that pod is unchanged after going through mutating webhook.
type podAnnotator struct{}

func (a *podAnnotator) Default(ctx context.Context, obj runtime.Object) error {

	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return fmt.Errorf("expected a Pod but got a %T", obj)
	}

	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}

	return nil
}

// Create a dummy pod for benchmarking patch
func createPodForBenchmark() *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod-name",
			Labels: map[string]string{
				"appname0": "app-name",
				"appname1": "app-name",
				"appname2": "app-name",
				"appname3": "app-name",
				"appname4": "app-name",
				"appname5": "app-name",
				"appname6": "app-name",
			},
			Annotations: map[string]string{
				"annoKey":  "annoValue",
				"annoKey1": "annoValue",
				"annoKey2": "annoValue",
				"annoKey3": "annoValue",
				"annoKey4": "annoValue",
				"annoKey5": "annoValue",
				"annoKey6": "annoValue",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:              resource.MustParse("4"),
							corev1.ResourceMemory:           resource.MustParse("8Gi"),
							corev1.ResourceEphemeralStorage: resource.MustParse("25Gi"),
						},
					},
				},
				{
					Name: "sidecar",
					Env:  []corev1.EnvVar{{Name: "sampleEnv", Value: "true"}},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("100m"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("4"),
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
					},
				},
			},
		},
	}
	return pod
}
