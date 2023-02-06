/*
Copyright 2023 The Kubernetes Authors.

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
	"net/http"
	"testing"

	_ "github.com/onsi/ginkgo/v2"
	gmg "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// countingRoundTripper is used to count HTTP requests.
type countingRoundTripper struct {
	roundTripper http.RoundTripper
	requestCount int
}

func newCountingRoundTripper(rt http.RoundTripper) *countingRoundTripper {
	return &countingRoundTripper{roundTripper: rt}
}

// RoundTrip implements http.RoundTripper.RoundTrip that additionally counts requests.
func (crt *countingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	crt.requestCount++

	return crt.roundTripper.RoundTrip(r)
}

// GetRequestCount returns how many requests have been made.
func (crt *countingRoundTripper) GetRequestCount() int {
	return crt.requestCount
}

// Reset sets the counter to 0.
func (crt *countingRoundTripper) Reset() {
	crt.requestCount = 0
}

func setupEnvtest(t *testing.T) (*rest.Config, func(t *testing.T)) {
	t.Log("Setup envtest")

	g := gmg.NewWithT(t)
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{"testdata"},
	}

	cfg, err := testEnv.Start()
	g.Expect(err).NotTo(gmg.HaveOccurred())
	g.Expect(cfg).NotTo(gmg.BeNil())

	teardownFunc := func(t *testing.T) {
		t.Log("Stop envtest")
		g.Expect(testEnv.Stop()).To(gmg.Succeed())
	}

	return cfg, teardownFunc
}

func TestLazyRestMapperProvider(t *testing.T) {
	restCfg, tearDownFn := setupEnvtest(t)
	defer tearDownFn(t)

	t.Run("LazyRESTMapper should fetch data based on the request", func(t *testing.T) {
		g := gmg.NewWithT(t)

		// To initialize mapper does 2 requests:
		// GET https://host/api
		// GET https://host/apis
		// Then, for each new group it performs just one request to the API server:
		// GET https://host/apis/<group>/<version>

		var crt *countingRoundTripper
		restCfg := rest.CopyConfig(restCfg)
		restCfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
			crt = newCountingRoundTripper(rt)
			return crt
		}

		lazyRestMapper, err := apiutil.NewDynamicRESTMapper(restCfg, apiutil.WithExperimentalLazyMapper)
		g.Expect(err).NotTo(gmg.HaveOccurred())

		// There are no requests before any call
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(0))

		mapping, err := lazyRestMapper.RESTMapping(schema.GroupKind{Group: "apps", Kind: "deployment"})
		g.Expect(err).NotTo(gmg.HaveOccurred())
		g.Expect(mapping.GroupVersionKind.Kind).To(gmg.Equal("deployment"))
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(3))

		mappings, err := lazyRestMapper.RESTMappings(schema.GroupKind{Group: "", Kind: "pod"})
		g.Expect(err).NotTo(gmg.HaveOccurred())
		g.Expect(len(mappings)).To(gmg.Equal(1))
		g.Expect(mappings[0].GroupVersionKind.Kind).To(gmg.Equal("pod"))
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(4))

		kind, err := lazyRestMapper.KindFor(schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"})
		g.Expect(err).NotTo(gmg.HaveOccurred())
		g.Expect(kind.Kind).To(gmg.Equal("Ingress"))
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(5))

		kinds, err := lazyRestMapper.KindsFor(schema.GroupVersionResource{Group: "authentication.k8s.io", Version: "v1", Resource: "tokenreviews"})
		g.Expect(err).NotTo(gmg.HaveOccurred())
		g.Expect(len(kinds)).To(gmg.Equal(1))
		g.Expect(kinds[0].Kind).To(gmg.Equal("TokenReview"))
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(6))

		resource, err := lazyRestMapper.ResourceFor(schema.GroupVersionResource{Group: "scheduling.k8s.io", Version: "v1", Resource: "priorityclasses"})
		g.Expect(err).NotTo(gmg.HaveOccurred())
		g.Expect(resource.Resource).To(gmg.Equal("priorityclasses"))
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(7))

		resources, err := lazyRestMapper.ResourcesFor(schema.GroupVersionResource{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets"})
		g.Expect(err).NotTo(gmg.HaveOccurred())
		g.Expect(len(resources)).To(gmg.Equal(1))
		g.Expect(resources[0].Resource).To(gmg.Equal("poddisruptionbudgets"))
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(8))
	})

	t.Run("LazyRESTMapper should cache fetched data and doesn't perform any additional requests", func(t *testing.T) {
		g := gmg.NewWithT(t)

		var crt *countingRoundTripper
		restCfg := rest.CopyConfig(restCfg)
		restCfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
			crt = newCountingRoundTripper(rt)
			return crt
		}

		lazyRestMapper, err := apiutil.NewDynamicRESTMapper(restCfg, apiutil.WithExperimentalLazyMapper)
		g.Expect(err).NotTo(gmg.HaveOccurred())

		g.Expect(crt.GetRequestCount()).To(gmg.Equal(0))

		mapping, err := lazyRestMapper.RESTMapping(schema.GroupKind{Group: "apps", Kind: "deployment"})
		g.Expect(err).NotTo(gmg.HaveOccurred())
		g.Expect(mapping.GroupVersionKind.Kind).To(gmg.Equal("deployment"))
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(3))

		// Data taken from cache - there are no more additional requests.

		mapping, err = lazyRestMapper.RESTMapping(schema.GroupKind{Group: "apps", Kind: "deployment"})
		g.Expect(err).NotTo(gmg.HaveOccurred())
		g.Expect(mapping.GroupVersionKind.Kind).To(gmg.Equal("deployment"))
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(3))

		kind, err := lazyRestMapper.KindFor((schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployment"}))
		g.Expect(err).NotTo(gmg.HaveOccurred())
		g.Expect(kind.Kind).To(gmg.Equal("Deployment"))
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(3))

		resource, err := lazyRestMapper.ResourceFor((schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployment"}))
		g.Expect(err).NotTo(gmg.HaveOccurred())
		g.Expect(resource.Resource).To(gmg.Equal("deployments"))
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(3))
	})

	t.Run("LazyRESTMapper should work correctly with empty versions list", func(t *testing.T) {
		g := gmg.NewWithT(t)

		var crt *countingRoundTripper
		restCfg := rest.CopyConfig(restCfg)
		restCfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
			crt = newCountingRoundTripper(rt)
			return crt
		}

		lazyRestMapper, err := apiutil.NewDynamicRESTMapper(restCfg, apiutil.WithExperimentalLazyMapper)
		g.Expect(err).NotTo(gmg.HaveOccurred())

		g.Expect(crt.GetRequestCount()).To(gmg.Equal(0))

		// crew.example.com has 2 versions: v1 and v2

		// If no versions were provided by user, we fetch all of them.
		// Here we expect 4 calls.
		// To initialize:
		// 	#1: GET https://host/api
		// 	#2: GET https://host/apis
		// Then, for each version it performs one request to the API server:
		// 	#3: GET https://host/apis/crew.example.com/v1
		//	#4: GET https://host/apis/crew.example.com/v2
		mapping, err := lazyRestMapper.RESTMapping(schema.GroupKind{Group: "crew.example.com", Kind: "driver"})
		g.Expect(err).NotTo(gmg.HaveOccurred())
		g.Expect(mapping.GroupVersionKind.Kind).To(gmg.Equal("driver"))
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(4))

		// All subsequent calls won't send requests to the server.
		mapping, err = lazyRestMapper.RESTMapping(schema.GroupKind{Group: "crew.example.com", Kind: "driver"})
		g.Expect(err).NotTo(gmg.HaveOccurred())
		g.Expect(mapping.GroupVersionKind.Kind).To(gmg.Equal("driver"))
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(4))
	})

	t.Run("LazyRESTMapper should work correctly with multiple API group versions", func(t *testing.T) {
		g := gmg.NewWithT(t)

		var crt *countingRoundTripper
		restCfg := rest.CopyConfig(restCfg)
		restCfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
			crt = newCountingRoundTripper(rt)
			return crt
		}

		lazyRestMapper, err := apiutil.NewDynamicRESTMapper(restCfg, apiutil.WithExperimentalLazyMapper)
		g.Expect(err).NotTo(gmg.HaveOccurred())

		g.Expect(crt.GetRequestCount()).To(gmg.Equal(0))

		// We explicitly ask for 2 versions: v1 and v2.
		// For each version it performs one request to the API server:
		// 	#1: GET https://host/apis/crew.example.com/v1
		//	#2: GET https://host/apis/crew.example.com/v2
		mapping, err := lazyRestMapper.RESTMapping(schema.GroupKind{Group: "crew.example.com", Kind: "driver"}, "v1", "v2")
		g.Expect(err).NotTo(gmg.HaveOccurred())
		g.Expect(mapping.GroupVersionKind.Kind).To(gmg.Equal("driver"))
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(2))

		// All subsequent calls won't send requests to the server as everything is stored in the cache.
		mapping, err = lazyRestMapper.RESTMapping(schema.GroupKind{Group: "crew.example.com", Kind: "driver"}, "v1")
		g.Expect(err).NotTo(gmg.HaveOccurred())
		g.Expect(mapping.GroupVersionKind.Kind).To(gmg.Equal("driver"))
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(2))

		mapping, err = lazyRestMapper.RESTMapping(schema.GroupKind{Group: "crew.example.com", Kind: "driver"})
		g.Expect(err).NotTo(gmg.HaveOccurred())
		g.Expect(mapping.GroupVersionKind.Kind).To(gmg.Equal("driver"))
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(2))
	})

	t.Run("LazyRESTMapper should work correctly with different API group versions", func(t *testing.T) {
		g := gmg.NewWithT(t)

		var crt *countingRoundTripper
		restCfg := rest.CopyConfig(restCfg)
		restCfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
			crt = newCountingRoundTripper(rt)
			return crt
		}

		lazyRestMapper, err := apiutil.NewDynamicRESTMapper(restCfg, apiutil.WithExperimentalLazyMapper)
		g.Expect(err).NotTo(gmg.HaveOccurred())

		g.Expect(crt.GetRequestCount()).To(gmg.Equal(0))

		// Now we want resources for crew.example.com/v1 version only.
		// Here we expect 1 call:
		// #1: GET https://host/apis/crew.example.com/v1
		mapping, err := lazyRestMapper.RESTMapping(schema.GroupKind{Group: "crew.example.com", Kind: "driver"}, "v1")
		g.Expect(err).NotTo(gmg.HaveOccurred())
		g.Expect(mapping.GroupVersionKind.Kind).To(gmg.Equal("driver"))
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(1))

		// Get additional resources from v2.
		// It sends another request:
		// #2: GET https://host/apis/crew.example.com/v2
		mapping, err = lazyRestMapper.RESTMapping(schema.GroupKind{Group: "crew.example.com", Kind: "driver"}, "v2")
		g.Expect(err).NotTo(gmg.HaveOccurred())
		g.Expect(mapping.GroupVersionKind.Kind).To(gmg.Equal("driver"))
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(2))

		// No subsequent calls require additional API requests.
		mapping, err = lazyRestMapper.RESTMapping(schema.GroupKind{Group: "crew.example.com", Kind: "driver"}, "v1")
		g.Expect(err).NotTo(gmg.HaveOccurred())
		g.Expect(mapping.GroupVersionKind.Kind).To(gmg.Equal("driver"))
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(2))

		mapping, err = lazyRestMapper.RESTMapping(schema.GroupKind{Group: "crew.example.com", Kind: "driver"}, "v1", "v2")
		g.Expect(err).NotTo(gmg.HaveOccurred())
		g.Expect(mapping.GroupVersionKind.Kind).To(gmg.Equal("driver"))
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(2))
	})

	t.Run("LazyRESTMapper should return an error if the group doesn't exist", func(t *testing.T) {
		g := gmg.NewWithT(t)

		// After initialization for each invalid group the mapper performs just 1 request to the API server.

		var crt *countingRoundTripper
		restCfg := rest.CopyConfig(restCfg)
		restCfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
			crt = newCountingRoundTripper(rt)
			return crt
		}

		lazyRestMapper, err := apiutil.NewDynamicRESTMapper(restCfg, apiutil.WithExperimentalLazyMapper)
		g.Expect(err).NotTo(gmg.HaveOccurred())

		_, err = lazyRestMapper.RESTMapping(schema.GroupKind{Group: "INVALID1"}, "v1")
		g.Expect(err).To(gmg.HaveOccurred())
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(1))

		_, err = lazyRestMapper.RESTMappings(schema.GroupKind{Group: "INVALID2"}, "v1")
		g.Expect(err).To(gmg.HaveOccurred())
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(2))

		_, err = lazyRestMapper.KindFor(schema.GroupVersionResource{Group: "INVALID3", Version: "v1"})
		g.Expect(err).To(gmg.HaveOccurred())
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(3))

		_, err = lazyRestMapper.KindsFor(schema.GroupVersionResource{Group: "INVALID4", Version: "v1"})
		g.Expect(err).To(gmg.HaveOccurred())
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(4))

		_, err = lazyRestMapper.ResourceFor(schema.GroupVersionResource{Group: "INVALID5", Version: "v1"})
		g.Expect(err).To(gmg.HaveOccurred())
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(5))

		_, err = lazyRestMapper.ResourcesFor(schema.GroupVersionResource{Group: "INVALID6", Version: "v1"})
		g.Expect(err).To(gmg.HaveOccurred())
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(6))
	})

	t.Run("LazyRESTMapper should return an error if a resource doesn't exist", func(t *testing.T) {
		g := gmg.NewWithT(t)

		// After initialization for each invalid resource the mapper performs just 1 request to the API server.
		var crt *countingRoundTripper
		restCfg := rest.CopyConfig(restCfg)
		restCfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
			crt = newCountingRoundTripper(rt)
			return crt
		}

		lazyRestMapper, err := apiutil.NewDynamicRESTMapper(restCfg, apiutil.WithExperimentalLazyMapper)
		g.Expect(err).NotTo(gmg.HaveOccurred())

		_, err = lazyRestMapper.RESTMapping(schema.GroupKind{Group: "apps", Kind: "INVALID"})
		g.Expect(err).To(gmg.HaveOccurred())
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(3))

		_, err = lazyRestMapper.RESTMappings(schema.GroupKind{Group: "", Kind: "INVALID"})
		g.Expect(err).To(gmg.HaveOccurred())
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(4))

		_, err = lazyRestMapper.KindFor(schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "INVALID"})
		g.Expect(err).To(gmg.HaveOccurred())
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(5))

		_, err = lazyRestMapper.KindsFor(schema.GroupVersionResource{Group: "authentication.k8s.io", Version: "v1", Resource: "INVALID"})
		g.Expect(err).To(gmg.HaveOccurred())
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(6))

		_, err = lazyRestMapper.ResourceFor(schema.GroupVersionResource{Group: "scheduling.k8s.io", Version: "v1", Resource: "INVALID"})
		g.Expect(err).To(gmg.HaveOccurred())
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(7))

		_, err = lazyRestMapper.ResourcesFor(schema.GroupVersionResource{Group: "policy", Version: "v1", Resource: "INVALID"})
		g.Expect(err).To(gmg.HaveOccurred())
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(8))
	})

	t.Run("LazyRESTMapper should return an error if the version doesn't exist", func(t *testing.T) {
		g := gmg.NewWithT(t)

		// After initialization, for each invalid resource mapper performs 1 requests to the API server.
		var crt *countingRoundTripper
		restCfg := rest.CopyConfig(restCfg)
		restCfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
			crt = newCountingRoundTripper(rt)
			return crt
		}

		lazyRestMapper, err := apiutil.NewDynamicRESTMapper(restCfg, apiutil.WithExperimentalLazyMapper)
		g.Expect(err).NotTo(gmg.HaveOccurred())

		_, err = lazyRestMapper.RESTMapping(schema.GroupKind{Group: "apps", Kind: "deployment"}, "INVALID")
		g.Expect(err).To(gmg.HaveOccurred())
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(1))

		_, err = lazyRestMapper.RESTMappings(schema.GroupKind{Group: "", Kind: "pod"}, "INVALID")
		g.Expect(err).To(gmg.HaveOccurred())
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(2))

		_, err = lazyRestMapper.KindFor(schema.GroupVersionResource{Group: "networking.k8s.io", Version: "INVALID", Resource: "ingresses"})
		g.Expect(err).To(gmg.HaveOccurred())
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(3))

		_, err = lazyRestMapper.KindsFor(schema.GroupVersionResource{Group: "authentication.k8s.io", Version: "INVALID", Resource: "tokenreviews"})
		g.Expect(err).To(gmg.HaveOccurred())
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(4))

		_, err = lazyRestMapper.ResourceFor(schema.GroupVersionResource{Group: "scheduling.k8s.io", Version: "INVALID", Resource: "priorityclasses"})
		g.Expect(err).To(gmg.HaveOccurred())
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(5))

		_, err = lazyRestMapper.ResourcesFor(schema.GroupVersionResource{Group: "policy", Version: "INVALID", Resource: "poddisruptionbudgets"})
		g.Expect(err).To(gmg.HaveOccurred())
		g.Expect(crt.GetRequestCount()).To(gmg.Equal(6))
	})
}
