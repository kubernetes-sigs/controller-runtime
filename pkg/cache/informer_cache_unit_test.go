package cache

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/cache/internal"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllertest"
	crscheme "sigs.k8s.io/controller-runtime/pkg/scheme"
)

const (
	itemPointerSliceTypeGroupName = "jakob.fabian"
	itemPointerSliceTypeVersion   = "v1"
)

var _ = Describe("ip.objectTypeForListObject", func() {
	ip := &informerCache{
		InformersMap: &internal.InformersMap{Scheme: scheme.Scheme},
	}

	It("should error on non-list types", func() {
		_, _, err := ip.objectTypeForListObject(&corev1.Pod{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(`non-list type *v1.Pod (kind "/v1, Kind=Pod") passed as output`))
	})

	It("should find the object type for unstructured lists", func() {
		unstructuredList := &unstructured.UnstructuredList{}
		unstructuredList.SetAPIVersion("v1")
		unstructuredList.SetKind("PodList")

		gvk, obj, err := ip.objectTypeForListObject(unstructuredList)
		Expect(err).ToNot(HaveOccurred())
		Expect(gvk.Group).To(Equal(""))
		Expect(gvk.Version).To(Equal("v1"))
		Expect(gvk.Kind).To(Equal("Pod"))
		referenceUnstructured := &unstructured.Unstructured{}
		referenceUnstructured.SetGroupVersionKind(*gvk)
		Expect(obj).To(Equal(referenceUnstructured))

	})

	It("should find the object type of a list with a slice of literals items field", func() {
		gvk, obj, err := ip.objectTypeForListObject(&corev1.PodList{})
		Expect(err).ToNot(HaveOccurred())
		Expect(gvk.Group).To(Equal(""))
		Expect(gvk.Version).To(Equal("v1"))
		Expect(gvk.Kind).To(Equal("Pod"))
		var referencePod *corev1.Pod
		Expect(obj).To(Equal(referencePod))

	})

	It("should find the object type of a list with a slice of pointers items field", func() {
		By("registering the type", func() {
			err := (&crscheme.Builder{
				GroupVersion: schema.GroupVersion{Group: itemPointerSliceTypeGroupName, Version: itemPointerSliceTypeVersion},
			}).
				Register(
					&controllertest.UnconventionalListType{},
					&controllertest.UnconventionalListTypeList{},
				).AddToScheme(scheme.Scheme)
			Expect(err).To(BeNil())
		})

		By("calling objectTypeForListObject", func() {
			gvk, obj, err := ip.objectTypeForListObject(&controllertest.UnconventionalListTypeList{})
			Expect(err).ToNot(HaveOccurred())
			Expect(gvk.Group).To(Equal(itemPointerSliceTypeGroupName))
			Expect(gvk.Version).To(Equal(itemPointerSliceTypeVersion))
			Expect(gvk.Kind).To(Equal("UnconventionalListType"))
			var referenceObject *controllertest.UnconventionalListType
			Expect(obj).To(Equal(referenceObject))
		})
	})
})
