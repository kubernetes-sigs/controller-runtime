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

package controller

import (
	"context"
	"sync"

	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

// TypedMultiClusterController is a Controller that is aware of the Cluster it is
// running in. It engage and disengage clusters dynamically, starting the
// watches and stopping them.
type TypedMultiClusterController[request comparable] interface {
	cluster.Aware
	TypedController[request]
}

// TypedMultiClusterOption is a functional option for TypedMultiClusterController.
type TypedMultiClusterOption[request comparable] func(*typedMultiClusterController[request])

// ClusterWatcher starts watches for a given Cluster. The ctx should be
// used to cancel the watch when the Cluster is disengaged.
type ClusterWatcher interface {
	Watch(ctx context.Context, cl cluster.Cluster) error
}

// NewTypedMultiClusterController creates a new TypedMultiClusterController for the given
// controller with the given ClusterWatcher.
func NewTypedMultiClusterController[request comparable](c TypedController[request], watcher ClusterWatcher, opts ...TypedMultiClusterOption[request]) TypedMultiClusterController[request] {
	mcc := &typedMultiClusterController[request]{
		TypedController: c,
		watcher:         watcher,
		clusters:        map[string]struct{}{},
	}
	for _, opt := range opts {
		opt(mcc)
	}

	return mcc
}

// WithClusterAware adds the given cluster.Aware instances to the MultiClusterController,
// being engaged and disengaged when the clusters are added or removed.
func WithClusterAware[request comparable](awares ...cluster.Aware) TypedMultiClusterOption[request] {
	return func(c *typedMultiClusterController[request]) {
		c.awares = append(c.awares, awares...)
	}
}

type typedMultiClusterController[request comparable] struct {
	TypedController[request]
	watcher ClusterWatcher

	lock     sync.Mutex
	clusters map[string]struct{}
	awares   []cluster.Aware
}

// Engage gets called when the runnable should start operations for the given Cluster.
func (c *typedMultiClusterController[request]) Engage(clusterCtx context.Context, cl cluster.Cluster) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if _, ok := c.clusters[cl.Name()]; ok {
		return nil
	}

	engaged := make([]cluster.Aware, 0, len(c.awares)+1)
	disengage := func() error {
		var errs []error
		for _, aware := range engaged {
			if err := aware.Disengage(clusterCtx, cl); err != nil {
				errs = append(errs, err)
			}
		}
		return kerrors.NewAggregate(errs)
	}

	// pass through in case the controller itself is cluster aware
	if ctrl, ok := c.TypedController.(cluster.Aware); ok {
		if err := ctrl.Engage(clusterCtx, cl); err != nil {
			return err
		}
		engaged = append(engaged, ctrl)
	}

	// engage cluster aware instances
	for _, aware := range c.awares {
		if err := aware.Engage(clusterCtx, cl); err != nil {
			if err := disengage(); err != nil {
				return err
			}
			return err
		}
		engaged = append(engaged, aware)
	}

	// start watches on the cluster
	if err := c.watcher.Watch(clusterCtx, cl); err != nil {
		if err := disengage(); err != nil {
			return err
		}
		return err
	}

	c.clusters[cl.Name()] = struct{}{}

	return nil
}

// Disengage gets called when the runnable should stop operations for the given Cluster.
func (c *typedMultiClusterController[request]) Disengage(ctx context.Context, cl cluster.Cluster) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if _, ok := c.clusters[cl.Name()]; !ok {
		return nil
	}
	delete(c.clusters, cl.Name())

	// pass through in case the controller itself is cluster aware
	var errs []error
	if ctrl, ok := c.TypedController.(cluster.Aware); ok {
		if err := ctrl.Disengage(ctx, cl); err != nil {
			errs = append(errs, err)
		}
	}

	return kerrors.NewAggregate(errs)
}
