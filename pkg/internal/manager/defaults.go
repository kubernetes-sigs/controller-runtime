package manager

import "time"

const (
	// Values taken from: https://github.com/kubernetes/component-base/blob/master/config/v1alpha1/defaults.go

	// DefaultLeaseDuration is the duration that non-leader candidates will wait
	// after observing a leadership renewal until attempting to acquire
	// leadership of a led but unrenewed leader slot. This is effectively the
	// maximum duration that a leader can be stopped before it is replaced
	// by another candidate. This is only applicable if leader election is
	// enabled.
	DefaultLeaseDuration = 15 * time.Second

	// DefaultRenewDeadline irenewDeadline is the interval between attempts by the acting master to
	// renew a leadership slot before it stops leading. This must be less
	// than or equal to the lease duration. This is only applicable if leader
	// election is enabled.
	DefaultRenewDeadline = 10 * time.Second

	// DefaultRetryPeriod is the duration the clients should wait between attempting
	// acquisition and renewal of a leadership. This is only applicable if
	// leader election is enabled.
	DefaultRetryPeriod = 2 * time.Second

	// DefaultGracefulShutdownPeriod is the duration the manager will wait for
	// active reconciliation loops to gracefully shutdown.
	DefaultGracefulShutdownPeriod = 30 * time.Second

	// DefaultReadinessEndpoint is the default endpoint for readiness probes.
	DefaultReadinessEndpoint = "/readyz"

	// DefaultLivenessEndpoint is the default endpoint for liveness probes.
	DefaultLivenessEndpoint = "/healthz"

	// DefaultMetricsEndpoint is the default endpoint for metrics.
	DefaultMetricsEndpoint = "/metrics"
)
