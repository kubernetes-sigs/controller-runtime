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
	"testing"

	gmg "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/restmapper"
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
				mapper:      restmapper.NewDiscoveryRESTMapper([]*restmapper.APIGroupResources{}),
				client:      &fakeAggregatedDiscoveryClient{DiscoveryInterface: fake.NewClientset().Discovery()},
				apiGroups:   tt.cachedAPIGroups,
				knownGroups: tt.cachedKnownGroups,
			}
			_, err := m.fetchGroupVersionResourcesLocked(tt.groupName, tt.versions...)
			g.Expect(err).NotTo(gmg.HaveOccurred())
			g.Expect(m.apiGroups).To(gmg.BeComparableTo(tt.expectedAPIGroups))
			g.Expect(m.knownGroups).To(gmg.BeComparableTo(tt.expectedKnownGroups))
		})
	}
}

type fakeAggregatedDiscoveryClient struct {
	discovery.DiscoveryInterface
}

func (f *fakeAggregatedDiscoveryClient) GroupsAndMaybeResources() (*metav1.APIGroupList, map[schema.GroupVersion]*metav1.APIResourceList, map[schema.GroupVersion]error, error) {
	groupList, err := f.DiscoveryInterface.ServerGroups()
	return groupList, nil, nil, err
}

// Test the cache invalidation when mapping is requested without specifying a version
func TestLazyRestMapper_AggregatedDiscovery_CacheInvalidation(t *testing.T) {
	tests := []struct {
		name                string
		initialAPIGroup     *metav1.APIGroup
		initialResourceList map[schema.GroupVersion]*metav1.APIResourceList
		updatedAPIGroup     *metav1.APIGroup
		updatedResourceList map[schema.GroupVersion]*metav1.APIResourceList
		expectedKind        schema.GroupKind
	}{
		{
			name: "new resource added to an existing group version",
			initialAPIGroup: &metav1.APIGroup{
				Name: "testGroup",
				Versions: []metav1.GroupVersionForDiscovery{
					{GroupVersion: "testGroup/v1", Version: "v1"},
				},
			},
			initialResourceList: map[schema.GroupVersion]*metav1.APIResourceList{
				{Group: "testGroup", Version: "v1"}: {
					GroupVersion: "testGroup/v1",
					APIResources: []metav1.APIResource{
						{Name: "firstResources", Kind: "FirstResources", Version: "v1"},
					},
				},
			},
			updatedAPIGroup: &metav1.APIGroup{
				Name: "testGroup",
				Versions: []metav1.GroupVersionForDiscovery{
					{GroupVersion: "testGroup/v1", Version: "v1"},
				},
			},
			updatedResourceList: map[schema.GroupVersion]*metav1.APIResourceList{
				{Group: "testGroup", Version: "v1"}: {
					GroupVersion: "testGroup/v1",
					APIResources: []metav1.APIResource{
						{Name: "firstResources", Kind: "FirstResources", Version: "v1"},
						{Name: "secondResources", Kind: "SecondResources", Version: "v1"},
					},
				},
			},
			expectedKind:   schema.GroupKind{Group: "testGroup", Kind: "SecondResources"},
		},
		{
			name: "new version added to existing group",
			initialAPIGroup: &metav1.APIGroup{
				Name: "testGroup",
				Versions: []metav1.GroupVersionForDiscovery{
					{GroupVersion: "testGroup/v1", Version: "v1"},
				},
			},
			initialResourceList: map[schema.GroupVersion]*metav1.APIResourceList{
				{Group: "testGroup", Version: "v1"}: {
					GroupVersion: "testGroup/v1",
					APIResources: []metav1.APIResource{
						{Name: "v1OnlyResources", Kind: "V1OnlyResource", Version: "v1"},
					},
				},
			},
			updatedAPIGroup: &metav1.APIGroup{
				Name: "testGroup",
				Versions: []metav1.GroupVersionForDiscovery{
					{GroupVersion: "testGroup/v1", Version: "v1"},
					{GroupVersion: "testGroup/v2", Version: "v2"},
				},
			},
			updatedResourceList: map[schema.GroupVersion]*metav1.APIResourceList{
				{Group: "testGroup", Version: "v1"}: {
					GroupVersion: "testGroup/v1",
					APIResources: []metav1.APIResource{
						{Name: "v1OnlyResources", Kind: "V1OnlyResource", Version: "v1"},
					},
				},
				{Group: "testGroup", Version: "v2"}: {
					GroupVersion: "testGroup/v2",
					APIResources: []metav1.APIResource{
						{Name: "v2OnlyResources", Kind: "V2OnlyResource", Version: "v2"},
					},
				},
			},
			expectedKind:   schema.GroupKind{Group: "testGroup", Kind: "V2OnlyResource"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gmg.NewWithT(t)
			m := &mapper{
				mapper: restmapper.NewDiscoveryRESTMapper([]*restmapper.APIGroupResources{}),
				client: &mockAggregatedDiscoveryClient{
					initialGroupList:    &metav1.APIGroupList{Groups: []metav1.APIGroup{*tt.initialAPIGroup}},
					initialResourceList: tt.initialResourceList,
					updatedGroupList:    &metav1.APIGroupList{Groups: []metav1.APIGroup{*tt.updatedAPIGroup}},
					updatedResourceList: tt.updatedResourceList,
				},
				apiGroups:   map[string]*metav1.APIGroup{},
				knownGroups: map[string]*restmapper.APIGroupResources{},
			}

			// First call the mapper should return an error since the resource is not yet served
			_, err := m.RESTMapping(tt.expectedKind)
			g.Expect(err).To(gmg.HaveOccurred(), "expected error while CRD is not yet installed")
			g.Expect(meta.IsNoMatchError(err)).To(gmg.BeTrue(), "expected NoMatchError, got: %v", err)

			// Second call should be successful
			mapping, err := m.RESTMapping(tt.expectedKind)
			g.Expect(err).NotTo(gmg.HaveOccurred(), "expected CRD to be found once it is installed")
			g.Expect(mapping.GroupVersionKind.Kind).To(gmg.Equal(tt.expectedKind.Kind))
		})
	}
}

type mockAggregatedDiscoveryClient struct {
	discovery.DiscoveryInterface
	initialized         bool
	initialGroupList    *metav1.APIGroupList
	initialResourceList map[schema.GroupVersion]*metav1.APIResourceList
	updatedGroupList    *metav1.APIGroupList
	updatedResourceList map[schema.GroupVersion]*metav1.APIResourceList
	currentResourceList map[schema.GroupVersion]*metav1.APIResourceList
}

func (c *mockAggregatedDiscoveryClient) GroupsAndMaybeResources() (*metav1.APIGroupList, map[schema.GroupVersion]*metav1.APIResourceList, map[schema.GroupVersion]error, error) {
	if !c.initialized {
		c.initialized = true
		c.currentResourceList = c.initialResourceList
		return c.initialGroupList, c.initialResourceList, nil, nil
	}
	c.currentResourceList = c.updatedResourceList
	return c.updatedGroupList, c.updatedResourceList, nil, nil
}

func (c *mockAggregatedDiscoveryClient) ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error) {
	for gv, resources := range c.currentResourceList {
		if gv.String() == groupVersion {
			return resources, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: groupVersion, Resource: ""}, "")
}
