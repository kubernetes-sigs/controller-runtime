/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package inject_test

import (
	"errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/inject"
)

type FakeConcrete struct {
	V float64
}

type fakeTarget struct {
	// NB(directxman12): these are in order so that our "failure" test
	// below checks that we continue on skip
	Interface    fakeInterface `inject:""`
	ConcreteImpl FakeConcrete
	Interface2   fakeInterface `inject:""`
}

func (f *fakeTarget) InjectFakeConcrete(v FakeConcrete) error {
	f.ConcreteImpl = FakeConcrete{V: v.V * 2}
	return nil
}

type fakeTargetBadField struct {
	// NB(directxman12): these are in order so that our "failure" test
	// below checks that we continue on skip
	iface        fakeInterface `inject:""`
	ConcreteImpl FakeConcrete  `inject:""`
}

type fakeTargetErrMethod struct {
	// NB(directxman12): these are in order so that our "failure" test
	// below checks that we continue on skip
	concrete  FakeConcrete
	Interface fakeInterface `inject:""`
}

func (f *fakeTargetErrMethod) InjectFakeConcrete(v FakeConcrete) error {
	return errors.New("why is your injector even erroring anyway?")
}

type wantsContext struct {
	Context inject.Context `inject:""`
}

type nonStructTarget func(FakeConcrete)

func (t nonStructTarget) InjectFakeConcrete(c FakeConcrete) error {
	t(c)
	return nil
}

var _ = Describe("Injection Contexts", func() {
	Context("when injecting indirectly", func() {
		Context("on struct pointers", func() {
			It("should populate based on struct tags and methods", func() {
				By("populating a context with dependencies")
				fakeImpl := &fakeImplTwo{v: 42}
				ctx := inject.ProvideA(inject.Nothing(), fakeImpl)
				ctx = inject.ProvideA(ctx, FakeConcrete{V: 21})

				By("injecting into a target")
				target := &fakeTarget{}
				all, err := inject.Into(ctx, target)

				By("checking that no errors occurred and everything was marked as injected")
				Expect(err).NotTo(HaveOccurred())
				Expect(all).To(BeTrue())

				By("checking that struct tag values were injected")
				Expect(target.Interface).To(Equal(fakeImpl))
				Expect(target.Interface2).To(Equal(fakeImpl))

				By("checking that method values were injected")
				Expect(target.ConcreteImpl).To(Equal(FakeConcrete{V: 42}))
			})

			It("should panic on unsettable fields", func() {
				By("populating a context with dependencies")
				ctx := inject.ProvideA(inject.Nothing(), &fakeImplTwo{v: 42})
				ctx = inject.ProvideA(ctx, FakeConcrete{V: 21})

				By("injecting into a target with a marked private field")
				target := &fakeTargetBadField{}
				Expect(func() { inject.Into(ctx, target) }).To(Panic())
			})
		})

		Context("on non-struct-pointers", func() {
			It("should populate based on methods", func() {
				By("populating a context with dependencies")
				ctx := inject.ProvideA(inject.Nothing(), FakeConcrete{V: 21})

				By("injecting into a target")
				var val FakeConcrete
				target := nonStructTarget(func(c FakeConcrete) { val = c })
				all, err := inject.Into(ctx, target)

				By("checking that no errors occurred and everything was marked as injected")
				Expect(err).NotTo(HaveOccurred())
				Expect(all).To(BeTrue())

				By("checking that method was called")
				Expect(val).To(Equal(FakeConcrete{V: 21}))
			})
		})

		Context("on any type", func() {
			It("should always be able to inject the Context itself", func() {
				By("populating a context with some (unused) dependencies")
				ctx := inject.ProvideA(inject.Nothing(), &fakeImplTwo{v: 42})

				By("injecting into a target that wants a Context")
				target := &wantsContext{}
				all, err := inject.Into(ctx, target)

				By("expecting everything to have been marked injected")
				Expect(err).NotTo(HaveOccurred())
				Expect(all).To(BeTrue())

				By("checking that the context was actually injected")
				Expect(target.Context).To(Equal(ctx))
			})

			It("should report injection method errors", func() {
				By("populating a context with dependencies")
				ctx := inject.ProvideA(inject.Nothing(), &fakeImplTwo{v: 42})
				ctx = inject.ProvideA(ctx, FakeConcrete{V: 21})

				By("injecting into a target with a method that returns an error")
				target := &fakeTargetErrMethod{}
				all, err := inject.Into(ctx, target)

				By("expecting an error to have been reported and not all fields to have been marked injected")
				Expect(err).To(HaveOccurred())
				Expect(all).To(BeFalse())

				By("confirming that our other field was still set")
				Expect(target.Interface).To(Equal(&fakeImplTwo{v: 42}))
			})

			It("should indicate if not all available injection points were used, and populate all available", func() {
				By("populating a context with only some dependencies")
				fakeImpl := &fakeImplTwo{v: 42}
				ctx := inject.ProvideA(inject.Nothing(), fakeImpl)

				By("injecting into a target")
				target := &fakeTarget{}
				all, err := inject.Into(ctx, target)

				By("checking that no errors occurred and our main values were injected")
				Expect(err).NotTo(HaveOccurred())
				Expect(target.Interface).To(Equal(fakeImpl))
				Expect(target.Interface2).To(Equal(fakeImpl))

				By("checking that we marked not all fields as settable")
				Expect(all).To(BeFalse())
			})

		})
	})
})
