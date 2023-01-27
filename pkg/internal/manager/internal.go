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
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/config/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/internal/httpserver"
	intrec "sigs.k8s.io/controller-runtime/pkg/internal/recorder"
	"sigs.k8s.io/controller-runtime/pkg/manager/api"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var _ api.Runnable = &ControllerManager{}

// ControllerManager is the internal implementation of the Manager interface.
type ControllerManager struct {
	sync.Mutex
	started bool

	StopProcedureEngaged *int64
	ErrChan              chan error
	Runnables            *Runnables

	// Cluster holds a variety of methods to interact with a Cluster. Required.
	Cluster cluster.Cluster

	// RecorderProvider is used to generate event recorders that will be injected into Controllers
	// (and EventHandlers, Sources and Predicates).
	RecorderProvider *intrec.Provider

	// ResourceLock forms the basis for leader election
	ResourceLock resourcelock.Interface

	// LeaderElectionReleaseOnCancel defines if the manager should step back from the leader lease
	// on shutdown
	LeaderElectionReleaseOnCancel bool

	// MetricsListener is used to serve prometheus metrics
	MetricsListener net.Listener

	// MetricsExtraHandlers contains extra handlers to register on http server that serves metrics.
	MetricsExtraHandlers map[string]http.Handler

	// HealthProbeListener is used to serve liveness probe
	HealthProbeListener net.Listener

	// Readiness probe endpoint name
	ReadinessEndpointName string

	// Liveness probe endpoint name
	LivenessEndpointName string

	// Readyz probe handler
	ReadyzHandler *healthz.Handler

	// Healthz probe handler
	HealthzHandler *healthz.Handler

	// ControllerOptions are the global controller options.
	ControllerOptions v1alpha1.ControllerConfigurationSpec

	// Logger is the Logger that should be used by this manager.
	// If none is set, it defaults to log.Log global Logger.
	Logger logr.Logger

	// LeaderElectionStopped is an internal channel used to signal the stopping procedure that the
	// LeaderElection.Run(...) function has returned and the shutdown can proceed.
	LeaderElectionStopped chan struct{}

	// LeaderElectionCancel is used to cancel the leader election. It is distinct from internalStopper,
	// because for safety reasons we need to os.Exit() when we lose the leader election, meaning that
	// it must be deferred until after gracefulShutdown is done.
	LeaderElectionCancel context.CancelFunc

	// ElectedChan is closed when this manager becomes the leader of a group of
	// managers, either because it won a leader election or because no leader
	// election was configured.
	ElectedChan chan struct{}

	// Port is the Port that the webhook server serves at.
	Port int
	// Host is the hostname that the webhook server binds to.
	Host string
	// CertDir is the directory that contains the server key and certificate.
	// if not set, webhook server would look up the server key and certificate in
	// {TempDir}/k8s-webhook-server/serving-certs
	CertDir string
	// TLSOpts is used to allow configuring the TLS config used for the webhook server.
	TLSOpts []func(*tls.Config)

	WebhookServer *webhook.Server
	// webhookServerOnce will be called in GetWebhookServer() to optionally initialize
	// webhookServer if unset, and Add() it to controllerManager.
	webhookServerOnce sync.Once

	// LeaderElectionID is the name of the resource that leader election
	// will use for holding the leader lock.
	LeaderElectionID string
	// LeaseDuration is the duration that non-leader candidates will
	// wait to force acquire leadership.
	LeaseDuration time.Duration
	// RenewDeadline is the duration that the acting controlplane will retry
	// refreshing leadership before giving up.
	RenewDeadline time.Duration
	// RetryPeriod is the duration the LeaderElector clients should wait
	// between tries of actions.
	RetryPeriod time.Duration

	// GracefulShutdownTimeout is the duration given to runnable to stop
	// before the manager actually returns on stop.
	GracefulShutdownTimeout time.Duration

	// OnStoppedLeading is callled when the leader election lease is lost.
	// It can be overridden for tests.
	OnStoppedLeading func()

	// shutdownCtx is the context that can be used during shutdown. It will be cancelled
	// after the gracefulShutdownTimeout ended. It must not be accessed before internalStop
	// is closed because it will be nil.
	shutdownCtx context.Context

	internalCtx    context.Context
	internalCancel context.CancelFunc

	// InternalProceduresStop channel is used internally to the manager when coordinating
	// the proper shutdown of servers. This channel is also used for dependency injection.
	InternalProceduresStop chan struct{}
}

type hasCache interface {
	api.Runnable
	GetCache() cache.Cache
}

// Add sets dependencies on i, and adds it to the list of Runnables to start.
func (cm *ControllerManager) Add(r api.Runnable) error {
	cm.Lock()
	defer cm.Unlock()
	return cm.add(r)
}

func (cm *ControllerManager) add(r api.Runnable) error {
	return cm.Runnables.Add(r)
}

// AddMetricsExtraHandler adds extra handler served on path to the http server that serves metrics.
func (cm *ControllerManager) AddMetricsExtraHandler(path string, handler http.Handler) error {
	cm.Lock()
	defer cm.Unlock()

	if cm.started {
		return fmt.Errorf("unable to add new metrics handler because metrics endpoint has already been created")
	}

	if path == DefaultMetricsEndpoint {
		return fmt.Errorf("overriding builtin %s endpoint is not allowed", DefaultMetricsEndpoint)
	}

	if _, found := cm.MetricsExtraHandlers[path]; found {
		return fmt.Errorf("can't register extra handler by duplicate path %q on metrics http server", path)
	}

	cm.MetricsExtraHandlers[path] = handler
	cm.Logger.V(2).Info("Registering metrics http server extra handler", "path", path)
	return nil
}

// AddHealthzCheck allows you to add Healthz checker.
func (cm *ControllerManager) AddHealthzCheck(name string, check healthz.Checker) error {
	cm.Lock()
	defer cm.Unlock()

	if cm.started {
		return fmt.Errorf("unable to add new checker because healthz endpoint has already been created")
	}

	if cm.HealthzHandler == nil {
		cm.HealthzHandler = &healthz.Handler{Checks: map[string]healthz.Checker{}}
	}

	cm.HealthzHandler.Checks[name] = check
	return nil
}

// AddReadyzCheck allows you to add Readyz checker.
func (cm *ControllerManager) AddReadyzCheck(name string, check healthz.Checker) error {
	cm.Lock()
	defer cm.Unlock()

	if cm.started {
		return fmt.Errorf("unable to add new checker because healthz endpoint has already been created")
	}

	if cm.ReadyzHandler == nil {
		cm.ReadyzHandler = &healthz.Handler{Checks: map[string]healthz.Checker{}}
	}

	cm.ReadyzHandler.Checks[name] = check
	return nil
}

// GetConfig returns the rest.Config used to talk to the apiserver.
func (cm *ControllerManager) GetConfig() *rest.Config {
	return cm.Cluster.GetConfig()
}

// GetClient returns a client configured with the Config.
func (cm *ControllerManager) GetClient() client.Client {
	return cm.Cluster.GetClient()
}

// GetScheme returns the scheme used by the manager.
func (cm *ControllerManager) GetScheme() *runtime.Scheme {
	return cm.Cluster.GetScheme()
}

// GetFieldIndexer returns the indexer used by the manager.
func (cm *ControllerManager) GetFieldIndexer() client.FieldIndexer {
	return cm.Cluster.GetFieldIndexer()
}

// GetCache returns the cache used by the manager.
func (cm *ControllerManager) GetCache() cache.Cache {
	return cm.Cluster.GetCache()
}

// GetEventRecorderFor returns a new EventRecorder for the provided name.
func (cm *ControllerManager) GetEventRecorderFor(name string) record.EventRecorder {
	return cm.Cluster.GetEventRecorderFor(name)
}

// GetRESTMapper returns the RESTMapper used to map go types to Kubernetes.
func (cm *ControllerManager) GetRESTMapper() meta.RESTMapper {
	return cm.Cluster.GetRESTMapper()
}

// GetAPIReader returns a new API reader.
func (cm *ControllerManager) GetAPIReader() client.Reader {
	return cm.Cluster.GetAPIReader()
}

// GetWebhookServer returns the webhook server used by the manager.
func (cm *ControllerManager) GetWebhookServer() *webhook.Server {
	cm.webhookServerOnce.Do(func() {
		if cm.WebhookServer == nil {
			cm.WebhookServer = &webhook.Server{
				Port:    cm.Port,
				Host:    cm.Host,
				CertDir: cm.CertDir,
				TLSOpts: cm.TLSOpts,
			}
		}
		if err := cm.Add(cm.WebhookServer); err != nil {
			panic(fmt.Sprintf("unable to add webhook server to the controller manager: %s", err))
		}
	})
	return cm.WebhookServer
}

// GetLogger returns the logger used by the manager.
func (cm *ControllerManager) GetLogger() logr.Logger {
	return cm.Logger
}

// GetControllerOptions returns the controller options used by the manager.
func (cm *ControllerManager) GetControllerOptions() v1alpha1.ControllerConfigurationSpec {
	return cm.ControllerOptions
}

func (cm *ControllerManager) serveMetrics() {
	handler := promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{
		ErrorHandling: promhttp.HTTPErrorOnError,
	})
	// TODO(JoelSpeed): Use existing Kubernetes machinery for serving metrics
	mux := http.NewServeMux()
	mux.Handle(DefaultMetricsEndpoint, handler)
	for path, extraHandler := range cm.MetricsExtraHandlers {
		mux.Handle(path, extraHandler)
	}

	server := httpserver.New(mux)
	go cm.httpServe("metrics", cm.Logger.WithValues("path", DefaultMetricsEndpoint), server, cm.MetricsListener)
}

func (cm *ControllerManager) serveHealthProbes() {
	mux := http.NewServeMux()
	server := httpserver.New(mux)

	if cm.ReadyzHandler != nil {
		mux.Handle(cm.ReadinessEndpointName, http.StripPrefix(cm.ReadinessEndpointName, cm.ReadyzHandler))
		// Append '/' suffix to handle subpaths
		mux.Handle(cm.ReadinessEndpointName+"/", http.StripPrefix(cm.ReadinessEndpointName, cm.ReadyzHandler))
	}
	if cm.HealthzHandler != nil {
		mux.Handle(cm.LivenessEndpointName, http.StripPrefix(cm.LivenessEndpointName, cm.HealthzHandler))
		// Append '/' suffix to handle subpaths
		mux.Handle(cm.LivenessEndpointName+"/", http.StripPrefix(cm.LivenessEndpointName, cm.HealthzHandler))
	}

	go cm.httpServe("health probe", cm.Logger, server, cm.HealthProbeListener)
}

func (cm *ControllerManager) httpServe(kind string, log logr.Logger, server *http.Server, ln net.Listener) {
	log = log.WithValues("kind", kind, "addr", ln.Addr())

	go func() {
		log.Info("Starting server")
		if err := server.Serve(ln); err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				return
			}
			if atomic.LoadInt64(cm.StopProcedureEngaged) > 0 {
				// There might be cases where connections are still open and we try to shutdown
				// but not having enough time to close the connection causes an error in Serve
				//
				// In that case we want to avoid returning an error to the main error channel.
				log.Error(err, "error on Serve after stop has been engaged")
				return
			}
			cm.ErrChan <- err
		}
	}()

	// Shutdown the server when stop is closed.
	<-cm.InternalProceduresStop
	if err := server.Shutdown(cm.shutdownCtx); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			// Avoid logging context related errors.
			return
		}
		if atomic.LoadInt64(cm.StopProcedureEngaged) > 0 {
			cm.Logger.Error(err, "error on Shutdown after stop has been engaged")
			return
		}
		cm.ErrChan <- err
	}
}

// Start starts the manager and waits indefinitely.
// There is only two ways to have start return:
// An error has occurred during in one of the internal operations,
// such as leader election, cache start, webhooks, and so on.
// Or, the context is cancelled.
func (cm *ControllerManager) Start(ctx context.Context) (err error) {
	cm.Lock()
	if cm.started {
		cm.Unlock()
		return errors.New("manager already started")
	}
	cm.started = true

	var ready bool
	defer func() {
		// Only unlock the manager if we haven't reached
		// the internal readiness condition.
		if !ready {
			cm.Unlock()
		}
	}()

	// Initialize the internal context.
	cm.internalCtx, cm.internalCancel = context.WithCancel(ctx)

	// This chan indicates that stop is complete, in other words all runnables have returned or timeout on stop request
	stopComplete := make(chan struct{})
	defer close(stopComplete)
	// This must be deferred after closing stopComplete, otherwise we deadlock.
	defer func() {
		// https://hips.hearstapps.com/hmg-prod.s3.amazonaws.com/images/gettyimages-459889618-1533579787.jpg
		stopErr := cm.engageStopProcedure(stopComplete)
		if stopErr != nil {
			if err != nil {
				// Utilerrors.Aggregate allows to use errors.Is for all contained errors
				// whereas fmt.Errorf allows wrapping at most one error which means the
				// other one can not be found anymore.
				err = kerrors.NewAggregate([]error{err, stopErr})
			} else {
				err = stopErr
			}
		}
	}()

	// Add the cluster runnable.
	if err := cm.add(cm.Cluster); err != nil {
		return fmt.Errorf("failed to add cluster to runnables: %w", err)
	}

	// Metrics should be served whether the controller is leader or not.
	// (If we don't serve metrics for non-leaders, prometheus will still scrape
	// the pod but will get a connection refused).
	if cm.MetricsListener != nil {
		cm.serveMetrics()
	}

	// Serve health probes.
	if cm.HealthProbeListener != nil {
		cm.serveHealthProbes()
	}

	// First start any webhook servers, which includes conversion, validation, and defaulting
	// webhooks that are registered.
	//
	// WARNING: Webhooks MUST start before any cache is populated, otherwise there is a race condition
	// between conversion webhooks and the cache sync (usually initial list) which causes the webhooks
	// to never start because no cache can be populated.
	if err := cm.Runnables.Webhooks.Start(cm.internalCtx); err != nil {
		if !errors.Is(err, wait.ErrWaitTimeout) {
			return err
		}
	}

	// Start and wait for caches.
	if err := cm.Runnables.Caches.Start(cm.internalCtx); err != nil {
		if !errors.Is(err, wait.ErrWaitTimeout) {
			return err
		}
	}

	// Start the non-leaderelection Runnables after the cache has synced.
	if err := cm.Runnables.Others.Start(cm.internalCtx); err != nil {
		if !errors.Is(err, wait.ErrWaitTimeout) {
			return err
		}
	}

	// Start the leader election and all required runnables.
	{
		ctx, cancel := context.WithCancel(context.Background())
		cm.LeaderElectionCancel = cancel
		go func() {
			if cm.ResourceLock != nil {
				if err := cm.startLeaderElection(ctx); err != nil {
					cm.ErrChan <- err
				}
			} else {
				// Treat not having leader election enabled the same as being elected.
				if err := cm.startLeaderElectionRunnables(); err != nil {
					cm.ErrChan <- err
				}
				close(cm.ElectedChan)
			}
		}()
	}

	ready = true
	cm.Unlock()
	select {
	case <-ctx.Done():
		// We are done
		return nil
	case err := <-cm.ErrChan:
		// Error starting or running a runnable
		return err
	}
}

// engageStopProcedure signals all runnables to stop, reads potential errors
// from the errChan and waits for them to end. It must not be called more than once.
func (cm *ControllerManager) engageStopProcedure(stopComplete <-chan struct{}) error {
	if !atomic.CompareAndSwapInt64(cm.StopProcedureEngaged, 0, 1) {
		return errors.New("stop procedure already engaged")
	}

	// Populate the shutdown context, this operation MUST be done before
	// closing the internalProceduresStop channel.
	//
	// The shutdown context immediately expires if the gracefulShutdownTimeout is not set.
	var shutdownCancel context.CancelFunc
	cm.shutdownCtx, shutdownCancel = context.WithTimeout(context.Background(), cm.GracefulShutdownTimeout)
	defer shutdownCancel()

	// Start draining the errors before acquiring the lock to make sure we don't deadlock
	// if something that has the lock is blocked on trying to write into the unbuffered
	// channel after something else already wrote into it.
	var closeOnce sync.Once
	go func() {
		for {
			// Closing in the for loop is required to avoid race conditions between
			// the closure of all internal procedures and making sure to have a reader off the error channel.
			closeOnce.Do(func() {
				// Cancel the internal stop channel and wait for the procedures to stop and complete.
				close(cm.InternalProceduresStop)
				cm.internalCancel()
			})
			select {
			case err, ok := <-cm.ErrChan:
				if ok {
					cm.Logger.Error(err, "error received after stop sequence was engaged")
				}
			case <-stopComplete:
				return
			}
		}
	}()

	// We want to close this after the other runnables stop, because we don't
	// want things like leader election to try and emit events on a closed
	// channel
	defer cm.RecorderProvider.Stop(cm.shutdownCtx)
	defer func() {
		// Cancel leader election only after we waited. It will os.Exit() the app for safety.
		if cm.ResourceLock != nil {
			// After asking the context to be cancelled, make sure
			// we wait for the leader stopped channel to be closed, otherwise
			// we might encounter race conditions between this code
			// and the event recorder, which is used within leader election code.
			cm.LeaderElectionCancel()
			<-cm.LeaderElectionStopped
		}
	}()

	go func() {
		// First stop the non-leader election runnables.
		cm.Logger.Info("Stopping and waiting for non leader election runnables")
		cm.Runnables.Others.StopAndWait(cm.shutdownCtx)

		// Stop all the leader election runnables, which includes reconcilers.
		cm.Logger.Info("Stopping and waiting for leader election runnables")
		cm.Runnables.LeaderElection.StopAndWait(cm.shutdownCtx)

		// Stop the caches before the leader election runnables, this is an important
		// step to make sure that we don't race with the reconcilers by receiving more events
		// from the API servers and enqueueing them.
		cm.Logger.Info("Stopping and waiting for caches")
		cm.Runnables.Caches.StopAndWait(cm.shutdownCtx)

		// Webhooks should come last, as they might be still serving some requests.
		cm.Logger.Info("Stopping and waiting for webhooks")
		cm.Runnables.Webhooks.StopAndWait(cm.shutdownCtx)

		// Proceed to close the manager and overall shutdown context.
		cm.Logger.Info("Wait completed, proceeding to shutdown the manager")
		shutdownCancel()
	}()

	<-cm.shutdownCtx.Done()
	if err := cm.shutdownCtx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		if errors.Is(err, context.DeadlineExceeded) {
			if cm.GracefulShutdownTimeout > 0 {
				return fmt.Errorf("failed waiting for all runnables to end within grace period of %s: %w", cm.GracefulShutdownTimeout, err)
			}
			return nil
		}
		// For any other error, return the error.
		return err
	}

	return nil
}

func (cm *ControllerManager) startLeaderElectionRunnables() error {
	return cm.Runnables.LeaderElection.Start(cm.internalCtx)
}

func (cm *ControllerManager) startLeaderElection(ctx context.Context) (err error) {
	l, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:          cm.ResourceLock,
		LeaseDuration: cm.LeaseDuration,
		RenewDeadline: cm.RenewDeadline,
		RetryPeriod:   cm.RetryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(_ context.Context) {
				if err := cm.startLeaderElectionRunnables(); err != nil {
					cm.ErrChan <- err
					return
				}
				close(cm.ElectedChan)
			},
			OnStoppedLeading: func() {
				if cm.OnStoppedLeading != nil {
					cm.OnStoppedLeading()
				}
				// Make sure graceful shutdown is skipped if we lost the leader lock without
				// intending to.
				cm.GracefulShutdownTimeout = time.Duration(0)
				// Most implementations of leader election log.Fatal() here.
				// Since Start is wrapped in log.Fatal when called, we can just return
				// an error here which will cause the program to exit.
				cm.ErrChan <- errors.New("leader election lost")
			},
		},
		ReleaseOnCancel: cm.LeaderElectionReleaseOnCancel,
		Name:            cm.LeaderElectionID,
	})
	if err != nil {
		return err
	}

	// Start the leader elector process
	go func() {
		l.Run(ctx)
		<-ctx.Done()
		close(cm.LeaderElectionStopped)
	}()
	return nil
}

// Elected returns a channel that is closed when this manager is elected leader.
func (cm *ControllerManager) Elected() <-chan struct{} {
	return cm.ElectedChan
}
