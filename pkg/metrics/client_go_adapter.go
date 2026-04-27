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
	"context"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	clientmetrics "k8s.io/client-go/tools/metrics"
)

// this file contains setup logic to initialize the myriad of places
// that client-go registers metrics.  We copy the names and formats
// from Kubernetes so that we match the core controllers.

const (
	hostLabel = "host"
	verbLabel = "verb"
)

var (
	// client metrics.

	requestResult = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rest_client_requests_total",
			Help: "Number of HTTP requests, partitioned by status code, method, and host.",
		},
		[]string{"code", "method", hostLabel},
	)

	requestLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:                            "rest_client_request_duration_seconds",
			Help:                            "Request latency in seconds. Broken down by verb and host.",
			Buckets:                         []float64{0.005, 0.025, 0.1, 0.25, 0.5, 1.0, 2.0, 4.0, 8.0, 15.0, 30.0, 60.0},
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		},
		[]string{verbLabel, hostLabel},
	)

	resolverLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:                            "rest_client_dns_resolution_duration_seconds",
			Help:                            "DNS resolver latency in seconds. Broken down by host.",
			Buckets:                         []float64{0.005, 0.025, 0.1, 0.25, 0.5, 1.0, 2.0, 4.0, 8.0, 15.0, 30.0, 60.0},
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		},
		[]string{hostLabel},
	)

	requestSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "rest_client_request_size_bytes",
			Help: "Request size in bytes. Broken down by verb and host.",
			// 64 bytes to 16MB
			Buckets:                         []float64{64, 256, 512, 1024, 4096, 16384, 65536, 262144, 1048576, 4194304, 16777216},
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		},
		[]string{verbLabel, hostLabel},
	)

	responseSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "rest_client_response_size_bytes",
			Help: "Response size in bytes. Broken down by verb and host.",
			// 64 bytes to 16MB
			Buckets:                         []float64{64, 256, 512, 1024, 4096, 16384, 65536, 262144, 1048576, 4194304, 16777216},
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		},
		[]string{verbLabel, hostLabel},
	)

	rateLimiterLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:                            "rest_client_rate_limiter_duration_seconds",
			Help:                            "Client side rate limiter latency in seconds. Broken down by verb, and host.",
			Buckets:                         []float64{0.005, 0.025, 0.1, 0.25, 0.5, 1.0, 2.0, 4.0, 8.0, 15.0, 30.0, 60.0},
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		},
		[]string{verbLabel, hostLabel},
	)

	requestRetry = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rest_client_request_retries_total",
			Help: "Number of request retries, partitioned by status code, verb, and host.",
		},
		[]string{"code", verbLabel, hostLabel},
	)
)

// RESTClientMetric identifies an opt-in client-go REST client metric.
// Pass values to RegisterRESTClientMetrics to enable a subset.
type RESTClientMetric int

const (
	// MetricRequestLatency enables the rest_client_request_duration_seconds metric.
	MetricRequestLatency = iota + 1
	// MetricResolverLatency enables the rest_client_dns_resolution_duration_seconds metric.
	MetricResolverLatency
	// MetricRequestSize enables the rest_client_request_size_bytes metric.
	MetricRequestSize
	// MetricResponseSize enables the rest_client_response_size_bytes metric.
	MetricResponseSize
	// MetricRateLimiterLatency enables the rest_client_rate_limiter_duration_seconds metric.
	MetricRateLimiterLatency
	// MetricRequestRetry enables the rest_client_request_retries_total metric.
	MetricRequestRetry
)

// Per-metric gates.
var (
	requestLatencyEnabled     atomic.Bool
	resolverLatencyEnabled    atomic.Bool
	requestSizeEnabled        atomic.Bool
	responseSizeEnabled       atomic.Bool
	rateLimiterLatencyEnabled atomic.Bool
	requestRetryEnabled       atomic.Bool
	requestLatencyOnce        sync.Once
	resolverLatencyOnce       sync.Once
	requestSizeOnce           sync.Once
	responseSizeOnce          sync.Once
	rateLimiterLatencyOnce    sync.Once
	requestRetryOnce          sync.Once
)

func init() {
	registerClientMetrics()
}

// registerClientMetrics sets up the client latency metrics from client-go.
func registerClientMetrics() {
	// register the metrics with our registry
	Registry.MustRegister(requestResult)

	// register the metrics with client-go
	clientmetrics.Register(clientmetrics.RegisterOpts{
		RequestResult:      &resultAdapter{metric: requestResult},
		RequestLatency:     &latencyAdapter{metric: requestLatency, enabled: &requestLatencyEnabled},
		ResolverLatency:    &resolverLatencyAdapter{metric: resolverLatency, enabled: &resolverLatencyEnabled},
		RequestSize:        &sizeAdapter{metric: requestSize, enabled: &requestSizeEnabled},
		ResponseSize:       &sizeAdapter{metric: responseSize, enabled: &responseSizeEnabled},
		RateLimiterLatency: &latencyAdapter{metric: rateLimiterLatency, enabled: &rateLimiterLatencyEnabled},
		RequestRetry:       &retryAdapter{metric: requestRetry, enabled: &requestRetryEnabled},
	})
}

// RegisterRESTClientMetrics enables the client metrics.
func RegisterRESTClientMetrics(metrics ...RESTClientMetric) {
	seen := map[RESTClientMetric]bool{}
	for _, m := range metrics {
		if seen[m] {
			continue
		}
		seen[m] = true
		switch m {
		case MetricRequestLatency:
			requestLatencyOnce.Do(func() {
				requestLatencyEnabled.Store(true)
				Registry.MustRegister(requestLatency)
			})
		case MetricResolverLatency:
			resolverLatencyOnce.Do(func() {
				resolverLatencyEnabled.Store(true)
				Registry.MustRegister(resolverLatency)
			})
		case MetricRequestSize:
			requestSizeOnce.Do(func() {
				requestSizeEnabled.Store(true)
				Registry.MustRegister(requestSize)
			})
		case MetricResponseSize:
			responseSizeOnce.Do(func() {
				responseSizeEnabled.Store(true)
				Registry.MustRegister(responseSize)
			})
		case MetricRateLimiterLatency:
			rateLimiterLatencyOnce.Do(func() {
				rateLimiterLatencyEnabled.Store(true)
				Registry.MustRegister(rateLimiterLatency)
			})
		case MetricRequestRetry:
			requestRetryOnce.Do(func() {
				requestRetryEnabled.Store(true)
				Registry.MustRegister(requestRetry)
			})
		default:
			// unknown metric, ignore
		}
	}
}

// this section contains adapters, implementations, and other sundry organic, artisanally
// hand-crafted syntax trees required to convince client-go that it actually wants to let
// someone use its metrics.

// Client metrics adapters (method #1 for client-go metrics),
// copied (more-or-less directly) from k8s.io/kubernetes setup code
// (which isn't anywhere in an easily-importable place).

type resultAdapter struct {
	metric *prometheus.CounterVec
}

func (r *resultAdapter) Increment(_ context.Context, code, method, host string) {
	r.metric.WithLabelValues(code, method, host).Inc()
}

type latencyAdapter struct {
	metric  *prometheus.HistogramVec
	enabled *atomic.Bool
}

func (l *latencyAdapter) Observe(_ context.Context, verb string, u url.URL, duration time.Duration) {
	if !l.enabled.Load() {
		return
	}
	l.metric.WithLabelValues(verb, u.Host).Observe(duration.Seconds())
}

type resolverLatencyAdapter struct {
	metric  *prometheus.HistogramVec
	enabled *atomic.Bool
}

func (r *resolverLatencyAdapter) Observe(_ context.Context, host string, duration time.Duration) {
	if !r.enabled.Load() {
		return
	}
	r.metric.WithLabelValues(host).Observe(duration.Seconds())
}

type sizeAdapter struct {
	metric  *prometheus.HistogramVec
	enabled *atomic.Bool
}

func (r *sizeAdapter) Observe(_ context.Context, verb string, host string, size float64) {
	if !r.enabled.Load() {
		return
	}
	r.metric.WithLabelValues(verb, host).Observe(size)
}

type retryAdapter struct {
	metric  *prometheus.CounterVec
	enabled *atomic.Bool
}

func (r *retryAdapter) IncrementRetry(_ context.Context, code, verb, host string) {
	if !r.enabled.Load() {
		return
	}
	r.metric.WithLabelValues(code, verb, host).Inc()
}
