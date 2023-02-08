package cache

import (
	"reflect"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("cache.inheritFrom", func() {
	defer GinkgoRecover()

	var (
		inherited  Options
		specified  Options
		gv         schema.GroupVersion
		coreScheme *runtime.Scheme
	)

	BeforeEach(func() {
		inherited = Options{}
		specified = Options{}
		gv = schema.GroupVersion{
			Group:   "example.com",
			Version: "v1alpha1",
		}
		coreScheme = runtime.NewScheme()
		Expect(scheme.AddToScheme(coreScheme)).To(Succeed())
	})

	Context("Scheme", func() {
		It("is nil when specified and inherited are unset", func() {
			Expect(checkError(specified.inheritFrom(inherited)).Scheme).To(BeNil())
		})
		It("is specified when only specified is set", func() {
			specified.Scheme = runtime.NewScheme()
			specified.Scheme.AddKnownTypes(gv, &unstructured.Unstructured{})
			Expect(specified.Scheme.KnownTypes(gv)).To(HaveLen(1))

			Expect(checkError(specified.inheritFrom(inherited)).Scheme.KnownTypes(gv)).To(HaveLen(1))
		})
		It("is inherited when only inherited is set", func() {
			inherited.Scheme = runtime.NewScheme()
			inherited.Scheme.AddKnownTypes(gv, &unstructured.Unstructured{})
			Expect(inherited.Scheme.KnownTypes(gv)).To(HaveLen(1))

			combined := checkError(specified.inheritFrom(inherited))
			Expect(combined.Scheme).NotTo(BeNil())
			Expect(combined.Scheme.KnownTypes(gv)).To(HaveLen(1))
		})
		It("is combined when both inherited and specified are set", func() {
			specified.Scheme = runtime.NewScheme()
			specified.Scheme.AddKnownTypes(gv, &unstructured.Unstructured{})
			Expect(specified.Scheme.AllKnownTypes()).To(HaveLen(1))

			inherited.Scheme = runtime.NewScheme()
			inherited.Scheme.AddKnownTypes(schema.GroupVersion{Group: "example.com", Version: "v1"}, &unstructured.Unstructured{})
			Expect(inherited.Scheme.AllKnownTypes()).To(HaveLen(1))

			Expect(checkError(specified.inheritFrom(inherited)).Scheme.AllKnownTypes()).To(HaveLen(2))
		})
	})
	Context("Mapper", func() {
		It("is nil when specified and inherited are unset", func() {
			Expect(checkError(specified.inheritFrom(inherited)).Mapper).To(BeNil())
		})
		It("is unchanged when only specified is set", func() {
			specified.Mapper = meta.NewDefaultRESTMapper(nil)
			Expect(checkError(specified.inheritFrom(inherited)).Mapper).To(Equal(specified.Mapper))
		})
		It("is inherited when only inherited is set", func() {
			inherited.Mapper = meta.NewDefaultRESTMapper(nil)
			Expect(checkError(specified.inheritFrom(inherited)).Mapper).To(Equal(inherited.Mapper))
		})
		It("is unchanged when both inherited and specified are set", func() {
			specified.Mapper = meta.NewDefaultRESTMapper(nil)
			inherited.Mapper = meta.NewDefaultRESTMapper([]schema.GroupVersion{gv})
			Expect(checkError(specified.inheritFrom(inherited)).Mapper).To(Equal(specified.Mapper))
		})
	})
	Context("Resync", func() {
		It("is nil when specified and inherited are unset", func() {
			Expect(checkError(specified.inheritFrom(inherited)).ResyncEvery).To(BeNil())
		})
		It("is unchanged when only specified is set", func() {
			specified.ResyncEvery = pointer.Duration(time.Second)
			Expect(checkError(specified.inheritFrom(inherited)).ResyncEvery).To(Equal(specified.ResyncEvery))
		})
		It("is inherited when only inherited is set", func() {
			inherited.ResyncEvery = pointer.Duration(time.Second)
			Expect(checkError(specified.inheritFrom(inherited)).ResyncEvery).To(Equal(inherited.ResyncEvery))
		})
		It("is unchanged when both inherited and specified are set", func() {
			specified.ResyncEvery = pointer.Duration(time.Second)
			inherited.ResyncEvery = pointer.Duration(time.Minute)
			Expect(checkError(specified.inheritFrom(inherited)).ResyncEvery).To(Equal(specified.ResyncEvery))
		})
	})
	Context("Namespace", func() {
		It("has zero length when Namespaces specified and inherited are unset", func() {
			Expect(checkError(specified.inheritFrom(inherited)).Namespaces).To(HaveLen(0))
		})
		It("is unchanged when only specified is set", func() {
			specified.Namespaces = []string{"specified"}
			Expect(checkError(specified.inheritFrom(inherited)).Namespaces).To(Equal(specified.Namespaces))
		})
		It("is inherited when only inherited is set", func() {
			inherited.Namespaces = []string{"inherited"}
			Expect(checkError(specified.inheritFrom(inherited)).Namespaces).To(Equal(inherited.Namespaces))
		})
		It("in unchanged when both inherited and specified are set", func() {
			specified.Namespaces = []string{"specified"}
			inherited.Namespaces = []string{"inherited"}
			Expect(checkError(specified.inheritFrom(inherited)).Namespaces).To(Equal(specified.Namespaces))
		})
	})
	Context("SelectorsByObject", func() {
		It("is unchanged when specified and inherited are unset", func() {
			Expect(checkError(specified.inheritFrom(inherited)).ByObject).To(HaveLen(0))
		})
		It("is unchanged when only specified is set", func() {
			specified.Scheme = coreScheme
			specified.ByObject = map[client.Object]ByObject{
				&corev1.Pod{}: {},
			}
			Expect(checkError(specified.inheritFrom(inherited)).ByObject).To(HaveLen(1))
		})
		It("is inherited when only inherited is set", func() {
			inherited.Scheme = coreScheme
			inherited.ByObject = map[client.Object]ByObject{
				&corev1.ConfigMap{}: {},
			}
			Expect(checkError(specified.inheritFrom(inherited)).ByObject).To(HaveLen(1))
		})
		It("is combined when both inherited and specified are set", func() {
			specified.Scheme = coreScheme
			inherited.Scheme = coreScheme
			specified.ByObject = map[client.Object]ByObject{
				&corev1.Pod{}: {},
			}
			inherited.ByObject = map[client.Object]ByObject{
				&corev1.ConfigMap{}: {},
			}
			Expect(checkError(specified.inheritFrom(inherited)).ByObject).To(HaveLen(2))
		})
		It("combines selectors if specified and inherited specify selectors for the same object", func() {
			specified.Scheme = coreScheme
			inherited.Scheme = coreScheme
			specified.ByObject = map[client.Object]ByObject{
				&corev1.Pod{}: {
					Label: labels.Set{"specified": "true"}.AsSelector(),
					Field: fields.Set{"metadata.name": "specified"}.AsSelector(),
				},
			}
			inherited.ByObject = map[client.Object]ByObject{
				&corev1.Pod{}: {
					Label: labels.Set{"inherited": "true"}.AsSelector(),
					Field: fields.Set{"metadata.namespace": "inherited"}.AsSelector(),
				},
			}
			combined := checkError(specified.inheritFrom(inherited)).ByObject
			Expect(combined).To(HaveLen(1))
			var (
				obj      client.Object
				byObject ByObject
			)
			for obj, byObject = range combined {
			}
			Expect(obj).To(BeAssignableToTypeOf(&corev1.Pod{}))

			Expect(byObject.Label.Matches(labels.Set{"specified": "true"})).To(BeFalse())
			Expect(byObject.Label.Matches(labels.Set{"inherited": "true"})).To(BeFalse())
			Expect(byObject.Label.Matches(labels.Set{"specified": "true", "inherited": "true"})).To(BeTrue())

			Expect(byObject.Field.Matches(fields.Set{"metadata.name": "specified", "metadata.namespace": "other"})).To(BeFalse())
			Expect(byObject.Field.Matches(fields.Set{"metadata.name": "other", "metadata.namespace": "inherited"})).To(BeFalse())
			Expect(byObject.Field.Matches(fields.Set{"metadata.name": "specified", "metadata.namespace": "inherited"})).To(BeTrue())
		})
		It("uses inherited scheme for inherited selectors", func() {
			inherited.Scheme = coreScheme
			inherited.ByObject = map[client.Object]ByObject{
				&corev1.ConfigMap{}: {},
			}
			Expect(checkError(specified.inheritFrom(inherited)).ByObject).To(HaveLen(1))
		})
		It("uses inherited scheme for specified selectors", func() {
			inherited.Scheme = coreScheme
			specified.ByObject = map[client.Object]ByObject{
				&corev1.ConfigMap{}: {},
			}
			Expect(checkError(specified.inheritFrom(inherited)).ByObject).To(HaveLen(1))
		})
		It("uses specified scheme for specified selectors", func() {
			specified.Scheme = coreScheme
			specified.ByObject = map[client.Object]ByObject{
				&corev1.ConfigMap{}: {},
			}
			Expect(checkError(specified.inheritFrom(inherited)).ByObject).To(HaveLen(1))
		})
	})
	Context("DefaultSelector", func() {
		It("is unchanged when specified and inherited are unset", func() {
			Expect(specified.DefaultLabelSelector).To(BeNil())
			Expect(inherited.DefaultLabelSelector).To(BeNil())
			Expect(specified.DefaultFieldSelector).To(BeNil())
			Expect(inherited.DefaultFieldSelector).To(BeNil())
			Expect(checkError(specified.inheritFrom(inherited)).DefaultLabelSelector).To(BeNil())
			Expect(checkError(specified.inheritFrom(inherited)).DefaultFieldSelector).To(BeNil())
		})
		It("is unchanged when only specified is set", func() {
			specified.DefaultLabelSelector = labels.Set{"specified": "true"}.AsSelector()
			specified.DefaultFieldSelector = fields.Set{"specified": "true"}.AsSelector()
			Expect(checkError(specified.inheritFrom(inherited)).DefaultLabelSelector).To(Equal(specified.DefaultLabelSelector))
			Expect(checkError(specified.inheritFrom(inherited)).DefaultFieldSelector).To(Equal(specified.DefaultFieldSelector))
		})
		It("is inherited when only inherited is set", func() {
			inherited.DefaultLabelSelector = labels.Set{"inherited": "true"}.AsSelector()
			inherited.DefaultFieldSelector = fields.Set{"inherited": "true"}.AsSelector()
			Expect(checkError(specified.inheritFrom(inherited)).DefaultLabelSelector).To(Equal(inherited.DefaultLabelSelector))
			Expect(checkError(specified.inheritFrom(inherited)).DefaultFieldSelector).To(Equal(inherited.DefaultFieldSelector))
		})
		It("is combined when both inherited and specified are set", func() {
			specified.DefaultLabelSelector = labels.Set{"specified": "true"}.AsSelector()
			specified.DefaultFieldSelector = fields.Set{"metadata.name": "specified"}.AsSelector()

			inherited.DefaultLabelSelector = labels.Set{"inherited": "true"}.AsSelector()
			inherited.DefaultFieldSelector = fields.Set{"metadata.namespace": "inherited"}.AsSelector()
			{
				combined := checkError(specified.inheritFrom(inherited)).DefaultLabelSelector
				Expect(combined).NotTo(BeNil())
				Expect(combined.Matches(labels.Set{"specified": "true"})).To(BeFalse())
				Expect(combined.Matches(labels.Set{"inherited": "true"})).To(BeFalse())
				Expect(combined.Matches(labels.Set{"specified": "true", "inherited": "true"})).To(BeTrue())
			}

			{
				combined := checkError(specified.inheritFrom(inherited)).DefaultFieldSelector
				Expect(combined.Matches(fields.Set{"metadata.name": "specified", "metadata.namespace": "other"})).To(BeFalse())
				Expect(combined.Matches(fields.Set{"metadata.name": "other", "metadata.namespace": "inherited"})).To(BeFalse())
				Expect(combined.Matches(fields.Set{"metadata.name": "specified", "metadata.namespace": "inherited"})).To(BeTrue())
			}

		})
	})
	Context("UnsafeDisableDeepCopyByObject", func() {
		It("is unchanged when specified and inherited are unset", func() {
			Expect(checkError(specified.inheritFrom(inherited)).ByObject).To(HaveLen(0))
		})
		It("is unchanged when only specified is set", func() {
			specified.Scheme = coreScheme
			specified.UnsafeDisableDeepCopy = pointer.Bool(true)
			Expect(*(checkError(specified.inheritFrom(inherited)).UnsafeDisableDeepCopy)).To(BeTrue())
		})
		It("is inherited when only inherited is set", func() {
			inherited.Scheme = coreScheme
			inherited.UnsafeDisableDeepCopy = pointer.Bool(true)
			Expect(*(checkError(specified.inheritFrom(inherited)).UnsafeDisableDeepCopy)).To(BeTrue())
		})
		It("is combined when both inherited and specified are set for different keys", func() {
			specified.Scheme = coreScheme
			inherited.Scheme = coreScheme
			specified.ByObject = map[client.Object]ByObject{
				&corev1.Pod{}: {
					UnsafeDisableDeepCopy: pointer.Bool(true),
				},
			}
			inherited.ByObject = map[client.Object]ByObject{
				&corev1.ConfigMap{}: {
					UnsafeDisableDeepCopy: pointer.Bool(true),
				},
			}
			Expect(checkError(specified.inheritFrom(inherited)).ByObject).To(HaveLen(2))
		})
		It("is true when inherited=false and specified=true for the same key", func() {
			specified.Scheme = coreScheme
			inherited.Scheme = coreScheme
			specified.ByObject = map[client.Object]ByObject{
				&corev1.Pod{}: {
					UnsafeDisableDeepCopy: pointer.Bool(true),
				},
			}
			inherited.ByObject = map[client.Object]ByObject{
				&corev1.Pod{}: {
					UnsafeDisableDeepCopy: pointer.Bool(false),
				},
			}
			combined := checkError(specified.inheritFrom(inherited)).ByObject
			Expect(combined).To(HaveLen(1))

			var (
				obj      client.Object
				byObject ByObject
			)
			for obj, byObject = range combined {
			}
			Expect(obj).To(BeAssignableToTypeOf(&corev1.Pod{}))
			Expect(byObject.UnsafeDisableDeepCopy).ToNot(BeNil())
			Expect(*byObject.UnsafeDisableDeepCopy).To(BeTrue())
		})
		It("is false when inherited=true and specified=false for the same key", func() {
			specified.Scheme = coreScheme
			inherited.Scheme = coreScheme
			specified.ByObject = map[client.Object]ByObject{
				&corev1.Pod{}: {
					UnsafeDisableDeepCopy: pointer.Bool(false),
				},
			}
			inherited.ByObject = map[client.Object]ByObject{
				&corev1.Pod{}: {
					UnsafeDisableDeepCopy: pointer.Bool(true),
				},
			}
			combined := checkError(specified.inheritFrom(inherited)).ByObject
			Expect(combined).To(HaveLen(1))

			var (
				obj      client.Object
				byObject ByObject
			)
			for obj, byObject = range combined {
			}
			Expect(obj).To(BeAssignableToTypeOf(&corev1.Pod{}))
			Expect(byObject.UnsafeDisableDeepCopy).ToNot(BeNil())
			Expect(*byObject.UnsafeDisableDeepCopy).To(BeFalse())
		})
	})
	Context("TransformByObject", func() {
		type transformed struct {
			podSpecified       bool
			podInherited       bool
			configmapSpecified bool
			configmapInherited bool
		}
		var tx transformed
		BeforeEach(func() {
			tx = transformed{}
		})
		It("is unchanged when specified and inherited are unset", func() {
			Expect(checkError(specified.inheritFrom(inherited)).ByObject).To(HaveLen(0))
		})
		It("is unchanged when only specified is set", func() {
			specified.Scheme = coreScheme
			specified.ByObject = map[client.Object]ByObject{
				&corev1.Pod{}: {
					Transform: func(i interface{}) (interface{}, error) {
						ti := i.(transformed)
						ti.podSpecified = true
						return ti, nil
					},
				},
			}
			combined := checkError(specified.inheritFrom(inherited)).ByObject
			Expect(combined).To(HaveLen(1))
			for obj, byObject := range combined {
				Expect(obj).To(BeAssignableToTypeOf(&corev1.Pod{}))
				out, _ := byObject.Transform(tx)
				Expect(out).To(And(
					BeAssignableToTypeOf(tx),
					WithTransform(func(i transformed) bool { return i.podSpecified }, BeTrue()),
					WithTransform(func(i transformed) bool { return i.podInherited }, BeFalse()),
				))
			}
		})
		It("is inherited when only inherited is set", func() {
			inherited.Scheme = coreScheme
			inherited.ByObject = map[client.Object]ByObject{
				&corev1.Pod{}: {
					Transform: func(i interface{}) (interface{}, error) {
						ti := i.(transformed)
						ti.podInherited = true
						return ti, nil
					},
				},
			}
			combined := checkError(specified.inheritFrom(inherited)).ByObject
			Expect(combined).To(HaveLen(1))
			for obj, byObject := range combined {
				Expect(obj).To(BeAssignableToTypeOf(&corev1.Pod{}))
				out, _ := byObject.Transform(tx)
				Expect(out).To(And(
					BeAssignableToTypeOf(tx),
					WithTransform(func(i transformed) bool { return i.podSpecified }, BeFalse()),
					WithTransform(func(i transformed) bool { return i.podInherited }, BeTrue()),
				))
			}
		})
		It("is combined when both inherited and specified are set for different keys", func() {
			specified.Scheme = coreScheme
			inherited.Scheme = coreScheme
			specified.ByObject = map[client.Object]ByObject{
				&corev1.Pod{}: {
					Transform: func(i interface{}) (interface{}, error) {
						ti := i.(transformed)
						ti.podSpecified = true
						return ti, nil
					},
				},
			}
			inherited.ByObject = map[client.Object]ByObject{
				&corev1.ConfigMap{}: {
					Transform: func(i interface{}) (interface{}, error) {
						ti := i.(transformed)
						ti.configmapInherited = true
						return ti, nil
					},
				},
			}
			combined := checkError(specified.inheritFrom(inherited)).ByObject
			Expect(combined).To(HaveLen(2))
			for obj, byObject := range combined {
				out, _ := byObject.Transform(tx)
				if reflect.TypeOf(obj) == reflect.TypeOf(&corev1.Pod{}) {
					Expect(out).To(And(
						BeAssignableToTypeOf(tx),
						WithTransform(func(i transformed) bool { return i.podSpecified }, BeTrue()),
						WithTransform(func(i transformed) bool { return i.podInherited }, BeFalse()),
						WithTransform(func(i transformed) bool { return i.configmapSpecified }, BeFalse()),
						WithTransform(func(i transformed) bool { return i.configmapInherited }, BeFalse()),
					))
				}
				if reflect.TypeOf(obj) == reflect.TypeOf(&corev1.ConfigMap{}) {
					Expect(out).To(And(
						BeAssignableToTypeOf(tx),
						WithTransform(func(i transformed) bool { return i.podSpecified }, BeFalse()),
						WithTransform(func(i transformed) bool { return i.podInherited }, BeFalse()),
						WithTransform(func(i transformed) bool { return i.configmapSpecified }, BeFalse()),
						WithTransform(func(i transformed) bool { return i.configmapInherited }, BeTrue()),
					))
				}
			}
		})
		It("is combined into a single transform function when both inherited and specified are set for the same key", func() {
			specified.Scheme = coreScheme
			inherited.Scheme = coreScheme
			specified.ByObject = map[client.Object]ByObject{
				&corev1.Pod{}: {
					Transform: func(i interface{}) (interface{}, error) {
						ti := i.(transformed)
						ti.podSpecified = true
						return ti, nil
					},
				},
			}
			inherited.ByObject = map[client.Object]ByObject{
				&corev1.Pod{}: {
					Transform: func(i interface{}) (interface{}, error) {
						ti := i.(transformed)
						ti.podInherited = true
						return ti, nil
					},
				},
			}
			combined := checkError(specified.inheritFrom(inherited)).ByObject
			Expect(combined).To(HaveLen(1))
			for obj, byObject := range combined {
				Expect(obj).To(BeAssignableToTypeOf(&corev1.Pod{}))
				out, _ := byObject.Transform(tx)
				Expect(out).To(And(
					BeAssignableToTypeOf(tx),
					WithTransform(func(i transformed) bool { return i.podSpecified }, BeTrue()),
					WithTransform(func(i transformed) bool { return i.podInherited }, BeTrue()),
					WithTransform(func(i transformed) bool { return i.configmapSpecified }, BeFalse()),
					WithTransform(func(i transformed) bool { return i.configmapInherited }, BeFalse()),
				))
			}
		})
	})
	Context("DefaultTransform", func() {
		type transformed struct {
			specified bool
			inherited bool
		}
		var tx transformed
		BeforeEach(func() {
			tx = transformed{}
		})
		It("is unchanged when specified and inherited are unset", func() {
			Expect(checkError(specified.inheritFrom(inherited)).DefaultTransform).To(BeNil())
		})
		It("is unchanged when only specified is set", func() {
			specified.DefaultTransform = func(i interface{}) (interface{}, error) {
				ti := i.(transformed)
				ti.specified = true
				return ti, nil
			}
			combined := checkError(specified.inheritFrom(inherited)).DefaultTransform
			out, _ := combined(tx)
			Expect(out).To(And(
				BeAssignableToTypeOf(tx),
				WithTransform(func(i transformed) bool { return i.specified }, BeTrue()),
				WithTransform(func(i transformed) bool { return i.inherited }, BeFalse()),
			))
		})
		It("is inherited when only inherited is set", func() {
			inherited.DefaultTransform = func(i interface{}) (interface{}, error) {
				ti := i.(transformed)
				ti.inherited = true
				return ti, nil
			}
			combined := checkError(specified.inheritFrom(inherited)).DefaultTransform
			out, _ := combined(tx)
			Expect(out).To(And(
				BeAssignableToTypeOf(tx),
				WithTransform(func(i transformed) bool { return i.specified }, BeFalse()),
				WithTransform(func(i transformed) bool { return i.inherited }, BeTrue()),
			))
		})
		It("is combined when the transform function is defined in both inherited and specified", func() {
			specified.DefaultTransform = func(i interface{}) (interface{}, error) {
				ti := i.(transformed)
				ti.specified = true
				return ti, nil
			}
			inherited.DefaultTransform = func(i interface{}) (interface{}, error) {
				ti := i.(transformed)
				ti.inherited = true
				return ti, nil
			}
			combined := checkError(specified.inheritFrom(inherited)).DefaultTransform
			Expect(combined).NotTo(BeNil())
			out, _ := combined(tx)
			Expect(out).To(And(
				BeAssignableToTypeOf(tx),
				WithTransform(func(i transformed) bool { return i.specified }, BeTrue()),
				WithTransform(func(i transformed) bool { return i.inherited }, BeTrue()),
			))
		})
	})
})

func checkError[T any](v T, err error) T {
	Expect(err).To(BeNil())
	return v
}
