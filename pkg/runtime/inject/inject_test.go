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
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
)

var instance *testSource
var uninjectable *failSource
var errInjectFail = fmt.Errorf("injection fails")

var _ = Describe("runtime inject", func() {

	BeforeEach(func() {
		instance = &testSource{}
		uninjectable = &failSource{}
	})

	It("should set dependencies", func() {

		f := func(interface{}) error { return nil }

		By("Validating injecting dependencies")
		res, err := InjectorInto(f, instance)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(true))
		Expect(reflect.ValueOf(f).Pointer()).To(Equal(reflect.ValueOf(instance.GetFunc()).Pointer()))

		By("Returning false if the type does not implement inject.Injector")
		res, err = InjectorInto(f, uninjectable)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(false))
		Expect(uninjectable.GetFunc()).To(BeNil())

		By("Returning an error if dependencies injection fails")
		res, err = InjectorInto(nil, instance)
		Expect(err).To(Equal(errInjectFail))
		Expect(res).To(Equal(true))
	})

})

type testSource struct {
	scheme *runtime.Scheme
	f      Func
}

func (s *testSource) InjectScheme(scheme *runtime.Scheme) error {
	if scheme != nil {
		s.scheme = scheme
		return nil
	}
	return fmt.Errorf("injection fails")
}

func (s *testSource) InjectFunc(f Func) error {
	if f != nil {
		s.f = f
		return nil
	}
	return fmt.Errorf("injection fails")
}

func (s *testSource) GetScheme() *runtime.Scheme {
	return s.scheme
}

func (s *testSource) GetFunc() Func {
	return s.f
}

type failSource struct {
	scheme *runtime.Scheme
	f      Func
}

func (s *failSource) GetScheme() *runtime.Scheme {
	return s.scheme
}

func (s *failSource) GetFunc() Func {
	return s.f
}
