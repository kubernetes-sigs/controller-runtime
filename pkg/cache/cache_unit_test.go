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
	"k8s.io/client-go/tools/cache"
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
		It("has zero length when View.Namespaces specified and inherited are unset", func() {
			Expect(checkError(specified.inheritFrom(inherited)).View.Namespaces).To(HaveLen(0))
		})
		It("is unchanged when only specified is set", func() {
			specified.View.Namespaces = []string{"specified"}
			Expect(checkError(specified.inheritFrom(inherited)).View.Namespaces).To(Equal(specified.View.Namespaces))
		})
		It("is inherited when only inherited is set", func() {
			inherited.View.Namespaces = []string{"inherited"}
			Expect(checkError(specified.inheritFrom(inherited)).View.Namespaces).To(Equal(inherited.View.Namespaces))
		})
		It("in unchanged when both inherited and specified are set", func() {
			specified.View.Namespaces = []string{"specified"}
			inherited.View.Namespaces = []string{"inherited"}
			Expect(checkError(specified.inheritFrom(inherited)).View.Namespaces).To(Equal(specified.View.Namespaces))
		})
	})
	Context("SelectorsByObject", func() {
		It("is unchanged when specified and inherited are unset", func() {
			Expect(checkError(specified.inheritFrom(inherited)).View.ByObject.Selectors).To(BeNil())
		})
		It("is unchanged when only specified is set", func() {
			specified.Scheme = coreScheme
			specified.View.ByObject.Selectors = map[client.Object]ObjectSelector{&corev1.Pod{}: {}}
			Expect(checkError(specified.inheritFrom(inherited)).View.ByObject.Selectors).To(HaveLen(1))
		})
		It("is inherited when only inherited is set", func() {
			inherited.Scheme = coreScheme
			inherited.View.ByObject.Selectors = map[client.Object]ObjectSelector{&corev1.ConfigMap{}: {}}
			Expect(checkError(specified.inheritFrom(inherited)).View.ByObject.Selectors).To(HaveLen(1))
		})
		It("is combined when both inherited and specified are set", func() {
			specified.Scheme = coreScheme
			inherited.Scheme = coreScheme
			specified.View.ByObject.Selectors = map[client.Object]ObjectSelector{&corev1.Pod{}: {}}
			inherited.View.ByObject.Selectors = map[client.Object]ObjectSelector{&corev1.ConfigMap{}: {}}
			Expect(checkError(specified.inheritFrom(inherited)).View.ByObject.Selectors).To(HaveLen(2))
		})
		It("combines selectors if specified and inherited specify selectors for the same object", func() {
			specified.Scheme = coreScheme
			inherited.Scheme = coreScheme
			specified.View.ByObject.Selectors = map[client.Object]ObjectSelector{&corev1.Pod{}: {
				Label: labels.Set{"specified": "true"}.AsSelector(),
				Field: fields.Set{"metadata.name": "specified"}.AsSelector(),
			}}
			inherited.View.ByObject.Selectors = map[client.Object]ObjectSelector{&corev1.Pod{}: {
				Label: labels.Set{"inherited": "true"}.AsSelector(),
				Field: fields.Set{"metadata.namespace": "inherited"}.AsSelector(),
			}}
			combined := checkError(specified.inheritFrom(inherited)).View.ByObject.Selectors
			Expect(combined).To(HaveLen(1))
			var (
				obj      client.Object
				selector ObjectSelector
			)
			for obj, selector = range combined {
			}
			Expect(obj).To(BeAssignableToTypeOf(&corev1.Pod{}))

			Expect(selector.Label.Matches(labels.Set{"specified": "true"})).To(BeFalse())
			Expect(selector.Label.Matches(labels.Set{"inherited": "true"})).To(BeFalse())
			Expect(selector.Label.Matches(labels.Set{"specified": "true", "inherited": "true"})).To(BeTrue())

			Expect(selector.Field.Matches(fields.Set{"metadata.name": "specified", "metadata.namespace": "other"})).To(BeFalse())
			Expect(selector.Field.Matches(fields.Set{"metadata.name": "other", "metadata.namespace": "inherited"})).To(BeFalse())
			Expect(selector.Field.Matches(fields.Set{"metadata.name": "specified", "metadata.namespace": "inherited"})).To(BeTrue())
		})
		It("uses inherited scheme for inherited selectors", func() {
			inherited.Scheme = coreScheme
			inherited.View.ByObject.Selectors = map[client.Object]ObjectSelector{&corev1.ConfigMap{}: {}}
			Expect(checkError(specified.inheritFrom(inherited)).View.ByObject.Selectors).To(HaveLen(1))
		})
		It("does not use specified scheme for inherited selectors", func() {
			inherited.Scheme = runtime.NewScheme()
			specified.Scheme = coreScheme
			inherited.View.ByObject.Selectors = map[client.Object]ObjectSelector{&corev1.ConfigMap{}: {}}
			_, err := specified.inheritFrom(inherited)
			Expect(err).To(WithTransform(runtime.IsNotRegisteredError, BeTrue()))
		})
		It("uses inherited scheme for specified selectors", func() {
			inherited.Scheme = coreScheme
			specified.View.ByObject.Selectors = map[client.Object]ObjectSelector{&corev1.ConfigMap{}: {}}
			Expect(checkError(specified.inheritFrom(inherited)).View.ByObject.Selectors).To(HaveLen(1))
		})
		It("uses specified scheme for specified selectors", func() {
			specified.Scheme = coreScheme
			specified.View.ByObject.Selectors = map[client.Object]ObjectSelector{&corev1.ConfigMap{}: {}}
			Expect(checkError(specified.inheritFrom(inherited)).View.ByObject.Selectors).To(HaveLen(1))
		})
	})
	Context("DefaultSelector", func() {
		It("is unchanged when specified and inherited are unset", func() {
			Expect(specified.View.DefaultSelector).To(Equal(ObjectSelector{}))
			Expect(inherited.View.DefaultSelector).To(Equal(ObjectSelector{}))
			Expect(checkError(specified.inheritFrom(inherited)).View.DefaultSelector).To(Equal(ObjectSelector{}))
		})
		It("is unchanged when only specified is set", func() {
			specified.View.DefaultSelector = ObjectSelector{Label: labels.Set{"specified": "true"}.AsSelector()}
			Expect(checkError(specified.inheritFrom(inherited)).View.DefaultSelector).To(Equal(specified.View.DefaultSelector))
		})
		It("is inherited when only inherited is set", func() {
			inherited.View.DefaultSelector = ObjectSelector{Label: labels.Set{"inherited": "true"}.AsSelector()}
			Expect(checkError(specified.inheritFrom(inherited)).View.DefaultSelector).To(Equal(inherited.View.DefaultSelector))
		})
		It("is combined when both inherited and specified are set", func() {
			specified.View.DefaultSelector = ObjectSelector{
				Label: labels.Set{"specified": "true"}.AsSelector(),
				Field: fields.Set{"metadata.name": "specified"}.AsSelector(),
			}
			inherited.View.DefaultSelector = ObjectSelector{
				Label: labels.Set{"inherited": "true"}.AsSelector(),
				Field: fields.Set{"metadata.namespace": "inherited"}.AsSelector(),
			}
			combined := checkError(specified.inheritFrom(inherited)).View.DefaultSelector
			Expect(combined).NotTo(BeNil())
			Expect(combined.Label.Matches(labels.Set{"specified": "true"})).To(BeFalse())
			Expect(combined.Label.Matches(labels.Set{"inherited": "true"})).To(BeFalse())
			Expect(combined.Label.Matches(labels.Set{"specified": "true", "inherited": "true"})).To(BeTrue())

			Expect(combined.Field.Matches(fields.Set{"metadata.name": "specified", "metadata.namespace": "other"})).To(BeFalse())
			Expect(combined.Field.Matches(fields.Set{"metadata.name": "other", "metadata.namespace": "inherited"})).To(BeFalse())
			Expect(combined.Field.Matches(fields.Set{"metadata.name": "specified", "metadata.namespace": "inherited"})).To(BeTrue())
		})
	})
	Context("UnsafeDisableDeepCopyByObject", func() {
		It("is unchanged when specified and inherited are unset", func() {
			Expect(checkError(specified.inheritFrom(inherited)).View.ByObject.UnsafeDisableDeepCopy).To(BeNil())
		})
		It("is unchanged when only specified is set", func() {
			specified.Scheme = coreScheme
			specified.View.ByObject.UnsafeDisableDeepCopy = map[client.Object]bool{ObjectAll{}: true}
			Expect(checkError(specified.inheritFrom(inherited)).View.ByObject.UnsafeDisableDeepCopy).To(HaveLen(1))
		})
		It("is inherited when only inherited is set", func() {
			inherited.Scheme = coreScheme
			inherited.View.ByObject.UnsafeDisableDeepCopy = map[client.Object]bool{ObjectAll{}: true}
			Expect(checkError(specified.inheritFrom(inherited)).View.ByObject.UnsafeDisableDeepCopy).To(HaveLen(1))
		})
		It("is combined when both inherited and specified are set for different keys", func() {
			specified.Scheme = coreScheme
			inherited.Scheme = coreScheme
			specified.View.ByObject.UnsafeDisableDeepCopy = map[client.Object]bool{&corev1.Pod{}: true}
			inherited.View.ByObject.UnsafeDisableDeepCopy = map[client.Object]bool{&corev1.ConfigMap{}: true}
			Expect(checkError(specified.inheritFrom(inherited)).View.ByObject.UnsafeDisableDeepCopy).To(HaveLen(2))
		})
		It("is true when inherited=false and specified=true for the same key", func() {
			specified.Scheme = coreScheme
			inherited.Scheme = coreScheme
			specified.View.ByObject.UnsafeDisableDeepCopy = map[client.Object]bool{&corev1.Pod{}: true}
			inherited.View.ByObject.UnsafeDisableDeepCopy = map[client.Object]bool{&corev1.Pod{}: false}
			combined := checkError(specified.inheritFrom(inherited)).View.ByObject.UnsafeDisableDeepCopy
			Expect(combined).To(HaveLen(1))

			var (
				obj             client.Object
				disableDeepCopy bool
			)
			for obj, disableDeepCopy = range combined {
			}
			Expect(obj).To(BeAssignableToTypeOf(&corev1.Pod{}))
			Expect(disableDeepCopy).To(BeTrue())
		})
		It("is false when inherited=true and specified=false for the same key", func() {
			specified.Scheme = coreScheme
			inherited.Scheme = coreScheme
			specified.View.ByObject.UnsafeDisableDeepCopy = map[client.Object]bool{&corev1.Pod{}: false}
			inherited.View.ByObject.UnsafeDisableDeepCopy = map[client.Object]bool{&corev1.Pod{}: true}
			combined := checkError(specified.inheritFrom(inherited)).View.ByObject.UnsafeDisableDeepCopy
			Expect(combined).To(HaveLen(1))

			var (
				obj             client.Object
				disableDeepCopy bool
			)
			for obj, disableDeepCopy = range combined {
			}
			Expect(obj).To(BeAssignableToTypeOf(&corev1.Pod{}))
			Expect(disableDeepCopy).To(BeFalse())
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
			Expect(checkError(specified.inheritFrom(inherited)).View.ByObject.Transform).To(BeNil())
		})
		It("is unchanged when only specified is set", func() {
			specified.Scheme = coreScheme
			specified.View.ByObject.Transform = map[client.Object]cache.TransformFunc{&corev1.Pod{}: func(i interface{}) (interface{}, error) {
				ti := i.(transformed)
				ti.podSpecified = true
				return ti, nil
			}}
			combined := checkError(specified.inheritFrom(inherited)).View.ByObject.Transform
			Expect(combined).To(HaveLen(1))
			for obj, fn := range combined {
				Expect(obj).To(BeAssignableToTypeOf(&corev1.Pod{}))
				out, _ := fn(tx)
				Expect(out).To(And(
					BeAssignableToTypeOf(tx),
					WithTransform(func(i transformed) bool { return i.podSpecified }, BeTrue()),
					WithTransform(func(i transformed) bool { return i.podInherited }, BeFalse()),
				))
			}
		})
		It("is inherited when only inherited is set", func() {
			inherited.Scheme = coreScheme
			inherited.View.ByObject.Transform = map[client.Object]cache.TransformFunc{&corev1.Pod{}: func(i interface{}) (interface{}, error) {
				ti := i.(transformed)
				ti.podInherited = true
				return ti, nil
			}}
			combined := checkError(specified.inheritFrom(inherited)).View.ByObject.Transform
			Expect(combined).To(HaveLen(1))
			for obj, fn := range combined {
				Expect(obj).To(BeAssignableToTypeOf(&corev1.Pod{}))
				out, _ := fn(tx)
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
			specified.View.ByObject.Transform = map[client.Object]cache.TransformFunc{&corev1.Pod{}: func(i interface{}) (interface{}, error) {
				ti := i.(transformed)
				ti.podSpecified = true
				return ti, nil
			}}
			inherited.View.ByObject.Transform = map[client.Object]cache.TransformFunc{&corev1.ConfigMap{}: func(i interface{}) (interface{}, error) {
				ti := i.(transformed)
				ti.configmapInherited = true
				return ti, nil
			}}
			combined := checkError(specified.inheritFrom(inherited)).View.ByObject.Transform
			Expect(combined).To(HaveLen(2))
			for obj, fn := range combined {
				out, _ := fn(tx)
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
			specified.View.ByObject.Transform = map[client.Object]cache.TransformFunc{&corev1.Pod{}: func(i interface{}) (interface{}, error) {
				ti := i.(transformed)
				ti.podSpecified = true
				return ti, nil
			}}
			inherited.View.ByObject.Transform = map[client.Object]cache.TransformFunc{&corev1.Pod{}: func(i interface{}) (interface{}, error) {
				ti := i.(transformed)
				ti.podInherited = true
				return ti, nil
			}}
			combined := checkError(specified.inheritFrom(inherited)).View.ByObject.Transform
			Expect(combined).To(HaveLen(1))
			for obj, fn := range combined {
				Expect(obj).To(BeAssignableToTypeOf(&corev1.Pod{}))
				out, _ := fn(tx)
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
			Expect(checkError(specified.inheritFrom(inherited)).View.DefaultTransform).To(BeNil())
		})
		It("is unchanged when only specified is set", func() {
			specified.View.DefaultTransform = func(i interface{}) (interface{}, error) {
				ti := i.(transformed)
				ti.specified = true
				return ti, nil
			}
			combined := checkError(specified.inheritFrom(inherited)).View.DefaultTransform
			out, _ := combined(tx)
			Expect(out).To(And(
				BeAssignableToTypeOf(tx),
				WithTransform(func(i transformed) bool { return i.specified }, BeTrue()),
				WithTransform(func(i transformed) bool { return i.inherited }, BeFalse()),
			))
		})
		It("is inherited when only inherited is set", func() {
			inherited.View.DefaultTransform = func(i interface{}) (interface{}, error) {
				ti := i.(transformed)
				ti.inherited = true
				return ti, nil
			}
			combined := checkError(specified.inheritFrom(inherited)).View.DefaultTransform
			out, _ := combined(tx)
			Expect(out).To(And(
				BeAssignableToTypeOf(tx),
				WithTransform(func(i transformed) bool { return i.specified }, BeFalse()),
				WithTransform(func(i transformed) bool { return i.inherited }, BeTrue()),
			))
		})
		It("is combined when the transform function is defined in both inherited and specified", func() {
			specified.View.DefaultTransform = func(i interface{}) (interface{}, error) {
				ti := i.(transformed)
				ti.specified = true
				return ti, nil
			}
			inherited.View.DefaultTransform = func(i interface{}) (interface{}, error) {
				ti := i.(transformed)
				ti.inherited = true
				return ti, nil
			}
			combined := checkError(specified.inheritFrom(inherited)).View.DefaultTransform
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
