package komega

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ = Describe("HaveJSONPathMatcher", func() {
	DescribeTable("Matching on a JSON path",
		func(obj interface{}, matcher types.GomegaMatcher, expectedErr types.GomegaMatcher, want types.GomegaMatcher) {
			success, err := matcher.Match(obj)
			Expect(err).Should(expectedErr)
			Expect(success).Should(want)
		},
		Entry("should error when passed an invalid jsonpath",
			&corev1.Pod{},
			HaveJSONPath("{^}", BeTrue()),
			MatchError("JSON Path '{^}' is invalid: unrecognized character in action: U+005E '^'"),
			BeFalse(),
		),
		Entry("should error when passed a jsonpath to an invalid location",
			&corev1.Pod{},
			HaveJSONPath("{.foo}", BeTrue()),
			MatchError("JSON Path '{.foo}' failed: foo is not found"),
			BeFalse(),
		),
		Entry("should succeed when passed a jsonpath to a valid location and correct matcher",
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
			HaveJSONPath("{.metadata.name}", Equal("foo")),
			Succeed(),
			BeTrue(),
		),
		Entry("should fail when passed a jsonpath to a valid location and incorrect matcher",
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
			HaveJSONPath("{.metadata.name}", Equal("bar")),
			Succeed(),
			BeFalse(),
		),
		Entry("should succeed when passed a jsonpath to a valid location and unstructured",
			func() *unstructured.Unstructured {
				u := &unstructured.Unstructured{}
				u.SetName("foo")
				return u
			}(),
			HaveJSONPath("{.metadata.name}", Equal("foo")),
			Succeed(),
			BeTrue(),
		),
		Entry("should succeed when passed a jsonpath to a struct and correct matcher",
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-pod",
					Namespace: "namespace",
					Labels: map[string]string{
						"hello": "pod",
					},
				},
			},
			HaveJSONPath("{.metadata}", MatchFields(IgnoreExtras, Fields{
				"Name":      Equal("my-pod"),
				"Namespace": Equal("namespace"),
				"Labels":    HaveKeyWithValue("hello", "pod"),
			})),
			Succeed(),
			BeTrue(),
		),
		Entry("should succeed when passed a jsonpath to a condition and correct matcher",
			&corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			HaveJSONPath(`{.status.conditions[?(@.type=="Ready")]}`, MatchFields(IgnoreExtras, Fields{
				"Status": BeEquivalentTo(corev1.ConditionTrue),
			})),
			Succeed(),
			BeTrue(),
		),
		Entry("should succeed when passed a jsonpath to an array and correct matcher",
			&corev1.PodList{
				Items: []corev1.Pod{
					{},
					{},
					{},
				},
			},
			HaveJSONPath("{.items[*].metadata.name}", HaveLen(3)),
			Succeed(),
			BeTrue(),
		),
	)
})

func TestHaveJSONPathMatcher(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Conditions")
}
