package apiutil_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"golang.org/x/time/rate"
	"golang.org/x/xerrors"
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
			By("reading successfully once")
			Expect(callWithTarget()).To(Succeed())
			Expect(callWithOther()).NotTo(Succeed())

			By("asking for a something that didn't exist previously after adding it to the mapper")
			addToMapper = func(baseMapper *meta.DefaultRESTMapper) {
				baseMapper.Add(targetGVK, meta.RESTScopeNamespace)
				baseMapper.Add(secondGVK, meta.RESTScopeNamespace)
			}
			Expect(callWithOther()).To(Succeed())
			Expect(callWithTarget()).To(Succeed())
		})

		It("should rate-limit reloads so that we don't get more than a certain number per second", func() {
			By("setting a small limit")
			*lim = *rate.NewLimiter(rate.Limit(1), 1)

			By("forcing a reload after changing the mapper")
			addToMapper = func(baseMapper *meta.DefaultRESTMapper) {
				baseMapper.Add(secondGVK, meta.RESTScopeNamespace)
			}
			Expect(callWithOther()).To(Succeed())

			By("calling another time that would need a requery and failing")
			Eventually(func() bool {
				return xerrors.As(callWithTarget(), &apiutil.ErrRateLimited{})
			}, "10s").Should(BeTrue())
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
