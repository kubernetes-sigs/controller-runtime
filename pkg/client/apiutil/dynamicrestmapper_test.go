/*
Copyright 2021 The Kubernetes Authors.

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

package apiutil_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
	"golang.org/x/time/rate"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

var (
	targetGVK     = schema.GroupVersionKind{Group: "test.kubebuilder.io", Version: "v1beta1", Kind: "SomeCR"}
	targetGVR     = targetGVK.GroupVersion().WithResource("somecrs")
	targetMapping = meta.RESTMapping{Resource: targetGVR, GroupVersionKind: targetGVK, Scope: meta.RESTScopeNamespace}

	secondGVK     = schema.GroupVersionKind{Group: "test.kubebuilder.io", Version: "v1beta1", Kind: "OtherCR"}
	secondGVR     = secondGVK.GroupVersion().WithResource("othercrs")
	secondMapping = meta.RESTMapping{Resource: secondGVR, GroupVersionKind: secondGVK, Scope: meta.RESTScopeNamespace}
)

var _ = Describe("Dynamic REST Mapper", func() {
	var mapper meta.RESTMapper
	var addToMapper func(*meta.DefaultRESTMapper)
	var lim *rate.Limiter

	BeforeEach(func() {
		var err error
		addToMapper = func(baseMapper *meta.DefaultRESTMapper) {
			baseMapper.Add(targetGVK, meta.RESTScopeNamespace)
		}

		lim = rate.NewLimiter(rate.Limit(5), 5)
		mapper, err = apiutil.NewDynamicRESTMapper(cfg, apiutil.WithLimiter(lim), apiutil.WithCustomMapper(func() (meta.RESTMapper, error) {
			baseMapper := meta.NewDefaultRESTMapper(nil)
			addToMapper(baseMapper)

			return baseMapper, nil
		}))
		Expect(err).NotTo(HaveOccurred())
	})

	var mapperTest = func(callWithTarget func() error, callWithOther func() error) {
		It("should read from the cache when possible", func() {
			By("reading successfully once when we expect to succeed")
			Expect(callWithTarget()).To(Succeed())

			By("causing requerying to fail, and trying again")
			addToMapper = func(_ *meta.DefaultRESTMapper) {
				Fail("shouldn't have re-queried")
			}
			Expect(callWithTarget()).To(Succeed())
		})

		It("should reload if not present in the cache", func() {
			By("reading target successfully once")
			Expect(callWithTarget()).To(Succeed())

			By("reading other not successfully")
			count := 0
			addToMapper = func(baseMapper *meta.DefaultRESTMapper) {
				count++
				baseMapper.Add(targetGVK, meta.RESTScopeNamespace)
			}
			Expect(callWithOther()).To(beNoMatchError())
			Expect(count).To(Equal(1), "should reload exactly once")

			By("reading both successfully now")
			addToMapper = func(baseMapper *meta.DefaultRESTMapper) {
				baseMapper.Add(targetGVK, meta.RESTScopeNamespace)
				baseMapper.Add(secondGVK, meta.RESTScopeNamespace)
			}
			Expect(callWithOther()).To(Succeed())
			Expect(callWithTarget()).To(Succeed())
		})

		It("should rate-limit then allow more at configured rate", func() {
			By("setting a small limit")
			*lim = *rate.NewLimiter(rate.Every(100*time.Millisecond), 1)

			By("forcing a reload after changing the mapper")
			addToMapper = func(baseMapper *meta.DefaultRESTMapper) {
				baseMapper.Add(secondGVK, meta.RESTScopeNamespace)
			}
			Expect(callWithOther()).To(Succeed())

			By("calling another time to trigger rate limiting")
			addToMapper = func(baseMapper *meta.DefaultRESTMapper) {
				baseMapper.Add(targetGVK, meta.RESTScopeNamespace)
			}
			// if call consistently fails, we are sure, that it was rate-limited,
			// otherwise it would have reloaded and succeeded
			Consistently(callWithTarget, "90ms", "10ms").Should(beNoMatchError())

			By("calling until no longer rate-limited")
			// once call succeeds, we are sure, that it was no longer rate-limited,
			// as it was allowed to reload and found matching kind/resource
			Eventually(callWithTarget, "30ms", "10ms").Should(And(Succeed(), Not(beNoMatchError())))
		})

		It("should avoid reloading twice if two requests for the same thing come in", func() {
			count := 0
			// we use sleeps here to simulate two simulataneous requests by slowing things down
			addToMapper = func(baseMapper *meta.DefaultRESTMapper) {
				count++
				baseMapper.Add(secondGVK, meta.RESTScopeNamespace)
				time.Sleep(100 * time.Millisecond)
			}

			By("calling two long-running refreshes in parallel and expecting them to succeed")
			done := make(chan struct{})
			go func() {
				defer GinkgoRecover()
				Expect(callWithOther()).To(Succeed())
				close(done)
			}()

			Expect(callWithOther()).To(Succeed())

			// wait till the other goroutine completes to avoid races from a
			// new test writing to mapper, and to make sure we read the right
			// count
			<-done

			By("ensuring that it was only refreshed once")
			Expect(count).To(Equal(1))
		})
	}

	PIt("should lazily initialize if the lazy option is used", func() {

	})

	Describe("KindFor", func() {
		mapperTest(func() error {
			gvk, err := mapper.KindFor(targetGVR)
			if err == nil {
				Expect(gvk).To(Equal(targetGVK))
			}
			return err
		}, func() error {
			gvk, err := mapper.KindFor(secondGVR)
			if err == nil {
				Expect(gvk).To(Equal(secondGVK))
			}
			return err
		})
	})

	Describe("KindsFor", func() {
		mapperTest(func() error {
			gvk, err := mapper.KindsFor(targetGVR)
			if err == nil {
				Expect(gvk).To(Equal([]schema.GroupVersionKind{targetGVK}))
			}
			return err
		}, func() error {
			gvk, err := mapper.KindsFor(secondGVR)
			if err == nil {
				Expect(gvk).To(Equal([]schema.GroupVersionKind{secondGVK}))
			}
			return err
		})
	})

	Describe("ResourceFor", func() {
		mapperTest(func() error {
			gvk, err := mapper.ResourceFor(targetGVR)
			if err == nil {
				Expect(gvk).To(Equal(targetGVR))
			}
			return err
		}, func() error {
			gvk, err := mapper.ResourceFor(secondGVR)
			if err == nil {
				Expect(gvk).To(Equal(secondGVR))
			}
			return err
		})
	})

	Describe("ResourcesFor", func() {
		mapperTest(func() error {
			gvk, err := mapper.ResourcesFor(targetGVR)
			if err == nil {
				Expect(gvk).To(Equal([]schema.GroupVersionResource{targetGVR}))
			}
			return err
		}, func() error {
			gvk, err := mapper.ResourcesFor(secondGVR)
			if err == nil {
				Expect(gvk).To(Equal([]schema.GroupVersionResource{secondGVR}))
			}
			return err
		})
	})

	Describe("RESTMappingFor", func() {
		mapperTest(func() error {
			gvk, err := mapper.RESTMapping(targetGVK.GroupKind(), targetGVK.Version)
			if err == nil {
				Expect(gvk).To(Equal(&targetMapping))
			}
			return err
		}, func() error {
			gvk, err := mapper.RESTMapping(secondGVK.GroupKind(), targetGVK.Version)
			if err == nil {
				Expect(gvk).To(Equal(&secondMapping))
			}
			return err
		})
	})

	Describe("RESTMappingsFor", func() {
		mapperTest(func() error {
			gvk, err := mapper.RESTMappings(targetGVK.GroupKind(), targetGVK.Version)
			if err == nil {
				Expect(gvk).To(Equal([]*meta.RESTMapping{&targetMapping}))
			}
			return err
		}, func() error {
			gvk, err := mapper.RESTMappings(secondGVK.GroupKind(), targetGVK.Version)
			if err == nil {
				Expect(gvk).To(Equal([]*meta.RESTMapping{&secondMapping}))
			}
			return err
		})
	})

	Describe("ResourceSingularizer", func() {
		mapperTest(func() error {
			gvk, err := mapper.ResourceSingularizer(targetGVR.Resource)
			if err == nil {
				Expect(gvk).To(Equal(targetGVR.Resource[:len(targetGVR.Resource)-1]))
			}
			return err
		}, func() error {
			gvk, err := mapper.ResourceSingularizer(secondGVR.Resource)
			if err == nil {
				Expect(gvk).To(Equal(secondGVR.Resource[:len(secondGVR.Resource)-1]))
			}
			return err
		})
	})
})

func beNoMatchError() types.GomegaMatcher {
	return noMatchErrorMatcher{}
}

type noMatchErrorMatcher struct{}

func (k noMatchErrorMatcher) Match(actual interface{}) (success bool, err error) {
	actualErr, actualOk := actual.(error)
	if !actualOk {
		return false, nil
	}

	return meta.IsNoMatchError(actualErr), nil
}

func (k noMatchErrorMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "to be a NoMatchError")
}
func (k noMatchErrorMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "not to be a NoMatchError")
}
