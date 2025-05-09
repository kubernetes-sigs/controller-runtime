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

package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	internalmetrics "sigs.k8s.io/controller-runtime/pkg/internal/metrics"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// reconcileTotal is a prometheus counter metrics which holds the total
	// number of reconciliations per controller. It has two labels. controller label refers
	// to the controller name and result label refers to the reconcile result i.e
	// success, error, requeue, requeue_after.
	reconcileTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "controller_runtime_reconcile_total",
		Help: "Total number of reconciliations per controller",
	}, []string{"controller", "result"})

	// reconcileErrors is a prometheus counter metrics which holds the total
	// number of errors from the Reconciler.
	reconcileErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "controller_runtime_reconcile_errors_total",
		Help: "Total number of reconciliation errors per controller",
	}, []string{"controller"})

	// terminalReconcileErrors is a prometheus counter metrics which holds the total
	// number of terminal errors from the Reconciler.
	terminalReconcileErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "controller_runtime_terminal_reconcile_errors_total",
		Help: "Total number of terminal reconciliation errors per controller",
	}, []string{"controller"})

	// reconcilePanics is a prometheus counter metrics which holds the total
	// number of panics from the Reconciler.
	reconcilePanics = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "controller_runtime_reconcile_panics_total",
		Help: "Total number of reconciliation panics per controller",
	}, []string{"controller"})

	// reconcileTime is a prometheus metric which keeps track of the duration
	// of reconciliations.
	reconcileTime = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "controller_runtime_reconcile_time_seconds",
		Help: "Length of time per reconciliation per controller",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0,
			1.25, 1.5, 1.75, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5, 5, 6, 7, 8, 9, 10, 15, 20, 25, 30, 40, 50, 60},
		NativeHistogramBucketFactor:     1.1,
		NativeHistogramMaxBucketNumber:  100,
		NativeHistogramMinResetDuration: 1 * time.Hour,
	}, []string{"controller"})

	// workerCount is a prometheus metric which holds the number of
	// concurrent reconciles per controller.
	workerCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "controller_runtime_max_concurrent_reconciles",
		Help: "Maximum number of concurrent reconciles per controller",
	}, []string{"controller"})

	// activeWorkers is a prometheus metric which holds the number
	// of active workers per controller.
	activeWorkers = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "controller_runtime_active_workers",
		Help: "Number of currently used workers per controller",
	}, []string{"controller"})
)

// ControllerMetricsProvider is an interface that provides methods for firing controller metrics
type ControllerMetricsProvider interface {
	// ReconcileTotal is a prometheus counter metrics which holds the total
	// number of reconciliations per controller. It has two labels. controller label refers
	// to the controller name and result label refers to the reconcile result i.e
	// success, error, requeue, requeue_after.
	ReconcileTotal() internalmetrics.CounterMetric
	// ReconcileErrors is a prometheus counter metrics which holds the total
	// number of errors from the Reconciler.
	ReconcileErrors() internalmetrics.CounterMetric
	// TerminalReconcileErrors is a prometheus counter metrics which holds the total
	// number of terminal errors from the Reconciler.
	TerminalReconcileErrors() internalmetrics.CounterMetric
	// ReconcilePanics is a prometheus counter metrics which holds the total
	// number of panics from the Reconciler.
	ReconcilePanics() internalmetrics.CounterMetric
	// ReconcileTime is a prometheus metric which keeps track of the duration
	// of reconciliations.
	ReconcileTime() internalmetrics.HistogramMetric
	// WorkerCount is a prometheus metric which holds the number of
	// concurrent reconciles per controller.
	WorkerCount() internalmetrics.GaugeMetric
	// ActiveWorkers is a prometheus metric which holds the number
	// of active workers per controller.
	ActiveWorkers() internalmetrics.GaugeMetric
}

// PrometheusProvider is a metrics.ControllerMetricsProvider and a metrics.LeaderElectionMetricsProvider
// that registers and fires prometheus metrics in response to leader election and controller events
type PrometheusProvider struct {
	reconcileTotal          *prometheus.CounterVec
	reconcileErrors         *prometheus.CounterVec
	terminalReconcileErrors *prometheus.CounterVec
	reconcilePanics         *prometheus.CounterVec
	reconcileTime           *prometheus.HistogramVec
	workerCount             *prometheus.GaugeVec
	activeWorkers           *prometheus.GaugeVec
}

// NewPrometheusProvider creates a PrometheusProvider
func NewPrometheusProvider() *PrometheusProvider {
	return &PrometheusProvider{
		reconcileTotal:          reconcileTotal,
		reconcileErrors:         reconcileErrors,
		terminalReconcileErrors: terminalReconcileErrors,
		reconcilePanics:         reconcilePanics,
		reconcileTime:           reconcileTime,
		workerCount:             workerCount,
		activeWorkers:           activeWorkers,
	}
}

// ReconcileTotal returns a Prometheus counter that fulfills the CounterMetric interface
func (p PrometheusProvider) ReconcileTotal() internalmetrics.CounterMetric {
	return &internalmetrics.PrometheusCounterAdapter{CounterVec: p.reconcileTotal}
}

// ReconcileErrors returns a Prometheus counter that fulfills the CounterMetric interface
func (p PrometheusProvider) ReconcileErrors() internalmetrics.CounterMetric {
	return &internalmetrics.PrometheusCounterAdapter{CounterVec: p.reconcileErrors}
}

// TerminalReconcileErrors returns a Prometheus counter that fulfills the CounterMetric interface
func (p PrometheusProvider) TerminalReconcileErrors() internalmetrics.CounterMetric {
	return &internalmetrics.PrometheusCounterAdapter{CounterVec: p.terminalReconcileErrors}
}

// ReconcilePanics returns a Prometheus counter that fulfills the CounterMetric interface
func (p PrometheusProvider) ReconcilePanics() internalmetrics.CounterMetric {
	return &internalmetrics.PrometheusCounterAdapter{CounterVec: p.reconcilePanics}
}

// ReconcileTime returns a Prometheus histogram that fulfills the ObservationMetric interface
func (p PrometheusProvider) ReconcileTime() internalmetrics.HistogramMetric {
	return &internalmetrics.PrometheusHistogramAdapter{HistogramVec: p.reconcileTime}
}

// WorkerCount returns a Prometheus gauge that fulfills the GaugeMetric interface
func (p PrometheusProvider) WorkerCount() internalmetrics.GaugeMetric {
	return &internalmetrics.PrometheusGaugeAdapter{GaugeVec: p.workerCount}
}

// ActiveWorkers returns a Prometheus gauge that fulfills the GaugeMetric interface
func (p PrometheusProvider) ActiveWorkers() internalmetrics.GaugeMetric {
	return &internalmetrics.PrometheusGaugeAdapter{GaugeVec: p.activeWorkers}
}

func init() {
	metrics.Registry.MustRegister(
		reconcileTotal,
		reconcileErrors,
		terminalReconcileErrors,
		reconcilePanics,
		reconcileTime,
		workerCount,
		activeWorkers,
	)
}

var controllerMetricsProvider ControllerMetricsProvider = NewPrometheusProvider()

// SetControllerMetricsProvider assigns a provider to the ControllerMetricsProvider for exposing controller metrics.
// The PrometheusProvider will be used by default if the provider is not overridden
func SetControllerMetricsProvider(provider ControllerMetricsProvider) {
	controllerMetricsProvider = provider
}

// GetControllerMetricsProvider returns the controller metrics provider being used by the controller reconciliation
func GetControllerMetricsProvider() ControllerMetricsProvider {
	return controllerMetricsProvider
}
