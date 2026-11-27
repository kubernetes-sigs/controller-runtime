/*
Copyright 2024 The Kubernetes Authors.

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

package apiutil

import (
	"context"
	"sync/atomic"
	"testing"

	gmg "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/controller-runtime/pkg/internal/legacyrestmapper"
)

func TestLazyRestMapper_fetchGroupVersionResourcesLocked_CacheInvalidation(t *testing.T) {
	tests := []struct {
		name                                   string
		groupName                              string
		versions                               []string
		cachedAPIGroups, expectedAPIGroups     map[string]*metav1.APIGroup
		cachedKnownGroups, expectedKnownGroups map[string]*restmapper.APIGroupResources
	}{
		{
			name:      "Not found version for cached groupVersion in apiGroups and knownGroups",
			groupName: "group1",
			versions:  []string{"v1", "v2"},
			cachedAPIGroups: map[string]*metav1.APIGroup{
				"group1": {
					Name: "group1",
					Versions: []metav1.GroupVersionForDiscovery{
						{
							Version: "v1",
						},
					},
				},
			},
			cachedKnownGroups: map[string]*restmapper.APIGroupResources{
				"group1": {
					VersionedResources: map[string][]metav1.APIResource{
						"v1": {
							{
								Name: "resource1",
							},
						},
					},
				},
			},
			expectedAPIGroups:   map[string]*metav1.APIGroup{},
			expectedKnownGroups: map[string]*restmapper.APIGroupResources{},
		},
		{
			name:      "Not found version for cached groupVersion only in apiGroups",
			groupName: "group1",
			versions:  []string{"v1", "v2"},
			cachedAPIGroups: map[string]*metav1.APIGroup{
				"group1": {
					Name: "group1",
					Versions: []metav1.GroupVersionForDiscovery{
						{
							Version: "v1",
						},
					},
				},
			},
			cachedKnownGroups: map[string]*restmapper.APIGroupResources{
				"group1": {
					VersionedResources: map[string][]metav1.APIResource{
						"v3": {
							{
								Name: "resource1",
							},
						},
					},
				},
			},
			expectedAPIGroups: map[string]*metav1.APIGroup{},
			expectedKnownGroups: map[string]*restmapper.APIGroupResources{
				"group1": {
					VersionedResources: map[string][]metav1.APIResource{
						"v3": {
							{
								Name: "resource1",
							},
						},
					},
				},
			},
		},
		{
			name:      "Not found version for cached groupVersion only in knownGroups",
			groupName: "group1",
			versions:  []string{"v1", "v2"},
			cachedAPIGroups: map[string]*metav1.APIGroup{
				"group1": {
					Name: "group1",
					Versions: []metav1.GroupVersionForDiscovery{
						{
							Version: "v3",
						},
					},
				},
			},
			cachedKnownGroups: map[string]*restmapper.APIGroupResources{
				"group1": {
					VersionedResources: map[string][]metav1.APIResource{
						"v2": {
							{
								Name: "resource1",
							},
						},
					},
				},
			},
			expectedAPIGroups: map[string]*metav1.APIGroup{
				"group1": {
					Name: "group1",
					Versions: []metav1.GroupVersionForDiscovery{
						{
							Version: "v3",
						},
					},
				},
			},
			expectedKnownGroups: map[string]*restmapper.APIGroupResources{},
		},
		{
			name:      "Not found version for non cached groupVersion",
			groupName: "group1",
			versions:  []string{"v1", "v2"},
			cachedAPIGroups: map[string]*metav1.APIGroup{
				"group1": {
					Name: "group1",
					Versions: []metav1.GroupVersionForDiscovery{
						{
							Version: "v3",
						},
					},
				},
			},
			cachedKnownGroups: map[string]*restmapper.APIGroupResources{
				"group1": {
					VersionedResources: map[string][]metav1.APIResource{
						"v3": {
							{
								Name: "resource1",
							},
						},
					},
				},
			},
			expectedAPIGroups: map[string]*metav1.APIGroup{
				"group1": {
					Name: "group1",
					Versions: []metav1.GroupVersionForDiscovery{
						{
							Version: "v3",
						},
					},
				},
			},
			expectedKnownGroups: map[string]*restmapper.APIGroupResources{
				"group1": {
					VersionedResources: map[string][]metav1.APIResource{
						"v3": {
							{
								Name: "resource1",
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gmg.NewWithT(t)
			m := &mapper{
				mapper:      restmapper.NewDiscoveryRESTMapperWithContext([]*restmapper.APIGroupResources{}),
				client:      &fakeAggregatedDiscoveryClient{DiscoveryInterfaceWithContext: discovery.ToDiscoveryInterfaceWithContext(fake.NewClientset().Discovery())},
				apiGroups:   tt.cachedAPIGroups,
				knownGroups: tt.cachedKnownGroups,
			}
			_, err := m.fetchGroupVersionResourcesLocked(t.Context(), tt.groupName, tt.versions...)
			g.Expect(err).NotTo(gmg.HaveOccurred())
			g.Expect(m.apiGroups).To(gmg.BeComparableTo(tt.expectedAPIGroups))
			g.Expect(m.knownGroups).To(gmg.BeComparableTo(tt.expectedKnownGroups))
		})
	}
}

type fakeAggregatedDiscoveryClient struct {
	discovery.DiscoveryInterfaceWithContext
}

func (f *fakeAggregatedDiscoveryClient) GroupsAndMaybeResourcesWithContext(ctx context.Context) (*metav1.APIGroupList, map[schema.GroupVersion]*metav1.APIResourceList, map[schema.GroupVersion]error, error) {
	groupList, err := f.DiscoveryInterfaceWithContext.ServerGroupsWithContext(ctx)
	return groupList, nil, nil, err
}

type scopeToggleDiscoveryClient struct {
	discovery.DiscoveryInterfaceWithContext
	widgetNamespaced atomic.Bool
	groupCalls       atomic.Int32
	resourceCalls    atomic.Int32
}

func (s *scopeToggleDiscoveryClient) GroupsAndMaybeResourcesWithContext(context.Context) (*metav1.APIGroupList, map[schema.GroupVersion]*metav1.APIResourceList, map[schema.GroupVersion]error, error) {
	s.groupCalls.Add(1)
	return &metav1.APIGroupList{
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
	}, nil, nil, nil
}

func (s *scopeToggleDiscoveryClient) ServerResourcesForGroupVersionWithContext(_ context.Context, groupVersion string) (*metav1.APIResourceList, error) {
	s.resourceCalls.Add(1)
	switch groupVersion {
	case "example.com/v1":
		return &metav1.APIResourceList{
			GroupVersion: "example.com/v1",
			APIResources: []metav1.APIResource{
				{
					Name:       "widgets",
					Kind:       "Widget",
					Namespaced: s.widgetNamespaced.Load(),
				},
			},
		}, nil
	case "other.example.com/v1":
		return &metav1.APIResourceList{
			GroupVersion: "other.example.com/v1",
			APIResources: []metav1.APIResource{
				{
					Name:       "gadgets",
					Kind:       "Gadget",
					Namespaced: true,
				},
			},
		}, nil
	default:
		return &metav1.APIResourceList{GroupVersion: groupVersion}, nil
	}
}

func TestDynamicRESTMapper_InvalidateGroupAndReset(t *testing.T) {
	g := gmg.NewWithT(t)

	dc := &scopeToggleDiscoveryClient{}
	rawMapper := &mapper{
		mapper:      restmapper.NewDiscoveryRESTMapperWithContext([]*restmapper.APIGroupResources{}),
		client:      dc,
		knownGroups: map[string]*restmapper.APIGroupResources{},
		apiGroups:   map[string]*metav1.APIGroup{},
	}
	wrapped := legacyrestmapper.Wrap(rawMapper)

	// Verify wrapped mapper satisfies ResettableRESTMapper, ResettableRESTMapperWithContext,
	// GroupInvalidator, and GroupGeneration.
	resettable, ok := wrapped.(meta.ResettableRESTMapper)
	g.Expect(ok).To(gmg.BeTrue())
	resettableCtx, ok := wrapped.(meta.ResettableRESTMapperWithContext)
	g.Expect(ok).To(gmg.BeTrue())
	invalidator, ok := wrapped.(GroupInvalidator)
	g.Expect(ok).To(gmg.BeTrue())
	genMapper, ok := wrapped.(GroupGeneration)
	g.Expect(ok).To(gmg.BeTrue())

	// Initial lookup: Cluster-scoped Widget and Namespaced Gadget.
	mapping, err := wrapped.RESTMappingWithContext(t.Context(), schema.GroupKind{Group: "example.com", Kind: "Widget"}, "v1")
	g.Expect(err).NotTo(gmg.HaveOccurred())
	g.Expect(mapping.Scope.Name()).To(gmg.Equal(meta.RESTScopeNameRoot))

	otherMapping, err := wrapped.RESTMappingWithContext(t.Context(), schema.GroupKind{Group: "other.example.com", Kind: "Gadget"}, "v1")
	g.Expect(err).NotTo(gmg.HaveOccurred())
	g.Expect(otherMapping.Scope.Name()).To(gmg.Equal(meta.RESTScopeNameNamespace))

	callsBeforeCacheHit := dc.resourceCalls.Load()
	exampleGenBeforeCacheHit := genMapper.GroupGeneration("example.com")
	otherGenBeforeCacheHit := genMapper.GroupGeneration("other.example.com")

	// Cached lookup should not trigger additional discovery calls or bump generation.
	mapping, err = wrapped.RESTMappingWithContext(t.Context(), schema.GroupKind{Group: "example.com", Kind: "Widget"}, "v1")
	g.Expect(err).NotTo(gmg.HaveOccurred())
	g.Expect(mapping.Scope.Name()).To(gmg.Equal(meta.RESTScopeNameRoot))
	g.Expect(dc.resourceCalls.Load()).To(gmg.Equal(callsBeforeCacheHit))
	g.Expect(genMapper.GroupGeneration("example.com")).To(gmg.Equal(exampleGenBeforeCacheHit))
	g.Expect(genMapper.GroupGeneration("other.example.com")).To(gmg.Equal(otherGenBeforeCacheHit))

	// Invalidating an uncached group is a no-op.
	invalidator.InvalidateGroup("uncached.example.com")
	g.Expect(genMapper.GroupGeneration("example.com")).To(gmg.Equal(exampleGenBeforeCacheHit))
	g.Expect(genMapper.GroupGeneration("other.example.com")).To(gmg.Equal(otherGenBeforeCacheHit))

	// Recreate CRD with Namespaced scope and invalidate only example.com.
	dc.widgetNamespaced.Store(true)
	invalidator.InvalidateGroup("example.com")
	g.Expect(genMapper.GroupGeneration("example.com")).To(gmg.BeNumerically(">", exampleGenBeforeCacheHit))
	g.Expect(genMapper.GroupGeneration("other.example.com")).To(gmg.Equal(otherGenBeforeCacheHit))

	// other.example.com remains cached and does not trigger discovery.
	callsBeforeOther := dc.resourceCalls.Load()
	otherMapping, err = wrapped.RESTMappingWithContext(t.Context(), schema.GroupKind{Group: "other.example.com", Kind: "Gadget"}, "v1")
	g.Expect(err).NotTo(gmg.HaveOccurred())
	g.Expect(otherMapping.Scope.Name()).To(gmg.Equal(meta.RESTScopeNameNamespace))
	g.Expect(dc.resourceCalls.Load()).To(gmg.Equal(callsBeforeOther))

	// example.com reloads and picks up the new Namespaced scope.
	mapping, err = wrapped.RESTMappingWithContext(t.Context(), schema.GroupKind{Group: "example.com", Kind: "Widget"}, "v1")
	g.Expect(err).NotTo(gmg.HaveOccurred())
	g.Expect(mapping.Scope.Name()).To(gmg.Equal(meta.RESTScopeNameNamespace))
	g.Expect(dc.resourceCalls.Load()).To(gmg.Equal(callsBeforeOther + 1))

	// Recreate CRD back to Cluster scope and reset via ResettableRESTMapperWithContext.
	dc.widgetNamespaced.Store(false)
	genBeforeReset := genMapper.GroupGeneration("example.com")
	otherGenBeforeReset := genMapper.GroupGeneration("other.example.com")
	resettableCtx.ResetWithContext(t.Context())
	g.Expect(genMapper.GroupGeneration("example.com")).To(gmg.BeNumerically(">", genBeforeReset))
	g.Expect(genMapper.GroupGeneration("other.example.com")).To(gmg.BeNumerically(">", otherGenBeforeReset))

	mapping, err = wrapped.RESTMappingWithContext(t.Context(), schema.GroupKind{Group: "example.com", Kind: "Widget"}, "v1")
	g.Expect(err).NotTo(gmg.HaveOccurred())
	g.Expect(mapping.Scope.Name()).To(gmg.Equal(meta.RESTScopeNameRoot))

	// Reset via legacy meta.ResettableRESTMapper also works.
	dc.widgetNamespaced.Store(true)
	genBeforeReset = genMapper.GroupGeneration("example.com")
	resettable.Reset()
	g.Expect(genMapper.GroupGeneration("example.com")).To(gmg.BeNumerically(">", genBeforeReset))

	mapping, err = wrapped.RESTMappingWithContext(t.Context(), schema.GroupKind{Group: "example.com", Kind: "Widget"}, "v1")
	g.Expect(err).NotTo(gmg.HaveOccurred())
	g.Expect(mapping.Scope.Name()).To(gmg.Equal(meta.RESTScopeNameNamespace))
}

func TestDynamicRESTMapper_PerGroupGenerationIsolation(t *testing.T) {
	g := gmg.NewWithT(t)

	dc := &scopeToggleDiscoveryClient{}
	rawMapper := &mapper{
		mapper:           restmapper.NewDiscoveryRESTMapperWithContext([]*restmapper.APIGroupResources{}),
		client:           dc,
		knownGroups:      map[string]*restmapper.APIGroupResources{},
		apiGroups:        map[string]*metav1.APIGroup{},
		groupGenerations: map[string]uint64{},
	}
	wrapped := legacyrestmapper.Wrap(rawMapper)

	genMapper, ok := wrapped.(GroupGeneration)
	g.Expect(ok).To(gmg.BeTrue())

	// Loading example.com bumps example.com only, leaving other.example.com at 0.
	_, err := wrapped.RESTMappingWithContext(t.Context(), schema.GroupKind{Group: "example.com", Kind: "Widget"}, "v1")
	g.Expect(err).NotTo(gmg.HaveOccurred())
	exampleGen := genMapper.GroupGeneration("example.com")
	g.Expect(exampleGen).To(gmg.BeNumerically(">", uint64(0)))
	g.Expect(genMapper.GroupGeneration("other.example.com")).To(gmg.Equal(uint64(0)))

	// Loading other.example.com bumps other.example.com only, leaving example.com unchanged.
	_, err = wrapped.RESTMappingWithContext(t.Context(), schema.GroupKind{Group: "other.example.com", Kind: "Gadget"}, "v1")
	g.Expect(err).NotTo(gmg.HaveOccurred())
	otherGen := genMapper.GroupGeneration("other.example.com")
	g.Expect(otherGen).To(gmg.BeNumerically(">", uint64(0)))
	g.Expect(genMapper.GroupGeneration("example.com")).To(gmg.Equal(exampleGen))

	// Probing a missing kind in example.com reloads discovery for example.com,
	// but since discovery resources did not change, neither group's generation bumps.
	for range 5 {
		_, err = wrapped.RESTMappingWithContext(t.Context(), schema.GroupKind{Group: "example.com", Kind: "MissingKind"}, "v1")
		g.Expect(meta.IsNoMatchError(err)).To(gmg.BeTrue())
		g.Expect(genMapper.GroupGeneration("example.com")).To(gmg.Equal(exampleGen))
		g.Expect(genMapper.GroupGeneration("other.example.com")).To(gmg.Equal(otherGen))
	}
}
