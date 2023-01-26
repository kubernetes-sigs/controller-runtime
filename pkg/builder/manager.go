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

package builder

import (
	"crypto/tls"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Manager is a builder for a Manager.
func Manager(config *rest.Config) *ManagerBuilder {
	return &ManagerBuilder{cfg: config}
}

// ManagerBuilder builds a Manager.
type ManagerBuilder struct {
	cfg *rest.Config

	cmConfigLoader config.ControllerManagerConfiguration
	opts           []manager.SetOptions
}

// Build builds a Manager.
func (m *ManagerBuilder) Build() (manager.Manager, error) {
	opts := manager.Options{}
	for _, opt := range m.opts {
		if err := opt.ApplyToManager(&opts); err != nil {
			return nil, err
		}
	}
	if m.cmConfigLoader != nil {
		var err error
		opts, err = opts.AndFrom(m.cmConfigLoader) //nolint:staticcheck
		if err != nil {
			return nil, errors.Wrap(err, "failed to load controller manager configuration")
		}
	}
	return manager.New(m.cfg, opts) //nolint:staticcheck
}

// Scheme sets the scheme for the Manager.
func (m *ManagerBuilder) Scheme(scheme *runtime.Scheme) *ManagerBuilder {
	m.opts = append(m.opts, manager.SetOptionsFunc(func(o *manager.Options) error {
		o.Scheme = scheme
		return nil
	}))
	return m
}

// WithLogger sets the logger for the Manager.
func (m *ManagerBuilder) WithLogger(l logr.Logger) *ManagerBuilder {
	m.opts = append(m.opts, manager.SetOptionsFunc(func(o *manager.Options) error {
		o.Logger = l
		return nil
	}))
	return m
}

// UseLeaderElection sets the Manager to use leader election.
func (m *ManagerBuilder) UseLeaderElection(opts LeaderElectionOpts) *ManagerBuilder {
	m.opts = append(m.opts, &opts)
	return m
}

// WithWebhook sets the Manager to use a webhook server
func (m *ManagerBuilder) WithWebhook(opts WebhookOpts) *ManagerBuilder {
	m.opts = append(m.opts, &opts)
	return m
}

// SetMetricsAddress sets the address the metrics endpoint binds to.
func (m *ManagerBuilder) SetMetricsAddress(addr string) *ManagerBuilder {
	m.opts = append(m.opts, manager.SetOptionsFunc(func(o *manager.Options) error {
		o.MetricsBindAddress = addr
		return nil
	}))
	return m
}

// Cache sets the NewCache function used by the Manager with the provided CacheBuilder.
func (m *ManagerBuilder) Cache(builder CacheFactory) *ManagerBuilder {
	m.opts = append(m.opts, manager.SetOptionsFunc(func(o *manager.Options) error {
		o.NewCache = builder.Factory()
		return nil
	}))
	return m
}

// WithConfig sets the ControllerManagerConfiguration for the Manager.
// This will override any other configuration options set on the Manager,
// it's usually useful for loading configuration from a file, or configmap.
func (m *ManagerBuilder) WithConfig(cfg config.ControllerManagerConfiguration) *ManagerBuilder {
	m.cmConfigLoader = cfg
	return m
}

// LeaderElectionOpts contains options for the Manager's leader election.
type LeaderElectionOpts struct {
	// ID determines the name of the resource that leader election
	// will use for holding the leader lock.
	ID string
	// Namespace determines the namespace in which the leader
	// election resource will be created.
	Namespace string

	// ResourceLock determines which resource lock to use for leader election,
	// defaults to "leases". Change this value only if you know what you are doing.
	//
	// If you are using `configmaps`/`endpoints` resource lock and want to migrate to "leases",
	// you might do so by migrating to the respective multilock first ("configmapsleases" or "endpointsleases"),
	// which will acquire a leader lock on both resources.
	// After all your users have migrated to the multilock, you can go ahead and migrate to "leases".
	// Please also keep in mind, that users might skip versions of your controller.
	//
	// Note: before controller-runtime version v0.7, it was set to "configmaps".
	// And from v0.7 to v0.11, the default was "configmapsleases", which was
	// used to migrate from configmaps to leases.
	// Since the default was "configmapsleases" for over a year, spanning five minor releases,
	// any actively maintained operators are very likely to have a released version that uses
	// "configmapsleases". Therefore defaulting to "leases" should be safe since v0.12.
	//
	// So, what do you have to do when you are updating your controller-runtime dependency
	// from a lower version to v0.12 or newer?
	// - If your operator matches at least one of these conditions:
	//   - the LeaderElectionResourceLock in your operator has already been explicitly set to "leases"
	//   - the old controller-runtime version is between v0.7.0 and v0.11.x and the
	//     LeaderElectionResourceLock wasn't set or was set to "leases"/"configmapsleases"/"endpointsleases"
	//   feel free to update controller-runtime to v0.12 or newer.
	// - Otherwise, you may have to take these steps:
	//   1. update controller-runtime to v0.12 or newer in your go.mod
	//   2. set LeaderElectionResourceLock to "configmapsleases" (or "endpointsleases")
	//   3. package your operator and upgrade it in all your clusters
	//   4. only if you have finished 3, you can remove the LeaderElectionResourceLock to use the default "leases"
	// Otherwise, your operator might end up with multiple running instances that
	// each acquired leadership through different resource locks during upgrades and thus
	// act on the same resources concurrently.
	ResourceLock string

	// ReleaseOnCancel defines if the leader should step down voluntarily
	// when the Manager ends. This requires the binary to immediately end when the
	// Manager is stopped, otherwise this setting is unsafe. Setting this significantly
	// speeds up voluntary leader transitions as the new leader doesn't have to wait
	// LeaseDuration time first.
	ReleaseOnCancel bool

	// ResourceLockInterface allows to provide a custom resourcelock.Interface that was created outside
	// of the controller-runtime. If this value is set the options LeaderElectionID, LeaderElectionNamespace,
	// LeaderElectionResourceLock, LeaseDuration, RenewDeadline and RetryPeriod will be ignored. This can be useful if you
	// want to use a locking mechanism that is currently not supported, like a MultiLock across two Kubernetes clusters.
	LockInterface resourcelock.Interface

	// LeaseDuration is the duration that non-leader candidates will
	// wait to force acquire leadership. This is measured against time of
	// last observed ack. Default is 15 seconds.
	LeaseDuration time.Duration
	// RenewDeadline is the duration that the acting controlplane will retry
	// refreshing leadership before giving up. Default is 10 seconds.
	RenewDeadline time.Duration

	// RetryPeriod is the duration the LeaderElector clients should wait
	// between tries of actions. Default is 2 seconds.
	RetryPeriod time.Duration
}

// ApplyToManager applies the options to the manager options.
func (o *LeaderElectionOpts) ApplyToManager(m *manager.Options) error {
	// If we're here, we're using leader election.
	m.LeaderElection = true

	// Set the leader election ID.
	if o.ID != "" {
		m.LeaderElectionID = o.ID
	}
	if o.Namespace != "" {
		m.LeaderElectionNamespace = o.Namespace
	}
	if o.ResourceLock != "" {
		m.LeaderElectionResourceLock = o.ResourceLock
	}
	if o.ReleaseOnCancel {
		m.LeaderElectionReleaseOnCancel = true
	}
	if o.LockInterface != nil {
		m.LeaderElectionResourceLockInterface = o.LockInterface
	}

	if o.LeaseDuration != 0 {
		m.LeaseDuration = &o.LeaseDuration
	}
	if o.RenewDeadline != 0 {
		m.RenewDeadline = &o.RenewDeadline
	}
	if o.RetryPeriod != 0 {
		m.RetryPeriod = &o.RetryPeriod
	}
	return nil
}

// WebhookOpts contains options for the Manager's webhook server.
type WebhookOpts struct {
	Host    string
	Port    int
	CertDir string
	TLSOpts []func(*tls.Config)
}

// ApplyToManager applies the options to the manager options.
func (o *WebhookOpts) ApplyToManager(m *manager.Options) error {
	m.Host = o.Host
	m.Port = o.Port
	m.CertDir = o.CertDir
	m.TLSOpts = o.TLSOpts
	return nil
}
