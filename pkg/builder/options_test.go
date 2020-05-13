/*
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

package builder

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ = Describe("options", func() {
	rvcPred := predicate.ResourceVersionChangedPredicate{}
	gcPred := predicate.GenerationChangedPredicate{}

	Describe("Builder", func() {
		It("should have empty predicates when no ForOption supplied", func() {
			bldr := &Builder{}
			bldr = bldr.For(&fakeType{})
			Expect(bldr.forInput).NotTo(BeNil())
			Expect(bldr.forInput.predicates).To(BeNil())
		})
		It("should have one predicate when one ForOption supplied", func() {
			bldr := &Builder{}
			bldr = bldr.For(&fakeType{}, WithPredicates(rvcPred))
			Expect(bldr.forInput).NotTo(BeNil())
			Expect(bldr.forInput.predicates).NotTo(BeNil())
			Expect(bldr.forInput.predicates).To(HaveLen(1))
			Expect(bldr.forInput.predicates).To(ContainElement(rvcPred))
		})
		It("should have two predicates when one multi-value Predicates ForOption supplied", func() {
			bldr := &Builder{}
			bldr = bldr.For(&fakeType{}, WithPredicates(rvcPred, gcPred))
			Expect(bldr.forInput).NotTo(BeNil())
			Expect(bldr.forInput.predicates).NotTo(BeNil())
			Expect(bldr.forInput.predicates).To(HaveLen(2))
			Expect(bldr.forInput.predicates).To(ContainElement(rvcPred))
			Expect(bldr.forInput.predicates).To(ContainElement(gcPred))
		})
		It("should have two predicates when two single-value Predicates ForOption supplied", func() {
			bldr := &Builder{}
			bldr = bldr.For(&fakeType{}, WithPredicates(rvcPred), WithPredicates(gcPred))
			Expect(bldr.forInput).NotTo(BeNil())
			Expect(bldr.forInput.predicates).NotTo(BeNil())
			Expect(bldr.forInput.predicates).To(HaveLen(2))
			Expect(bldr.forInput.predicates).To(ContainElement(rvcPred))
			Expect(bldr.forInput.predicates).To(ContainElement(gcPred))
		})
	})
})
