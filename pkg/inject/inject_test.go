/*
Copyright 2018 The Kubernetes Authors.

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

package inject

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("manger.Manager", func() {
	var stop chan struct{}

	BeforeEach(func() {
		stop = make(chan struct{})
	})

	AfterEach(func() {
		close(stop)
	})

	Describe("Inject", func() {
		It("should invoke Inject functions", func() {
			i := Injector{}
			p := &Provider{}

			Expect(i.AddDependency("hello world")).To(Succeed())
			Expect(i.AddProvider(func() Foo { return Foo{i: 1} })).To(Succeed())
			Expect(i.AddProvider(func() *Foo { return &Foo{i: 2} })).To(Succeed())
			Expect(i.AddProvider(p.ProvideInt)).To(Succeed())

			r := &InjectInto{}
			Expect(i.Inject(r)).Should(Succeed())

			Expect(r.foo).To(Equal(Foo{i: 1}))
			Expect(r.fooPtr).To(Equal(&Foo{i: 2}))
			Expect(r.injector).To(Equal(&i))
			Expect(r.s).To(Equal("hello world"))
			Expect(r.i).To(Equal(3))

		})

		It("should invoke Fail if deps are missing", func() {
			i := Injector{}
			p := &Provider{}

			Expect(i.AddDependency("hello world")).To(Succeed())
			Expect(i.AddProvider(p.ProvideInt)).To(Succeed())

			r := &InjectIntoFail{}
			Expect(i.Inject(r)).ShouldNot(Succeed())
		})

		It("should Fail if dependencies are not found", func() {
			i := Injector{}
			p := &Provider{}

			Expect(i.AddDependency("hello world")).To(Succeed())
			Expect(i.AddProvider(p.ProvideInt)).To(Succeed())

			r := &InjectIntoFail{}
			Expect(i.Inject(r)).ShouldNot(Succeed())
		})

		It("should ignore functions without the Inject prefix", func() {
			i := Injector{}

			Expect(i.AddDependency("hello world")).To(Succeed())

			r := &InjectIntoMissing{}
			Expect(i.Inject(r)).Should(Succeed())
			Expect(r.s).To(Equal(""))
		})

		It("should ignore Inject functions with too many arguments", func() {
			i := Injector{}

			Expect(i.AddDependency("hello world")).To(Succeed())
			Expect(i.AddDependency(int(1))).To(Succeed())

			r := &InjectIntoMissingTooMany{}
			Expect(i.Inject(r)).Should(Succeed())
			Expect(r.s).To(Equal(""))
		})

		It("should ignore Inject functions with not enough many arguments", func() {
			i := Injector{}

			Expect(i.AddDependency("hello world")).To(Succeed())
			Expect(i.AddDependency(int(1))).To(Succeed())

			r := &InjectIntoMissingNotEnough{}
			Expect(i.Inject(r)).Should(Succeed())
			Expect(r.s).To(Equal(""))
		})
	})
})

type Provider struct{}

func (p *Provider) ProvideInt() int {
	return 3
}

type InjectIntoFail struct {
	foo      Foo
	fooPtr   *Foo
	i        int
	s        string
	injector *Injector
}

func (into *InjectIntoFail) InjectInt(i int) {
	into.i = i
}

func (into *InjectIntoFail) InjectString(s string) {
	into.s = s
}

func (into *InjectIntoFail) InjectFloat(d float32) {
	// Fails
}

type InjectIntoMissing struct {
	s string
}

func (into *InjectIntoMissing) String(s string) {
	into.s = s
}

type InjectIntoMissingTooMany struct {
	s string
}

func (into *InjectIntoMissing) InjectString(s string, i int) {
	into.s = s
}

type InjectIntoMissingNotEnough struct {
	s string
}

func (into *InjectIntoMissingNotEnough) InjectString() {
	into.s = "hello world"
}
