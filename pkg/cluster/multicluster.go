package cluster

import (
	"context"
)

// Aware is an interface that can be implemented by components that
// can engage and disengage when clusters are added or removed at runtime.
type Aware interface {
	// Engage gets called when the component should start operations for the given Cluster.
	// The given context is tied to the Cluster's lifecycle and will be cancelled when the
	// Cluster is removed or an error occurs.
	//
	// Implementers should return an error if they cannot start operations for the given Cluster,
	// and should ensure this operation is re-entrant and non-blocking.
	//
	//	\_________________|)____.---'--`---.____
	//              ||    \----.________.----/
	//              ||     / /    `--'
	//            __||____/ /_
	//           |___         \
	//               `--------'
	Engage(context.Context, Cluster) error

	// Disengage gets called when the component should stop operations for the given Cluster.
	Disengage(context.Context, Cluster) error
}

// Provider defines methods to retrieve clusters by name. The provider is
// responsible for discovering and managing the lifecycle of each cluster.
//
// Example: A Cluster API provider would be responsible for discovering and
// managing clusters that are backed by Cluster API resources, which can live
// in multiple namespaces in a single management cluster.
type Provider interface {
	// Get returns a cluster for the given identifying cluster name. Get
	// returns an existing cluster if it has been created before.
	Get(ctx context.Context, clusterName string) (Cluster, error)
}
