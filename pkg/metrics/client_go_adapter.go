/*
Copyright 2025 The Kubernetes Authors.

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
	"time"

	"github.com/prometheus/client_golang/prometheus"
	clientmetrics "k8s.io/client-go/tools/metrics"
)

// this file contains setup logic to initialize the myriad of places
// that client-go registers metrics.  We copy the names and formats
// from Kubernetes so that we match the core controllers.

var (
	// client metrics from https://github.com/kubernetes/kubernetes/blob/v1.33.0/staging/src/k8s.io/component-base/metrics/prometheus/restclient/metrics.go
	// except for rest_client_exec_plugin_* metrics which controllers wouldn't use

	// requestLatency is a Prometheus Histogram metric type partitioned by
	// "verb", and "host" labels. It is used for the rest client latency metrics.
	requestLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rest_client_request_duration_seconds",
			Help:    "Request latency in seconds. Broken down by verb, and host.",
			Buckets: []float64{0.005, 0.025, 0.1, 0.25, 0.5, 1.0, 2.0, 4.0, 8.0, 15.0, 30.0, 60.0},
		},
		[]string{"verb", "host"},
	)

	// resolverLatency is a Prometheus Histogram metric type partitioned by
	// "host" labels. It is used for the rest client DNS resolver latency metrics.
	resolverLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rest_client_dns_resolution_duration_seconds",
			Help:    "DNS resolver latency in seconds. Broken down by host.",
			Buckets: []float64{0.005, 0.025, 0.1, 0.25, 0.5, 1.0, 2.0, 4.0, 8.0, 15.0, 30.0},
		},
		[]string{"host"},
	)

	requestSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "rest_client_request_size_bytes",
			Help: "Request size in bytes. Broken down by verb and host.",
			// 64 bytes to 16MB
			Buckets: []float64{64, 256, 512, 1024, 4096, 16384, 65536, 262144, 1048576, 4194304, 16777216},
		},
		[]string{"verb", "host"},
	)

	responseSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "rest_client_response_size_bytes",
			Help: "Response size in bytes. Broken down by verb and host.",
			// 64 bytes to 16MB
			Buckets: []float64{64, 256, 512, 1024, 4096, 16384, 65536, 262144, 1048576, 4194304, 16777216},
		},
		[]string{"verb", "host"},
	)

	rateLimiterLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rest_client_rate_limiter_duration_seconds",
			Help:    "Client side rate limiter latency in seconds. Broken down by verb, and host.",
			Buckets: []float64{0.005, 0.025, 0.1, 0.25, 0.5, 1.0, 2.0, 4.0, 8.0, 15.0, 30.0, 60.0},
		},
		[]string{"verb", "host"},
	)

	requestResult = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rest_client_requests_total",
			Help: "Number of HTTP requests, partitioned by status code, method, and host.",
		},
		[]string{"code", "method", "host"},
	)

	requestRetry = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rest_client_request_retries_total",
			Help: "Number of request retries, partitioned by status code, verb, and host.",
		},
		[]string{"code", "verb", "host"},
	)

	transportCacheEntries = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "rest_client_transport_cache_entries",
			Help: "Number of transport entries in the internal cache.",
		},
	)

	transportCacheCalls = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rest_client_transport_create_calls_total",
			Help: "Number of calls to get a new transport, partitioned by the result of the operation " +
				"hit: obtained from the cache, miss: created and added to the cache, uncacheable: created and not cached",
		},
		[]string{"result"},
	)
)

func init() {
	registerClientMetrics()
}

// registerClientMetrics sets up the client latency metrics from client-go.
func registerClientMetrics() {
	// register the metrics with our registry
	Registry.MustRegister(requestResult,
		requestLatency,
		resolverLatency,
		requestSize,
		responseSize,
		rateLimiterLatency,
		requestRetry,
		transportCacheEntries,
		transportCacheCalls,
	)

	// register the metrics with client-go
	clientmetrics.Register(clientmetrics.RegisterOpts{
		RequestResult:         &requestResultAdapter{metric: requestResult},
		RequestLatency:        &requestLatencyAdapter{metric: requestLatency},
		ResolverLatency:       &resolverLatencyAdapter{metric: resolverLatency},
		RequestSize:           &requestSizeAdapter{metric: requestSize},
		ResponseSize:          &responseSizeAdapter{metric: responseSize},
		RateLimiterLatency:    &rateLimiterLatencyAdapter{metric: rateLimiterLatency},
		RequestRetry:          &requestRetryAdapter{metric: requestRetry},
		TransportCacheEntries: &transportCacheEntriesAdapter{metric: transportCacheEntries},
		TransportCreateCalls:  &transportCreateCallsAdapter{metric: transportCacheCalls},
	})
}

// Prometheus adapters for client-go metrics hooks.

type requestResultAdapter struct {
	metric *prometheus.CounterVec
}

func (r *requestResultAdapter) Increment(_ context.Context, code, method, host string) {
	r.metric.WithLabelValues(code, method, host).Inc()
}

type requestLatencyAdapter struct {
	metric *prometheus.HistogramVec
}

func (l *requestLatencyAdapter) Observe(_ context.Context, verb string, u url.URL, latency time.Duration) {
	l.metric.WithLabelValues(verb, u.Host).Observe(latency.Seconds())
}

type resolverLatencyAdapter struct {
	metric *prometheus.HistogramVec
}

func (r *resolverLatencyAdapter) Observe(_ context.Context, host string, latency time.Duration) {
	r.metric.WithLabelValues(host).Observe(latency.Seconds())
}

type requestSizeAdapter struct {
	metric *prometheus.HistogramVec
}

func (s *requestSizeAdapter) Observe(_ context.Context, verb string, host string, size float64) {
	s.metric.WithLabelValues(verb, host).Observe(size)
}

type responseSizeAdapter struct {
	metric *prometheus.HistogramVec
}

func (s *responseSizeAdapter) Observe(_ context.Context, verb string, host string, size float64) {
	s.metric.WithLabelValues(verb, host).Observe(size)
}

type rateLimiterLatencyAdapter struct {
	metric *prometheus.HistogramVec
}

func (l *rateLimiterLatencyAdapter) Observe(_ context.Context, verb string, u url.URL, latency time.Duration) {
	l.metric.WithLabelValues(verb, u.Host).Observe(latency.Seconds())
}

type requestRetryAdapter struct {
	metric *prometheus.CounterVec
}

func (r *requestRetryAdapter) IncrementRetry(_ context.Context, code string, method string, host string) {
	r.metric.WithLabelValues(code, method, host).Inc()
}

type transportCacheEntriesAdapter struct {
	metric prometheus.Gauge
}

func (t *transportCacheEntriesAdapter) Observe(value int) {
	t.metric.Set(float64(value))
}

type transportCreateCallsAdapter struct {
	metric *prometheus.CounterVec
}

func (t *transportCreateCallsAdapter) Increment(result string) {
	t.metric.WithLabelValues(result).Inc()
}
