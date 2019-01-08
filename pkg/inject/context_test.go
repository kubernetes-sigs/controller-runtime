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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/inject"
)

type fakeInterface interface {
	doAThing()
}

type fakeImplOne struct {
	v int
}

func (f fakeImplOne) doAThing() {}

type fakeImplTwo struct {
	v int
}

func (f *fakeImplTwo) doAThing() {}

var _ = Describe("Injection Contexts", func() {
	Context("when populating directly", func() {
		It("should populate a provided type", func() {
			By("providing a type")
			srcObj := fakeImplOne{v: 42}
			ctx := inject.ProvideA(inject.Nothing(), srcObj)

			By("populating a value of that type")
			var obj fakeImplOne
			Expect(ctx.PopulateThis("", &obj)).To(BeTrue(), "should have indicate that a value was populated")

			By("checking that the right value was populated")
			Expect(obj).To(Equal(srcObj))
		})

		It("should panic if not passed a pointer", func() {
			By("providing a type")
			srcObj := fakeImplOne{v: 42}
			ctx := inject.ProvideA(inject.Nothing(), srcObj)

			By("trying to populate a non-pointer value")
			var obj fakeImplOne
			Expect(func() { ctx.PopulateThis("", obj) }).To(Panic())
		})

		It("should populate a provided instance of an interface", func() {
			By("providing a interface")
			srcObj := fakeImplOne{v: 42}
			ctx := inject.ProvideA(inject.Nothing(), srcObj)

			By("populating a value of that type")
			var obj fakeInterface
			ctx.PopulateThis("", &obj)

			By("checking that the right value was populated")
			Expect(obj).To(Equal(srcObj))
		})
		It("should not populate the wrong type", func() {
			By("providing a type")
			srcObj := fakeImplOne{v: 42}
			ctx := inject.ProvideA(inject.Nothing(), srcObj)

			By("populating a value of a different type")
			obj := fakeImplTwo{v: 2}
			Expect(ctx.PopulateThis("", &obj)).To(BeFalse(), "should have indicated that no value was populated")

			By("checking that our obj is not populated")
			Expect(obj).To(Equal(fakeImplTwo{v: 2}))
		})

		It("should populate a provided type after providing another type", func() {
			By("providing a type")
			srcObj := fakeImplOne{v: 42}
			ctx := inject.ProvideA(inject.Nothing(), srcObj)

			By("providing another type")
			ctx = inject.ProvideA(ctx, fakeImplTwo{v: 128})

			By("populating a value of the first type")
			var obj fakeImplOne
			Expect(ctx.PopulateThis("", &obj)).To(BeTrue(), "should have indicate that a value was populated")

			By("checking that the right value was populated")
			Expect(obj).To(Equal(srcObj))
		})

		It("should only populate the most recent value for a type", func() {
			By("providing a type")
			srcObj := fakeImplOne{v: 42}
			ctx := inject.ProvideA(inject.Nothing(), srcObj)

			By("providing another type")
			ctx = inject.ProvideA(ctx, fakeImplTwo{v: 128})

			By("providing a different first type value")
			ctx = inject.ProvideA(ctx, fakeImplOne{v: 256})

			By("populating a value of the first type")
			var obj fakeImplOne
			Expect(ctx.PopulateThis("", &obj)).To(BeTrue(), "should have indicate that a value was populated")

			By("checking that the right value was populated")
			Expect(obj).To(Equal(fakeImplOne{v: 256}))
		})

		It("should support providing multiple dependencies at once", func() {
			By("providing multiple dependencies")
			ctx := inject.ProvideSome(inject.Nothing(), fakeImplOne{v: 42}, &fakeImplTwo{v: 21})

			By("fetching those dependencies")
			var targetOne fakeImplOne
			var targetTwo *fakeImplTwo
			Expect(ctx.PopulateThis("", &targetOne)).To(BeTrue(), "should have indicated that a value was populated")
			Expect(ctx.PopulateThis("", &targetTwo)).To(BeTrue(), "should have indicated that a value was populated")

			By("checking that those values are correct")
			Expect(targetOne).To(Equal(fakeImplOne{v: 42}))
			Expect(targetTwo).To(Equal(&fakeImplTwo{v: 21}))
		})

		It("should support passing dependencies with specific names", func() {
			By("providing a dependency with a specific name")
			stopCh := make(chan struct{})
			ctx := inject.ProvideASpecific(inject.Nothing(), "StopChannel", stopCh)

			By("fetching that dependency without a key and verifying that it fails")
			var otherCh chan struct{}
			Expect(ctx.PopulateThis("", &otherCh)).To(BeFalse())
			Expect(otherCh).To(BeNil())

			By("fetching that dependency with a key and verifying that it succeeds")
			Expect(ctx.PopulateThis("StopChannel", &otherCh)).To(BeTrue())
			Expect(otherCh).To(Equal(stopCh))
		})
	})
})
