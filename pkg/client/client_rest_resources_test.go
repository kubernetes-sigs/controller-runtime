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
	invalidator, ok := mapper.(interface{ InvalidateGroup(string) })
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
