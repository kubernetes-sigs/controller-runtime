/*
Copyright 2022 The Kubernetes Authors.

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

package selector_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/fields"

	. "sigs.k8s.io/controller-runtime/pkg/internal/field/selector"
)

var _ = Describe("RequiresExactMatch function", func() {

	It("Returns false when the selector matches everything", func() {
		_, requiresExactMatch := RequiresExactMatch(fields.Everything())
		Expect(requiresExactMatch).To(BeFalse())
	})

	It("Returns false when the selector matches nothing", func() {
		_, requiresExactMatch := RequiresExactMatch(fields.Nothing())
		Expect(requiresExactMatch).To(BeFalse())
	})

	It("Returns false when the selector has the form key!=val", func() {
		_, requiresExactMatch := RequiresExactMatch(fields.ParseSelectorOrDie("key!=val"))
		Expect(requiresExactMatch).To(BeFalse())
	})

	It("Returns true when the selector has the form key1==val1,key2==val2", func() {
		_, requiresExactMatch := RequiresExactMatch(fields.ParseSelectorOrDie("key1==val1,key2==val2"))
		Expect(requiresExactMatch).To(BeTrue())
	})

	It("Returns true when the selector has the form key==val", func() {
		_, requiresExactMatch := RequiresExactMatch(fields.ParseSelectorOrDie("key==val"))
		Expect(requiresExactMatch).To(BeTrue())
	})

	It("Returns true when the selector has the form key=val", func() {
		_, requiresExactMatch := RequiresExactMatch(fields.ParseSelectorOrDie("key=val"))
		Expect(requiresExactMatch).To(BeTrue())
	})

	It("Returns empty key and value when the selector matches everything", func() {
		requires, _ := RequiresExactMatch(fields.Everything())
		Expect(requires).To(BeNil())
	})

	It("Returns empty key and value when the selector matches nothing", func() {
		requires, _ := RequiresExactMatch(fields.Nothing())
		Expect(requires).To(BeNil())
	})

	It("Returns empty key and value when the selector has the form key!=val", func() {
		requires, _ := RequiresExactMatch(fields.ParseSelectorOrDie("key!=val"))
		Expect(requires).To(BeNil())
	})

	It("Returns key and value when the selector has the form key==val", func() {
		requires, _ := RequiresExactMatch(fields.ParseSelectorOrDie("key==val"))
		Expect(requires).To(HaveKeyWithValue("key", "val"))
	})

	It("Returns key and value when the selector has the form key=val", func() {
		requires, _ := RequiresExactMatch(fields.ParseSelectorOrDie("key=val"))
		Expect(requires).To(HaveKeyWithValue("key", "val"))
	})
})
