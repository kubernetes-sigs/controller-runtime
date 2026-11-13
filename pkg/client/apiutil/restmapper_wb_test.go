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
	"slices"
	"sync"
	"testing"
	"time"

	gmg "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/utils/clock"
	testingclock "k8s.io/utils/clock/testing"
)

func TestLazyRestMapper_fetchGroupVersionResources_CacheInvalidation(t *testing.T) {
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
				missBackoff: flowcontrol.NewBackOff(missBackoffInitial, missBackoffMax),
			}
			_, notFound, err := m.fetchGroupVersionResources(tt.groupName, tt.versions...)
			g.Expect(err).NotTo(gmg.HaveOccurred())
			m.removeNotFoundGroupVersionsLocked(notFound)
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

func TestLazyRestMapper_MissedGroupVersions(t *testing.T) {
	deployments := &metav1.APIResourceList{
		GroupVersion: "apps/v1",
		APIResources: []metav1.APIResource{{Name: "deployments", Kind: "Deployment", Namespaced: true}},
	}
	widgets := &metav1.APIResourceList{
		GroupVersion: "widgets.example.com/v1",
		APIResources: []metav1.APIResource{{Name: "widgets", Kind: "Widget", Namespaced: true}},
	}
	deploymentsAndMissings := &metav1.APIResourceList{
		GroupVersion: "apps/v1",
		APIResources: append(slices.Clone(deployments.APIResources), metav1.APIResource{Name: "missings", Kind: "Missing", Namespaced: true}),
	}

	tests := []struct {
		name string
		// lookup misses until the server serves updated.
		lookup  func(m *mapper) error
		updated []*metav1.APIResourceList
	}{
		{
			name: "kind missing in a served group version",
			lookup: func(m *mapper) error {
				_, err := m.RESTMapping(schema.GroupKind{Group: "apps", Kind: "Missing"}, "v1")
				return err
			},
			updated: []*metav1.APIResourceList{deploymentsAndMissings},
		},
		{
			name: "kind missing in a served group without a version",
			lookup: func(m *mapper) error {
				_, err := m.KindFor(schema.GroupVersionResource{Group: "apps", Resource: "missings"})
				return err
			},
			updated: []*metav1.APIResourceList{deploymentsAndMissings},
		},
		{
			name: "group version the server doesn't serve",
			lookup: func(m *mapper) error {
				_, err := m.RESTMapping(schema.GroupKind{Group: "widgets.example.com", Kind: "Widget"}, "v1")
				return err
			},
			updated: []*metav1.APIResourceList{deployments, widgets},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gmg.NewWithT(t)

			client := newFakeDiscoveryClient(deployments)
			clk := testingclock.NewFakeClock(time.Now())
			m := newTestMapper(client, clk)

			_, err := m.RESTMapping(schema.GroupKind{Group: "apps", Kind: "Deployment"}, "v1")
			g.Expect(err).NotTo(gmg.HaveOccurred())
			client.resetRequests()

			// Only the first miss fetches the group version.
			for range 10 {
				g.Expect(tt.lookup(m)).To(gmg.Satisfy(meta.IsNoMatchError))
			}
			g.Expect(client.getRequests()).To(gmg.Equal(1))

			// The next miss after the backoff fetches it again, and the backoff doubles.
			clk.Step(missBackoffInitial - time.Millisecond)
			g.Expect(tt.lookup(m)).To(gmg.Satisfy(meta.IsNoMatchError))
			g.Expect(client.getRequests()).To(gmg.Equal(1))

			clk.Step(time.Millisecond)
			g.Expect(tt.lookup(m)).To(gmg.Satisfy(meta.IsNoMatchError))
			g.Expect(client.getRequests()).To(gmg.Equal(2))

			// A kind added to the group version is found once the backoff has passed.
			client.setResources(tt.updated...)
			clk.Step(2*missBackoffInitial - time.Millisecond)
			g.Expect(tt.lookup(m)).To(gmg.Satisfy(meta.IsNoMatchError))
			g.Expect(client.getRequests()).To(gmg.Equal(2))

			clk.Step(time.Millisecond)
			g.Expect(tt.lookup(m)).To(gmg.Succeed())
			g.Expect(client.getRequests()).To(gmg.Equal(3))
		})
	}
}

func TestLazyRestMapper_LookupsDontWaitForDiscovery(t *testing.T) {
	g := gmg.NewWithT(t)

	client := newFakeDiscoveryClient(&metav1.APIResourceList{
		GroupVersion: "apps/v1",
		APIResources: []metav1.APIResource{{Name: "deployments", Kind: "Deployment", Namespaced: true}},
	})
	// The clock doesn't move, so the backoff can't pass while workers wait to be scheduled.
	m := newTestMapper(client, testingclock.NewFakeClock(time.Now()))

	_, err := m.RESTMapping(schema.GroupKind{Group: "apps", Kind: "Deployment"}, "v1")
	g.Expect(err).NotTo(gmg.HaveOccurred())
	client.resetRequests()

	held, release := client.holdRequests()
	releaseRequests := sync.OnceFunc(func() { close(release) })
	errs := make(chan error, 10)
	var wg sync.WaitGroup
	// Don't leave the workers blocked if an assertion fails.
	t.Cleanup(func() {
		releaseRequests()
		wg.Wait()
	})
	for range 10 {
		wg.Go(func() {
			_, err := m.RESTMapping(schema.GroupKind{Group: "apps", Kind: "Missing"}, "v1")
			errs <- err
		})
	}
	g.Eventually(held).WithTimeout(10 * time.Second).Should(gmg.Receive())

	// A lookup of a cached kind doesn't wait for the discovery request.
	done := make(chan error, 1)
	go func() {
		_, err := m.RESTMapping(schema.GroupKind{Group: "apps", Kind: "Deployment"}, "v1")
		done <- err
	}()
	g.Eventually(done).WithTimeout(10 * time.Second).Should(gmg.Receive(gmg.Succeed()))

	releaseRequests()
	wg.Wait()
	close(errs)
	for err := range errs {
		g.Expect(err).To(gmg.Satisfy(meta.IsNoMatchError))
	}
	// Concurrent misses in the same group version send one request.
	g.Expect(client.getRequests()).To(gmg.Equal(1))
}

func TestLazyRestMapper_NotFoundVersionKeepsVersionsInBackoff(t *testing.T) {
	g := gmg.NewWithT(t)

	widgets := func(version string) *metav1.APIResourceList {
		return &metav1.APIResourceList{
			GroupVersion: "widgets.example.com/" + version,
			APIResources: []metav1.APIResource{{Name: "widgets", Kind: "Widget", Namespaced: true}},
		}
	}
	client := newFakeDiscoveryClient(widgets("v1"), widgets("v2"), widgets("v3"))
	m := newTestMapper(client, testingclock.NewFakeClock(time.Now()))

	_, err := m.RESTMapping(schema.GroupKind{Group: "widgets.example.com", Kind: "Widget"})
	g.Expect(err).NotTo(gmg.HaveOccurred())

	// Put v1 in backoff.
	_, err = m.RESTMapping(schema.GroupKind{Group: "widgets.example.com", Kind: "Missing"}, "v1")
	g.Expect(err).To(gmg.Satisfy(meta.IsNoMatchError))

	// The server stops serving v2. A miss without a version fetches all versions, finds that v2
	// is gone, and rebuilds the group's cache. v1 must stay in it although it is in backoff.
	client.setResources(widgets("v1"), widgets("v3"))
	client.resetRequests()
	_, err = m.RESTMapping(schema.GroupKind{Group: "widgets.example.com", Kind: "Missing"})
	g.Expect(err).To(gmg.Satisfy(meta.IsNoMatchError))
	g.Expect(client.getRequests()).To(gmg.Equal(3))

	mappings, err := m.RESTMappings(schema.GroupKind{Group: "widgets.example.com", Kind: "Widget"})
	g.Expect(err).NotTo(gmg.HaveOccurred())
	versions := make([]string, 0, len(mappings))
	for _, mapping := range mappings {
		versions = append(versions, mapping.GroupVersionKind.Version)
	}
	g.Expect(versions).To(gmg.ConsistOf("v1", "v3"))
	g.Expect(client.getRequests()).To(gmg.Equal(3))
}

func newTestMapper(client discovery.AggregatedDiscoveryInterface, clk clock.Clock) *mapper {
	return &mapper{
		mapper:      restmapper.NewDiscoveryRESTMapper([]*restmapper.APIGroupResources{}),
		client:      client,
		knownGroups: map[string]*restmapper.APIGroupResources{},
		apiGroups:   map[string]*metav1.APIGroup{},
		missBackoff: flowcontrol.NewFakeBackOff(missBackoffInitial, missBackoffMax, clk),
	}
}

// fakeDiscoveryClient serves the passed resources, counts requests for group versions,
// and can hold those requests.
type fakeDiscoveryClient struct {
	*fakeAggregatedDiscoveryClient
	fake *fakediscovery.FakeDiscovery

	mu       sync.Mutex
	requests int
	held     chan struct{}
	release  chan struct{}
}

func newFakeDiscoveryClient(resources ...*metav1.APIResourceList) *fakeDiscoveryClient {
	fake := fake.NewClientset().Discovery().(*fakediscovery.FakeDiscovery)
	fake.Resources = resources
	return &fakeDiscoveryClient{
		fakeAggregatedDiscoveryClient: &fakeAggregatedDiscoveryClient{DiscoveryInterface: fake},
		fake:                          fake,
	}
}

func (f *fakeDiscoveryClient) ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error) {
	f.mu.Lock()
	f.requests++
	held, release := f.held, f.release
	f.mu.Unlock()

	if release != nil {
		select {
		case held <- struct{}{}:
		default:
		}
		<-release
	}
	return f.fakeAggregatedDiscoveryClient.ServerResourcesForGroupVersion(groupVersion)
}

// holdRequests makes requests for group versions wait until release is closed.
// held receives a value when a request starts waiting.
func (f *fakeDiscoveryClient) holdRequests() (held <-chan struct{}, release chan<- struct{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.held = make(chan struct{}, 1)
	f.release = make(chan struct{})
	return f.held, f.release
}

// setResources replaces the served resources. It must not be called during a request.
func (f *fakeDiscoveryClient) setResources(resources ...*metav1.APIResourceList) {
	f.fake.Resources = resources
}

func (f *fakeDiscoveryClient) getRequests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests
}

func (f *fakeDiscoveryClient) resetRequests() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = 0
}
