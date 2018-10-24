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
	"github.com/golang/glog"
)

func Example() {
	i := Injector{}

	// Configure the Injector to inject a string
	if err := i.AddDependency("hello world"); err != nil {
		glog.Fatal(err)
	}

	// Configure the Injector to inject an int
	if err := i.AddDependency(int(3)); err != nil {
		glog.Fatal(err)
	}

	// Configure the Injector to inject a pointer to a struct
	if err := i.AddDependency(&Foo{i: 2}); err != nil {
		glog.Fatal(err)
	}

	// Configure the Injector to inject a struct with a dynamic value
	val := 1
	if err := i.AddProvider(func() Foo {
		val = val + 1
		return Foo{i: val}
	}); err != nil {
		glog.Fatal(err)
	}

	r := &InjectInto{}
	if err := i.Inject(r); err != nil {
		glog.Fatal(err)
	}
}

type InjectInto struct {
	foo      Foo
	fooPtr   *Foo
	i        int
	s        string
	injector *Injector
}

func (into *InjectInto) InjectFoo(f Foo) {
	into.foo = f
}

func (into *InjectInto) InjectFooPtr(f *Foo) {
	into.fooPtr = f
}

func (into *InjectInto) InjectInt(i int) {
	into.i = i
}

func (into *InjectInto) InjectString(s string) {
	into.s = s
}

func (into *InjectInto) InjectInjector(i *Injector) {
	into.injector = i
}

type Foo struct {
	i int
}
