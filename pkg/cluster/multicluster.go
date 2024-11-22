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

// Provider allows to retrieve clusters by name. The provider is responsible for discovering
// and managing the lifecycle of each cluster, calling `Engage` and `Disengage` on the manager
// it is run against whenever a new cluster is discovered or a cluster is unregistered.
//
// Example: A Cluster API provider would be responsible for discovering and
// managing clusters that are backed by Cluster API resources, which can live
// in multiple namespaces in a single management cluster.
type Provider interface {
	// Get returns a cluster for the given identifying cluster name. Get
	// returns an existing cluster if it has been created before.
	// If no cluster is known to the provider under the given cluster name,
	// an error should be returned.
	Get(ctx context.Context, clusterName string) (Cluster, error)
}
