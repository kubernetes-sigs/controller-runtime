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

package apiutil

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

// DynamicRESTMapper is a RESTMapper that implements both the context-aware
// meta.RESTMapperWithContext and the legacy meta.RESTMapper interface.
//
// The methods of meta.RESTMapperWithContext should be preferred because they support
// cancellation and contextual logging. The legacy methods are copied from meta.RESTMapper
// and use context.Background().
type DynamicRESTMapper interface {
	meta.RESTMapperWithContext

	// The following methods are copied from meta.RESTMapper so we can mark them as deprecated.

	// KindFor takes a partial resource and returns the single match.  Returns an error if there are multiple matches
	//
	// Deprecated: Use KindForWithContext instead, it supports cancellation and contextual logging.
	KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error)

	// KindsFor takes a partial resource and returns the list of potential kinds in priority order
	//
	// Deprecated: Use KindsForWithContext instead, it supports cancellation and contextual logging.
	KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error)

	// ResourceFor takes a partial resource and returns the single match.  Returns an error if there are multiple matches
	//
	// Deprecated: Use ResourceForWithContext instead, it supports cancellation and contextual logging.
	ResourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error)

	// ResourcesFor takes a partial resource and returns the list of potential resource in priority order
	//
	// Deprecated: Use ResourcesForWithContext instead, it supports cancellation and contextual logging.
	ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error)

	// RESTMapping identifies a preferred resource mapping for the provided group kind.
	//
	// Deprecated: Use RESTMappingWithContext instead, it supports cancellation and contextual logging.
	RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error)

	// RESTMappings returns all resource mappings for the provided group kind if no
	// version search is provided. Otherwise identifies a preferred resource mapping for
	// the provided version(s).
	//
	// Deprecated: Use RESTMappingsWithContext instead, it supports cancellation and contextual logging.
	RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error)

	// ResourceSingularizer converts a resource name from plural to singular (e.g., from pods to pod).
	//
	// Deprecated: Use ResourceSingularizerWithContext instead, it supports cancellation and contextual logging.
	ResourceSingularizer(resource string) (singular string, err error)
}

var (
	// DynamicRESTMapper must stay assignable to both upstream interfaces.
	_ meta.RESTMapper            = DynamicRESTMapper(nil)
	_ meta.RESTMapperWithContext = DynamicRESTMapper(nil)
)

// NewDynamicRESTMapper returns a dynamic RESTMapper for cfg. The dynamic
// RESTMapper dynamically discovers resource types at runtime.
//
// The returned mapper implements both meta.RESTMapperWithContext and meta.RESTMapper.
//
// The context is only used for the duration of the call and does not bound the lifetime
// of the returned mapper. The mapper does not run discovery on construction, but only
// lazily when mapping is requested.
func NewDynamicRESTMapper(_ context.Context, cfg *rest.Config, httpClient *http.Client) (DynamicRESTMapper, error) {
	if httpClient == nil {
		return nil, fmt.Errorf("httpClient must not be nil, consider using rest.HTTPClientFor(c) to create a client")
	}

	client, err := discovery.NewDiscoveryClientForConfigAndClient(cfg, httpClient)
	if err != nil {
		return nil, err
	}

	return &mapper{
		mapper:      restmapper.NewDiscoveryRESTMapperWithContext([]*restmapper.APIGroupResources{}),
		client:      client,
		knownGroups: map[string]*restmapper.APIGroupResources{},
		apiGroups:   map[string]*metav1.APIGroup{},
	}, nil
}

var _ DynamicRESTMapper = &mapper{}

// mapper is a RESTMapper that will lazily query the provided
// client for discovery information to do REST mappings.
type mapper struct {
	mapper      meta.RESTMapperWithContext
	client      discovery.AggregatedDiscoveryInterfaceWithContext
	knownGroups map[string]*restmapper.APIGroupResources
	apiGroups   map[string]*metav1.APIGroup

	initialDiscoveryDone bool

	// mutex to provide thread-safe mapper reloading.
	// It protects all fields in the mapper as well as methods
	// that have the `Locked` suffix.
	mu sync.RWMutex
}

// KindFor implements Mapper.KindFor.
//
// KindForWithContext is a better alternative because it supports contextual logging and cancellation.
func (m *mapper) KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	return m.KindForWithContext(context.Background(), resource)
}

// KindForWithContext implements meta.RESTMapperWithContext.KindForWithContext.
func (m *mapper) KindForWithContext(ctx context.Context, resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	res, err := m.getMapper().KindForWithContext(ctx, resource)
	if meta.IsNoMatchError(err) {
		if err := m.addKnownGroupAndReload(ctx, resource.Group, resource.Version); err != nil {
			return schema.GroupVersionKind{}, err
		}
		res, err = m.getMapper().KindForWithContext(ctx, resource)
	}

	return res, err
}

// KindsFor implements Mapper.KindsFor.
//
// KindsForWithContext is a better alternative because it supports contextual logging and cancellation.
func (m *mapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	return m.KindsForWithContext(context.Background(), resource)
}

// KindsForWithContext implements meta.RESTMapperWithContext.KindsForWithContext.
func (m *mapper) KindsForWithContext(ctx context.Context, resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	res, err := m.getMapper().KindsForWithContext(ctx, resource)
	if meta.IsNoMatchError(err) {
		if err := m.addKnownGroupAndReload(ctx, resource.Group, resource.Version); err != nil {
			return nil, err
		}
		res, err = m.getMapper().KindsForWithContext(ctx, resource)
	}

	return res, err
}

// ResourceFor implements Mapper.ResourceFor.
//
// ResourceForWithContext is a better alternative because it supports contextual logging and cancellation.
func (m *mapper) ResourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	return m.ResourceForWithContext(context.Background(), input)
}

// ResourceForWithContext implements meta.RESTMapperWithContext.ResourceForWithContext.
func (m *mapper) ResourceForWithContext(ctx context.Context, input schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	res, err := m.getMapper().ResourceForWithContext(ctx, input)
	if meta.IsNoMatchError(err) {
		if err := m.addKnownGroupAndReload(ctx, input.Group, input.Version); err != nil {
			return schema.GroupVersionResource{}, err
		}
		res, err = m.getMapper().ResourceForWithContext(ctx, input)
	}

	return res, err
}

// ResourcesFor implements Mapper.ResourcesFor.
//
// ResourcesForWithContext is a better alternative because it supports contextual logging and cancellation.
func (m *mapper) ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	return m.ResourcesForWithContext(context.Background(), input)
}

// ResourcesForWithContext implements meta.RESTMapperWithContext.ResourcesForWithContext.
func (m *mapper) ResourcesForWithContext(ctx context.Context, input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	res, err := m.getMapper().ResourcesForWithContext(ctx, input)
	if meta.IsNoMatchError(err) {
		if err := m.addKnownGroupAndReload(ctx, input.Group, input.Version); err != nil {
			return nil, err
		}
		res, err = m.getMapper().ResourcesForWithContext(ctx, input)
	}

	return res, err
}

// RESTMapping implements Mapper.RESTMapping.
//
// RESTMappingWithContext is a better alternative because it supports contextual logging and cancellation.
func (m *mapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	return m.RESTMappingWithContext(context.Background(), gk, versions...)
}

// RESTMappingWithContext implements meta.RESTMapperWithContext.RESTMappingWithContext.
func (m *mapper) RESTMappingWithContext(ctx context.Context, gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	res, err := m.getMapper().RESTMappingWithContext(ctx, gk, versions...)
	if meta.IsNoMatchError(err) {
		if err := m.addKnownGroupAndReload(ctx, gk.Group, versions...); err != nil {
			return nil, err
		}
		res, err = m.getMapper().RESTMappingWithContext(ctx, gk, versions...)
	}

	return res, err
}

// RESTMappings implements Mapper.RESTMappings.
//
// RESTMappingsWithContext is a better alternative because it supports contextual logging and cancellation.
func (m *mapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	return m.RESTMappingsWithContext(context.Background(), gk, versions...)
}

// RESTMappingsWithContext implements meta.RESTMapperWithContext.RESTMappingsWithContext.
func (m *mapper) RESTMappingsWithContext(ctx context.Context, gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	res, err := m.getMapper().RESTMappingsWithContext(ctx, gk, versions...)
	if meta.IsNoMatchError(err) {
		if err := m.addKnownGroupAndReload(ctx, gk.Group, versions...); err != nil {
			return nil, err
		}
		res, err = m.getMapper().RESTMappingsWithContext(ctx, gk, versions...)
	}

	return res, err
}

// ResourceSingularizer implements Mapper.ResourceSingularizer.
//
// ResourceSingularizerWithContext is a better alternative because it supports contextual logging and cancellation.
func (m *mapper) ResourceSingularizer(resource string) (string, error) {
	return m.ResourceSingularizerWithContext(context.Background(), resource)
}

// ResourceSingularizerWithContext implements meta.RESTMapperWithContext.ResourceSingularizerWithContext.
func (m *mapper) ResourceSingularizerWithContext(ctx context.Context, resource string) (string, error) {
	return m.getMapper().ResourceSingularizerWithContext(ctx, resource)
}

func (m *mapper) getMapper() meta.RESTMapperWithContext {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mapper
}

// addKnownGroupAndReload reloads the mapper with updated information about missing API group.
// versions can be specified for partial updates, for instance for v1beta1 version only.
func (m *mapper) addKnownGroupAndReload(ctx context.Context, groupName string, versions ...string) error {
	// versions will here be [""] if the forwarded Version value of
	// GroupVersionResource (in calling method) was not specified.
	if len(versions) == 1 && versions[0] == "" {
		versions = nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// If no specific versions are set by user, we will scan all available ones for the API group.
	// This operation requires 2 requests: /api and /apis, but only once. For all subsequent calls
	// this data will be taken from cache.
	//
	// We always run this once, because if the server supports aggregated discovery, this will
	// load everything with two api calls which we assume is overall cheaper.
	if len(versions) == 0 || !m.initialDiscoveryDone {
		apiGroup, didAggregatedDiscovery, err := m.findAPIGroupByNameAndMaybeAggregatedDiscoveryLocked(ctx, groupName)
		if err != nil {
			return err
		}
		if apiGroup != nil && len(versions) == 0 {
			for _, version := range apiGroup.Versions {
				versions = append(versions, version.Version)
			}
		}

		// No need to do anything further if aggregatedDiscovery is supported and we did a lookup
		if didAggregatedDiscovery {
			failedGroups := make(map[schema.GroupVersion]error)
			for _, version := range versions {
				if m.knownGroups[groupName] == nil || m.knownGroups[groupName].VersionedResources[version] == nil {
					failedGroups[schema.GroupVersion{Group: groupName, Version: version}] = &meta.NoResourceMatchError{
						PartialResource: schema.GroupVersionResource{
							Group:   groupName,
							Version: version,
						}}
				}
			}
			if len(failedGroups) > 0 {
				return new(ErrResourceDiscoveryFailed(failedGroups))
			}
			return nil
		}
	}

	// Update information for group resources about versioned resources.
	// The number of API calls is equal to the number of versions: /apis/<group>/<version>.
	// If we encounter a missing API version (NotFound error), we will remove the group from
	// the m.apiGroups and m.knownGroups caches.
	// If this happens, in the next call the group will be added back to apiGroups
	// and only the existing versions will be loaded in knownGroups.
	groupVersionResources, err := m.fetchGroupVersionResourcesLocked(ctx, groupName, versions...)
	if err != nil {
		return fmt.Errorf("failed to get API group resources: %w", err)
	}

	m.addGroupVersionResourcesToCacheAndReloadLocked(groupVersionResources)
	return nil
}

// addGroupVersionResourcesToCacheAndReloadLocked does what the name suggests. The mutex must be held when
// calling it.
func (m *mapper) addGroupVersionResourcesToCacheAndReloadLocked(gvr map[schema.GroupVersion]*metav1.APIResourceList) {
	// Update information for group resources about the API group by adding new versions.
	// Ignore the versions that are already registered
	for groupVersion, resources := range gvr {
		var groupResources *restmapper.APIGroupResources
		if _, ok := m.knownGroups[groupVersion.Group]; ok {
			groupResources = m.knownGroups[groupVersion.Group]
		} else {
			groupResources = &restmapper.APIGroupResources{
				Group:              metav1.APIGroup{Name: groupVersion.Group},
				VersionedResources: make(map[string][]metav1.APIResource),
			}
		}

		version := groupVersion.Version

		groupResources.VersionedResources[version] = resources.APIResources
		found := false
		for _, v := range groupResources.Group.Versions {
			if v.Version == version {
				found = true
				break
			}
		}

		if !found {
			gv := metav1.GroupVersionForDiscovery{
				GroupVersion: metav1.GroupVersion{Group: groupVersion.Group, Version: version}.String(),
				Version:      version,
			}

			// Prepend if preferred version, else append. The upstream DiscoveryRestMappper assumes
			// the first version is the preferred one: https://github.com/kubernetes/kubernetes/blob/ef54ac803b712137871c1a1f8d635d50e69ffa6c/staging/src/k8s.io/apimachinery/pkg/api/meta/restmapper.go#L458-L461
			if group, ok := m.apiGroups[groupVersion.Group]; ok && group.PreferredVersion.Version == version {
				groupResources.Group.Versions = append([]metav1.GroupVersionForDiscovery{gv}, groupResources.Group.Versions...)
			} else {
				groupResources.Group.Versions = append(groupResources.Group.Versions, gv)
			}
		}

		// Update data in the cache.
		m.knownGroups[groupVersion.Group] = groupResources
	}

	// Finally, reload the mapper.
	updatedGroupResources := make([]*restmapper.APIGroupResources, 0, len(m.knownGroups))
	for _, agr := range m.knownGroups {
		updatedGroupResources = append(updatedGroupResources, agr)
	}

	m.mapper = restmapper.NewDiscoveryRESTMapperWithContext(updatedGroupResources)
}

// findAPIGroupByNameAndMaybeAggregatedDiscoveryLocked tries to find the passed apiGroup.
// If the server supports aggregated discovery, it will always perform that.
func (m *mapper) findAPIGroupByNameAndMaybeAggregatedDiscoveryLocked(ctx context.Context, groupName string) (_ *metav1.APIGroup, didAggregatedDiscovery bool, _ error) {
	// Looking in the cache first
	group, ok := m.apiGroups[groupName]
	if ok {
		return group, false, nil
	}

	// Update the cache if nothing was found.
	apiGroups, maybeResources, _, err := m.client.GroupsAndMaybeResourcesWithContext(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get server groups: %w", err)
	}
	if len(apiGroups.Groups) == 0 {
		return nil, false, fmt.Errorf("received an empty API groups list")
	}

	m.initialDiscoveryDone = true
	for i := range apiGroups.Groups {
		group := &apiGroups.Groups[i]
		m.apiGroups[group.Name] = group
	}
	if len(maybeResources) > 0 {
		didAggregatedDiscovery = true
		m.addGroupVersionResourcesToCacheAndReloadLocked(maybeResources)
	}

	// Looking in the cache again.
	// Don't return an error here if the API group is not present.
	// The reloaded RESTMapper will take care of returning a NoMatchError.
	return m.apiGroups[groupName], didAggregatedDiscovery, nil
}

// fetchGroupVersionResourcesLocked fetches the resources for the specified group and its versions.
// This method might modify the cache so it needs to be called under the lock.
func (m *mapper) fetchGroupVersionResourcesLocked(ctx context.Context, groupName string, versions ...string) (map[schema.GroupVersion]*metav1.APIResourceList, error) {
	groupVersionResources := make(map[schema.GroupVersion]*metav1.APIResourceList)
	failedGroups := make(map[schema.GroupVersion]error)

	for _, version := range versions {
		groupVersion := schema.GroupVersion{Group: groupName, Version: version}

		apiResourceList, err := m.client.ServerResourcesForGroupVersionWithContext(ctx, groupVersion.String())
		if apierrors.IsNotFound(err) {
			// If the version is not found, we remove the group from the cache
			// so it gets refreshed on the next call.
			if m.isAPIGroupCachedLocked(groupVersion) {
				delete(m.apiGroups, groupName)
			}
			if m.isGroupVersionCachedLocked(groupVersion) {
				delete(m.knownGroups, groupName)
			}
			continue
		} else if err != nil {
			failedGroups[groupVersion] = err
		}

		if apiResourceList != nil {
			// even in case of error, some fallback might have been returned.
			groupVersionResources[groupVersion] = apiResourceList
		}
	}

	if len(failedGroups) > 0 {
		err := ErrResourceDiscoveryFailed(failedGroups)
		return nil, &err
	}

	return groupVersionResources, nil
}

// isGroupVersionCachedLocked checks if a version for a group is cached in the known groups cache.
func (m *mapper) isGroupVersionCachedLocked(gv schema.GroupVersion) bool {
	if cachedGroup, ok := m.knownGroups[gv.Group]; ok {
		_, cached := cachedGroup.VersionedResources[gv.Version]
		return cached
	}

	return false
}

// isAPIGroupCachedLocked checks if a version for a group is cached in the api groups cache.
func (m *mapper) isAPIGroupCachedLocked(gv schema.GroupVersion) bool {
	cachedGroup, ok := m.apiGroups[gv.Group]
	if !ok {
		return false
	}

	for _, version := range cachedGroup.Versions {
		if version.Version == gv.Version {
			return true
		}
	}

	return false
}
