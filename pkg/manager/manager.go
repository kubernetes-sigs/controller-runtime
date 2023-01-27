/*
Copyright 2018 The Kubernetes Authors.

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

package manager

import (
	"context"
	"net/http"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/internal/manager"
	intrec "sigs.k8s.io/controller-runtime/pkg/internal/recorder"
	"sigs.k8s.io/controller-runtime/pkg/leaderelection"
	"sigs.k8s.io/controller-runtime/pkg/manager/api"
)

// Manager initializes shared dependencies such as Caches and Clients, and provides them to Runnables.
// A Manager is required to create Controllers.
type Manager api.Manager

// BaseContextFunc is a function used to provide a base Context to Runnables
// managed by a Manager.
type BaseContextFunc api.BaseContextFunc

// Runnable allows a component to be started.
// It's very important that Start blocks until
// it's done running.
type Runnable api.Runnable

// RunnableFunc implements Runnable using a function.
// It's very important that the given function block
// until it's done running.
type RunnableFunc api.RunnableFunc

// Start implements Runnable.
func (r RunnableFunc) Start(ctx context.Context) error {
	return r(ctx)
}

// LeaderElectionRunnable knows if a Runnable needs to be run in the leader election mode.
type LeaderElectionRunnable api.LeaderElectionRunnable

// New returns a new Manager for creating Controllers.
func New(config *rest.Config, opts ...manager.SetOptions) (Manager, error) {
	options := manager.Options{}
	for _, opt := range opts {
		if err := opt.ApplyToManager(&options); err != nil {
			return nil, err
		}
	}
	// Set default values for options fields
	options.Defaults()

	cluster, err := cluster.New(config, func(clusterOptions *cluster.Options) {
		clusterOptions.Scheme = options.Scheme
		clusterOptions.MapperProvider = options.MapperProvider
		clusterOptions.Logger = options.Logger
		clusterOptions.SyncPeriod = options.SyncPeriod
		clusterOptions.Namespace = options.Namespace
		clusterOptions.NewCache = options.NewCache
		clusterOptions.NewClient = options.NewClient
		clusterOptions.ClientDisableCacheFor = options.ClientDisableCacheFor
		clusterOptions.DryRunClient = options.DryRunClient
		clusterOptions.EventBroadcaster = options.EventBroadcaster //nolint:staticcheck
	})
	if err != nil {
		return nil, err
	}

	// Create the recorder provider to inject event recorders for the components.
	// TODO(directxman12): the log for the event provider should have a context (name, tags, etc) specific
	// to the particular controller that it's being injected into, rather than a generic one like is here.
	recorderProvider, err := options.NewRecorderProvider(config, cluster.GetScheme(), options.Logger.WithName("events"), options.MakeBroadcaster)
	if err != nil {
		return nil, err
	}

	// Create the resource lock to enable leader election)
	var leaderConfig *rest.Config
	var leaderRecorderProvider *intrec.Provider

	if options.LeaderElectionConfig == nil {
		leaderConfig = rest.CopyConfig(config)
		leaderRecorderProvider = recorderProvider
	} else {
		leaderConfig = rest.CopyConfig(options.LeaderElectionConfig)
		leaderRecorderProvider, err = options.NewRecorderProvider(leaderConfig, cluster.GetScheme(), options.Logger.WithName("events"), options.MakeBroadcaster)
		if err != nil {
			return nil, err
		}
	}

	var resourceLock resourcelock.Interface
	if options.LeaderElectionResourceLockInterface != nil && options.LeaderElection {
		resourceLock = options.LeaderElectionResourceLockInterface
	} else {
		resourceLock, err = options.NewResourceLock(leaderConfig, leaderRecorderProvider, leaderelection.Options{
			LeaderElection:             options.LeaderElection,
			LeaderElectionResourceLock: options.LeaderElectionResourceLock,
			LeaderElectionID:           options.LeaderElectionID,
			LeaderElectionNamespace:    options.LeaderElectionNamespace,
		})
		if err != nil {
			return nil, err
		}
	}

	// Create the metrics listener. This will throw an error if the metrics bind
	// address is invalid or already in use.
	metricsListener, err := options.NewMetricsListener(options.MetricsBindAddress)
	if err != nil {
		return nil, err
	}

	// By default we have no extra endpoints to expose on metrics http server.
	metricsExtraHandlers := make(map[string]http.Handler)

	// Create health probes listener. This will throw an error if the bind
	// address is invalid or already in use.
	healthProbeListener, err := options.NewHealthProbeListener(options.HealthProbeBindAddress)
	if err != nil {
		return nil, err
	}

	errChan := make(chan error)
	runnables := manager.NewRunnables(options.BaseContext, errChan)

	return &manager.ControllerManager{
		StopProcedureEngaged:          pointer.Int64(0),
		Cluster:                       cluster,
		Runnables:                     runnables,
		ErrChan:                       errChan,
		RecorderProvider:              recorderProvider,
		ResourceLock:                  resourceLock,
		MetricsListener:               metricsListener,
		MetricsExtraHandlers:          metricsExtraHandlers,
		ControllerOptions:             options.Controller,
		Logger:                        options.Logger,
		ElectedChan:                   make(chan struct{}),
		Port:                          options.Port,
		Host:                          options.Host,
		CertDir:                       options.CertDir,
		TLSOpts:                       options.TLSOpts,
		WebhookServer:                 options.WebhookServer,
		LeaderElectionID:              options.LeaderElectionID,
		LeaseDuration:                 *options.LeaseDuration,
		RenewDeadline:                 *options.RenewDeadline,
		RetryPeriod:                   *options.RetryPeriod,
		HealthProbeListener:           healthProbeListener,
		ReadinessEndpointName:         options.ReadinessEndpointName,
		LivenessEndpointName:          options.LivenessEndpointName,
		GracefulShutdownTimeout:       *options.GracefulShutdownTimeout,
		InternalProceduresStop:        make(chan struct{}),
		LeaderElectionStopped:         make(chan struct{}),
		LeaderElectionReleaseOnCancel: options.LeaderElectionReleaseOnCancel,
	}, nil
}
