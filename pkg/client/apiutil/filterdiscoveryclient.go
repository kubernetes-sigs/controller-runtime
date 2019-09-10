package apiutil

import (
	openapi_v2 "github.com/googleapis/gnostic/OpenAPIv2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

// FilterGroupFunc is a plugin function to FilterDiscoveryClient
// It should return true for groups that should be included in the discovery,
// and false for groups that should be excluded.
type FilterGroupFunc func(groupVersion string) bool

// FilterDiscoveryClient is a wrapper to a discovery interface that adds group filtering
// in order to reduce the number of groups that are read from the server.
// It should help to reduce the amount of api calls for clients that require just a few number of groups.
// Perhaps this can be later added as an optional filter to client-go discovery client to avoid the need for wrapping.
type FilterDiscoveryClient struct {
	Discovery discovery.DiscoveryInterface
	Filter    FilterGroupFunc
}

// ServerResourcesForGroupVersion is doing the actual filtering
func (d *FilterDiscoveryClient) ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error) {
	if d.Filter(groupVersion) {
		return d.Discovery.ServerResourcesForGroupVersion(groupVersion)
	}
	return &metav1.APIResourceList{
		GroupVersion: groupVersion,
		APIResources: []metav1.APIResource{},
	}, nil
}

// ServerGroupsAndResources calls the public module function with d as the interface
// so that it will callback to d.ServerResourcesForGroupVersion(groupVersion) for filtering
func (d *FilterDiscoveryClient) ServerGroupsAndResources() ([]*metav1.APIGroup, []*metav1.APIResourceList, error) {
	// withRetries was copied from client-go/discovery in order to behave like the standard client
	// but with the discovery interface calling back to our wrapper
	return withRetries(defaultRetries, func() ([]*metav1.APIGroup, []*metav1.APIResourceList, error) {
		return discovery.ServerGroupsAndResources(d)
	})
}

// ServerPreferredResources is copied from DiscoveryClient
func (d *FilterDiscoveryClient) ServerPreferredResources() ([]*metav1.APIResourceList, error) {
	// withRetries was copied from client-go/discovery in order to behave like the standard client
	// but with the discovery interface calling back to our wrapper
	_, rs, err := withRetries(defaultRetries, func() ([]*metav1.APIGroup, []*metav1.APIResourceList, error) {
		rs, err := discovery.ServerPreferredResources(d)
		return nil, rs, err
	})
	return rs, err
}

// ServerResources is returning just the resources from the ServerGroupsAndResources() response
func (d *FilterDiscoveryClient) ServerResources() ([]*metav1.APIResourceList, error) {
	_, rs, err := d.ServerGroupsAndResources()
	return rs, err
}

// ServerPreferredNamespacedResources is a proxy func
func (d *FilterDiscoveryClient) ServerPreferredNamespacedResources() ([]*metav1.APIResourceList, error) {
	return discovery.ServerPreferredNamespacedResources(d)
}

// ServerGroups is delegated to the underlying discovery client
func (d *FilterDiscoveryClient) ServerGroups() (*metav1.APIGroupList, error) {
	return d.Discovery.ServerGroups()
}

// RESTClient is delegated to the underlying discovery client
func (d *FilterDiscoveryClient) RESTClient() rest.Interface {
	return d.Discovery.RESTClient()
}

// ServerVersion is delegated to the underlying discovery client
func (d *FilterDiscoveryClient) ServerVersion() (*version.Info, error) {
	return d.Discovery.ServerVersion()
}

// OpenAPISchema is delegated to the underlying discovery client
func (d *FilterDiscoveryClient) OpenAPISchema() (*openapi_v2.Document, error) {
	return d.Discovery.OpenAPISchema()
}

// defaultRetries is the number of times a resource discovery is repeated if an api group disappears on the fly (e.g. ThirdPartyResources).
const defaultRetries = 2

// withRetries retries the given recovery function in case the groups supported by the server change after ServerGroup() returns.
// This was copied from client-go/discovery in order to wrap the discovery client.
func withRetries(
	maxRetries int,
	f func() (
		[]*metav1.APIGroup,
		[]*metav1.APIResourceList,
		error,
	),
) (
	[]*metav1.APIGroup,
	[]*metav1.APIResourceList,
	error,
) {
	var result []*metav1.APIResourceList
	var resultGroups []*metav1.APIGroup
	var err error
	for i := 0; i < maxRetries; i++ {
		resultGroups, result, err = f()
		if err == nil {
			return resultGroups, result, nil
		}
		if _, ok := err.(*discovery.ErrGroupDiscoveryFailed); !ok {
			return nil, nil, err
		}
	}
	return resultGroups, result, err
}
