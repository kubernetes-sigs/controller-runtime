/*
Copyright 2026 The Kubernetes Authors.

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

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	gmg "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

func TestClientScopeChangeInvalidation(t *testing.T) {
	g := gmg.NewWithT(t)

	var (
		widgetNamespaced atomic.Bool
		discoveryCalls   atomic.Int32
		mu               sync.Mutex
		writePaths       []string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api":
			_ = json.NewEncoder(w).Encode(&metav1.APIVersions{})
		case "/apis":
			_ = json.NewEncoder(w).Encode(&metav1.APIGroupList{
				Groups: []metav1.APIGroup{
					{
						Name: "example.com",
						Versions: []metav1.GroupVersionForDiscovery{
							{GroupVersion: "example.com/v1", Version: "v1"},
						},
						PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "example.com/v1", Version: "v1"},
					},
				},
			})
		case "/apis/example.com/v1":
			discoveryCalls.Add(1)
			_ = json.NewEncoder(w).Encode(&metav1.APIResourceList{
				GroupVersion: "example.com/v1",
				APIResources: []metav1.APIResource{
					{
						Name:       "widgets",
						Kind:       "Widget",
						Namespaced: widgetNamespaced.Load(),
						Verbs:      metav1.Verbs{"get", "list", "create", "update", "delete"},
					},
				},
			})
		case "/apis/example.com/v1/widgets", "/apis/example.com/v1/namespaces/default/widgets":
			mu.Lock()
			writePaths = append(writePaths, r.URL.Path)
			mu.Unlock()

			var obj unstructured.Unstructured
			if err := json.NewDecoder(r.Body).Decode(&obj); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(&obj)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := &rest.Config{Host: srv.URL}
	httpClient, err := rest.HTTPClientFor(cfg)
	g.Expect(err).NotTo(gmg.HaveOccurred())

	mapper, err := apiutil.NewDynamicRESTMapper(cfg, httpClient)
	g.Expect(err).NotTo(gmg.HaveOccurred())

	c, err := New(cfg, Options{
		HTTPClient: httpClient,
		Mapper:     mapper,
		Scheme:     runtime.NewScheme(),
	})
	g.Expect(err).NotTo(gmg.HaveOccurred())

	newWidget := func() *unstructured.Unstructured {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "example.com",
			Version: "v1",
			Kind:    "Widget",
		})
		obj.SetNamespace("default")
		obj.SetName("my-widget")
		return obj
	}

	// 1. Initial Cluster-scoped Create uses the cluster path.
	g.Expect(c.Create(t.Context(), newWidget())).To(gmg.Succeed())
	callsAfterFirst := discoveryCalls.Load()
	g.Expect(callsAfterFirst).To(gmg.Equal(int32(1)))

	// 2. Subsequent Create hits cache without extra discovery.
	g.Expect(c.Create(t.Context(), newWidget())).To(gmg.Succeed())
	g.Expect(discoveryCalls.Load()).To(gmg.Equal(callsAfterFirst))

	// 3. Recreate CRD as Namespaced and invalidate its group.
	widgetNamespaced.Store(true)
	invalidator, ok := mapper.(apiutil.GroupInvalidator)
	g.Expect(ok).To(gmg.BeTrue())
	invalidator.InvalidateGroup("example.com")

	g.Expect(c.Create(t.Context(), newWidget())).To(gmg.Succeed())
	callsAfterNamespaced := discoveryCalls.Load()
	g.Expect(callsAfterNamespaced).To(gmg.Equal(callsAfterFirst + 1))

	// Cached lookup after scope change hits cache without extra discovery.
	g.Expect(c.Create(t.Context(), newWidget())).To(gmg.Succeed())
	g.Expect(discoveryCalls.Load()).To(gmg.Equal(callsAfterNamespaced))

	// 4. Recreate CRD back to Cluster scope and reset via meta.ResettableRESTMapper.
	widgetNamespaced.Store(false)
	resettable, ok := mapper.(meta.ResettableRESTMapper)
	g.Expect(ok).To(gmg.BeTrue())
	resettable.Reset()

	g.Expect(c.Create(t.Context(), newWidget())).To(gmg.Succeed())
	g.Expect(discoveryCalls.Load()).To(gmg.Equal(callsAfterNamespaced + 1))

	mu.Lock()
	defer mu.Unlock()
	g.Expect(writePaths).To(gmg.Equal([]string{
		"/apis/example.com/v1/widgets",
		"/apis/example.com/v1/widgets",
		"/apis/example.com/v1/namespaces/default/widgets",
		"/apis/example.com/v1/namespaces/default/widgets",
		"/apis/example.com/v1/widgets",
	}))
}

type invalidatingDuringLookupMapper struct {
	apiutil.RESTMapper
	invalidateOnce sync.Once
	onFirstLookup  func()
}

func (m *invalidatingDuringLookupMapper) RESTMappingWithContext(ctx context.Context, gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	mapping, err := m.RESTMapper.RESTMappingWithContext(ctx, gk, versions...)
	if err == nil && gk.Group == "example.com" {
		m.invalidateOnce.Do(func() {
			if m.onFirstLookup != nil {
				m.onFirstLookup()
			}
		})
	}
	return mapping, err
}

func (m *invalidatingDuringLookupMapper) InvalidateGroup(group string) {
	if inv, ok := m.RESTMapper.(apiutil.GroupInvalidator); ok {
		inv.InvalidateGroup(group)
	}
}

func (m *invalidatingDuringLookupMapper) GroupGeneration(group string) uint64 {
	if g, ok := m.RESTMapper.(apiutil.GroupGeneration); ok {
		return g.GroupGeneration(group)
	}
	return 0
}

func TestClientScopeChangeConcurrentInvalidation(t *testing.T) {
	g := gmg.NewWithT(t)

	var (
		widgetNamespaced atomic.Bool
		mu               sync.Mutex
		writePaths       []string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api":
			_ = json.NewEncoder(w).Encode(&metav1.APIVersions{})
		case "/apis":
			_ = json.NewEncoder(w).Encode(&metav1.APIGroupList{
				Groups: []metav1.APIGroup{
					{
						Name: "example.com",
						Versions: []metav1.GroupVersionForDiscovery{
							{GroupVersion: "example.com/v1", Version: "v1"},
						},
						PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "example.com/v1", Version: "v1"},
					},
				},
			})
		case "/apis/example.com/v1":
			_ = json.NewEncoder(w).Encode(&metav1.APIResourceList{
				GroupVersion: "example.com/v1",
				APIResources: []metav1.APIResource{
					{
						Name:       "widgets",
						Kind:       "Widget",
						Namespaced: widgetNamespaced.Load(),
						Verbs:      metav1.Verbs{"get", "list", "create", "update", "delete"},
					},
				},
			})
		case "/apis/example.com/v1/widgets", "/apis/example.com/v1/namespaces/default/widgets":
			mu.Lock()
			writePaths = append(writePaths, r.URL.Path)
			mu.Unlock()

			var obj unstructured.Unstructured
			if err := json.NewDecoder(r.Body).Decode(&obj); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(&obj)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := &rest.Config{Host: srv.URL}
	httpClient, err := rest.HTTPClientFor(cfg)
	g.Expect(err).NotTo(gmg.HaveOccurred())

	baseMapper, err := apiutil.NewDynamicRESTMapper(cfg, httpClient)
	g.Expect(err).NotTo(gmg.HaveOccurred())

	wrappedMapper := &invalidatingDuringLookupMapper{
		RESTMapper: baseMapper,
		onFirstLookup: func() {
			widgetNamespaced.Store(true)
			baseMapper.(apiutil.GroupInvalidator).InvalidateGroup("example.com")
		},
	}

	c, err := New(cfg, Options{
		HTTPClient: httpClient,
		Mapper:     wrappedMapper,
		Scheme:     runtime.NewScheme(),
	})
	g.Expect(err).NotTo(gmg.HaveOccurred())

	newWidget := func() *unstructured.Unstructured {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "example.com",
			Version: "v1",
			Kind:    "Widget",
		})
		obj.SetNamespace("default")
		obj.SetName("my-widget")
		return obj
	}

	// First Create gets the old cluster-scoped mapping while InvalidateGroup runs
	// mid-lookup; it must not cache the stale mapping under the new generation.
	g.Expect(c.Create(t.Context(), newWidget())).To(gmg.Succeed())

	// Second Create must detect the generation mismatch and resolve the namespaced path.
	g.Expect(c.Create(t.Context(), newWidget())).To(gmg.Succeed())

	mu.Lock()
	defer mu.Unlock()
	g.Expect(writePaths).To(gmg.Equal([]string{
		"/apis/example.com/v1/widgets",
		"/apis/example.com/v1/namespaces/default/widgets",
	}))
}

func TestClientScopeChangeUnrelatedGroupAndMissingKindIsolation(t *testing.T) {
	g := gmg.NewWithT(t)

	var (
		widgetNamespaced atomic.Bool
		exampleCalls     atomic.Int32
		otherCalls       atomic.Int32
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api":
			_ = json.NewEncoder(w).Encode(&metav1.APIVersions{})
		case "/apis":
			_ = json.NewEncoder(w).Encode(&metav1.APIGroupList{
				Groups: []metav1.APIGroup{
					{
						Name: "example.com",
						Versions: []metav1.GroupVersionForDiscovery{
							{GroupVersion: "example.com/v1", Version: "v1"},
						},
						PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "example.com/v1", Version: "v1"},
					},
					{
						Name: "other.example.com",
						Versions: []metav1.GroupVersionForDiscovery{
							{GroupVersion: "other.example.com/v1", Version: "v1"},
						},
						PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "other.example.com/v1", Version: "v1"},
					},
				},
			})
		case "/apis/example.com/v1":
			exampleCalls.Add(1)
			_ = json.NewEncoder(w).Encode(&metav1.APIResourceList{
				GroupVersion: "example.com/v1",
				APIResources: []metav1.APIResource{
					{
						Name:       "widgets",
						Kind:       "Widget",
						Namespaced: widgetNamespaced.Load(),
						Verbs:      metav1.Verbs{"get", "list", "create", "update", "delete"},
					},
				},
			})
		case "/apis/other.example.com/v1":
			otherCalls.Add(1)
			_ = json.NewEncoder(w).Encode(&metav1.APIResourceList{
				GroupVersion: "other.example.com/v1",
				APIResources: []metav1.APIResource{
					{
						Name:       "gadgets",
						Kind:       "Gadget",
						Namespaced: true,
						Verbs:      metav1.Verbs{"get", "list", "create", "update", "delete"},
					},
				},
			})
		case "/apis/example.com/v1/widgets",
			"/apis/example.com/v1/namespaces/default/widgets",
			"/apis/other.example.com/v1/namespaces/default/gadgets":
			var obj unstructured.Unstructured
			if err := json.NewDecoder(r.Body).Decode(&obj); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(&obj)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := &rest.Config{Host: srv.URL}
	httpClient, err := rest.HTTPClientFor(cfg)
	g.Expect(err).NotTo(gmg.HaveOccurred())

	mapper, err := apiutil.NewDynamicRESTMapper(cfg, httpClient)
	g.Expect(err).NotTo(gmg.HaveOccurred())

	c, err := New(cfg, Options{
		HTTPClient: httpClient,
		Mapper:     mapper,
		Scheme:     runtime.NewScheme(),
	})
	g.Expect(err).NotTo(gmg.HaveOccurred())

	cl, ok := c.(*client)
	g.Expect(ok).To(gmg.BeTrue())

	newObj := func(group, kind string) *unstructured.Unstructured {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   group,
			Version: "v1",
			Kind:    kind,
		})
		obj.SetNamespace("default")
		obj.SetName("my-obj")
		return obj
	}

	// Warm up example.com Widget cache (initial cold-start lookup + generation sync).
	g.Expect(c.Create(t.Context(), newObj("example.com", "Widget"))).To(gmg.Succeed())
	g.Expect(c.Create(t.Context(), newObj("example.com", "Widget"))).To(gmg.Succeed())

	widgetKey := cacheKey{
		gvk:                  schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"},
		forceDisableProtoBuf: true,
	}
	cl.unstructuredClient.resources.mu.RLock()
	initialWidgetMeta := cl.unstructuredClient.resources.resourceByType[widgetKey]
	cl.unstructuredClient.resources.mu.RUnlock()
	g.Expect(initialWidgetMeta).NotTo(gmg.BeNil())

	// Using an unrelated group must not invalidate example.com's cached resourceMeta.
	g.Expect(c.Create(t.Context(), newObj("other.example.com", "Gadget"))).To(gmg.Succeed())
	g.Expect(otherCalls.Load()).To(gmg.Equal(int32(1)))

	g.Expect(c.Create(t.Context(), newObj("example.com", "Widget"))).To(gmg.Succeed())
	cl.unstructuredClient.resources.mu.RLock()
	afterOtherMeta := cl.unstructuredClient.resources.resourceByType[widgetKey]
	cl.unstructuredClient.resources.mu.RUnlock()
	g.Expect(afterOtherMeta).To(gmg.BeIdenticalTo(initialWidgetMeta))

	// Probing a missing kind in example.com must not invalidate example.com's cached resourceMeta.
	for range 3 {
		_, err = mapper.RESTMappingWithContext(t.Context(), schema.GroupKind{Group: "example.com", Kind: "MissingKind"}, "v1")
		g.Expect(meta.IsNoMatchError(err)).To(gmg.BeTrue())
	}
	g.Expect(c.Create(t.Context(), newObj("example.com", "Widget"))).To(gmg.Succeed())
	cl.unstructuredClient.resources.mu.RLock()
	afterProbeMeta := cl.unstructuredClient.resources.resourceByType[widgetKey]
	cl.unstructuredClient.resources.mu.RUnlock()
	g.Expect(afterProbeMeta).To(gmg.BeIdenticalTo(initialWidgetMeta))

	// Invalidating example.com refreshes the mapping while reusing the existing REST client.
	widgetNamespaced.Store(true)
	mapper.(apiutil.GroupInvalidator).InvalidateGroup("example.com")
	g.Expect(c.Create(t.Context(), newObj("example.com", "Widget"))).To(gmg.Succeed())

	cl.unstructuredClient.resources.mu.RLock()
	refreshedMeta := cl.unstructuredClient.resources.resourceByType[widgetKey]
	cl.unstructuredClient.resources.mu.RUnlock()
	g.Expect(refreshedMeta).NotTo(gmg.BeIdenticalTo(initialWidgetMeta))
	g.Expect(refreshedMeta.Interface).To(gmg.BeIdenticalTo(initialWidgetMeta.Interface))
	g.Expect(refreshedMeta.isNamespaced()).To(gmg.BeTrue())
}
