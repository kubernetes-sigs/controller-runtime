package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/internal/controller/metrics"
)

// LeaderElectionMetricsProvider is an interface that provides methods for firing leader election metrics
type LeaderElectionMetricsProvider interface {
	LeaderGauge() GaugeMetric
	SlowpathExercised() CounterMetric
}

// ControllerMetricsProvider is an interface that provides methods for firing controller metrics
type ControllerMetricsProvider interface {
	// ReconcileTotal is a prometheus counter metrics which holds the total
	// number of reconciliations per controller. It has two labels. controller label refers
	// to the controller name and result label refers to the reconcile result i.e
	// success, error, requeue, requeue_after.
	ReconcileTotal() CounterMetric
	// ReconcileErrors is a prometheus counter metrics which holds the total
	// number of errors from the Reconciler.
	ReconcileErrors() CounterMetric
	// TerminalReconcileErrors is a prometheus counter metrics which holds the total
	// number of terminal errors from the Reconciler.
	TerminalReconcileErrors() CounterMetric
	// ReconcilePanics is a prometheus counter metrics which holds the total
	// number of panics from the Reconciler.
	ReconcilePanics() CounterMetric
	// ReconcileTime is a prometheus metric which keeps track of the duration
	// of reconciliations.
	ReconcileTime() ObservationMetric
	// WorkerCount is a prometheus metric which holds the number of
	// concurrent reconciles per controller.
	WorkerCount() GaugeMetric
	// ActiveWorkers is a prometheus metric which holds the number
	// of active workers per controller.
	ActiveWorkers() GaugeMetric
}

// ObservationMetric is a metric that stores the set of observed values
type ObservationMetric interface {
	Observe(map[string]string, float64)
}

// GaugeMetric is a metric that gets set and can be changed dynamically at runtime
type GaugeMetric interface {
	Set(map[string]string, float64)
	Add(map[string]string, float64)
}

// CounterMetric is a metric that gets incremented monotonically
type CounterMetric interface {
	Inc(map[string]string)
	Add(map[string]string, float64)
}

var once sync.Once

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
	leaderGauge             *prometheus.GaugeVec
	leaderSlowpathCounter   *prometheus.CounterVec
}

// NewPrometheusProvider creates a PrometheusProvider
func NewPrometheusProvider() *PrometheusProvider {
	leaderGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "leader_election_master_status",
		Help: "Gauge of if the reporting system is master of the relevant lease, 0 indicates backup, 1 indicates master. 'name' is the string used to identify the lease. Please make sure to group by name.",
	}, []string{"name"})
	leaderSlowpathCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "leader_election_slowpath_total",
		Help: "Total number of slow path exercised in renewing leader leases. 'name' is the string used to identify the lease. Please make sure to group by name.",
	}, []string{"name"})
	once.Do(func() {
		Registry.MustRegister(
			metrics.ReconcileTotal,
			metrics.ReconcileErrors,
			metrics.TerminalReconcileErrors,
			metrics.ReconcilePanics,
			metrics.ReconcileTime,
			metrics.WorkerCount,
			metrics.ActiveWorkers,
			leaderGauge,
			leaderSlowpathCounter,
		)
	})
	return &PrometheusProvider{
		reconcileTotal:          metrics.ReconcileTotal,
		reconcileErrors:         metrics.ReconcileErrors,
		terminalReconcileErrors: metrics.TerminalReconcileErrors,
		reconcilePanics:         metrics.ReconcilePanics,
		reconcileTime:           metrics.ReconcileTime,
		workerCount:             metrics.WorkerCount,
		activeWorkers:           metrics.ActiveWorkers,
		leaderGauge:             leaderGauge,
		leaderSlowpathCounter:   leaderSlowpathCounter,
	}
}

type prometheusCounterAdapter struct {
	*prometheus.CounterVec
}

func (p *prometheusCounterAdapter) Inc(labels map[string]string) {
	p.With(labels).Inc()
}

func (p *prometheusCounterAdapter) Add(labels map[string]string, val float64) {
	p.With(labels).Add(val)
}

type prometheusGaugeAdapter struct {
	*prometheus.GaugeVec
}

func (p *prometheusGaugeAdapter) Set(labels map[string]string, val float64) {
	p.With(labels).Set(val)
}

func (p *prometheusGaugeAdapter) Add(labels map[string]string, val float64) {
	p.With(labels).Add(val)
}

type prometheusHistogramAdapter struct {
	*prometheus.HistogramVec
}

func (p *prometheusHistogramAdapter) Observe(labels map[string]string, val float64) {
	p.With(labels).Observe(val)
}

// ReconcileTotal returns a Prometheus counter that fulfills the CounterMetric interface
func (p PrometheusProvider) ReconcileTotal() CounterMetric {
	return &prometheusCounterAdapter{CounterVec: p.reconcileTotal}
}

// ReconcileErrors returns a Prometheus counter that fulfills the CounterMetric interface
func (p PrometheusProvider) ReconcileErrors() CounterMetric {
	return &prometheusCounterAdapter{CounterVec: p.reconcileErrors}
}

// TerminalReconcileErrors returns a Prometheus counter that fulfills the CounterMetric interface
func (p PrometheusProvider) TerminalReconcileErrors() CounterMetric {
	return &prometheusCounterAdapter{CounterVec: p.terminalReconcileErrors}
}

// ReconcilePanics returns a Prometheus counter that fulfills the CounterMetric interface
func (p PrometheusProvider) ReconcilePanics() CounterMetric {
	return &prometheusCounterAdapter{CounterVec: p.reconcilePanics}
}

// ReconcileTime returns a Prometheus histogram that fulfills the ObservationMetric interface
func (p PrometheusProvider) ReconcileTime() ObservationMetric {
	return &prometheusHistogramAdapter{HistogramVec: p.reconcileTime}
}

// WorkerCount returns a Prometheus gauge that fulfills the GaugeMetric interface
func (p PrometheusProvider) WorkerCount() GaugeMetric {
	return &prometheusGaugeAdapter{GaugeVec: p.workerCount}
}

// ActiveWorkers returns a Prometheus gauge that fulfills the GaugeMetric interface
func (p PrometheusProvider) ActiveWorkers() GaugeMetric {
	return &prometheusGaugeAdapter{GaugeVec: p.activeWorkers}
}

// LeaderGauge returns a Prometheus gauge that fulfills the GaugeMetric interface
func (p PrometheusProvider) LeaderGauge() GaugeMetric {
	return &prometheusGaugeAdapter{GaugeVec: p.leaderGauge}
}

// SlowpathExercised returns a Prometheus counter that fulfills the CounterMetric interface
func (p PrometheusProvider) SlowpathExercised() CounterMetric {
	return &prometheusCounterAdapter{CounterVec: p.leaderSlowpathCounter}
}
