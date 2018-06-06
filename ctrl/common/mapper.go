package common

import (
	"k8s.io/client-go/rest"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
)	

// NewDiscoveryRESTMapper constructs a new RESTMapper based on discovery
// information fetched by a new client with the given config.
func NewDiscoveryRESTMapper(c *rest.Config) (meta.RESTMapper, error) {
	// Get a mapper
	dc := discovery.NewDiscoveryClientForConfigOrDie(c)
	gr, err := discovery.GetAPIGroupResources(dc)
	if err != nil {
		return nil, err
	}
	return discovery.NewRESTMapper(gr, dynamic.VersionInterfaces), nil
}
