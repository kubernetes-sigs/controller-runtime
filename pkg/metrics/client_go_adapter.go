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
	"time"

	"github.com/prometheus/client_golang/prometheus"
	clientmetrics "k8s.io/client-go/tools/metrics"
)

var (
	requestResult = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rest_client_requests_total",
			Help: "Number of HTTP requests, partitioned by status code, method, and host.",
		},
		[]string{"code", "method", "host"},
	)

	requestLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rest_client_request_duration_seconds",
			Help:    "Request latency in seconds. Broken down by verb. Intentionally not by host to avoid high cardinality.",
			Buckets: []float64{0.005, 0.025, 0.1, 0.25, 0.5, 1.0, 2.0, 4.0, 8.0, 15.0, 30.0, 60.0},
		},
		[]string{"verb"},
	)

	rateLimiterLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rest_client_rate_limiter_duration_seconds",
			Help:    "Client side rate limiter latency in seconds. Broken down by verb. Intentionally not by host to avoid high cardinality.",
			Buckets: []float64{0.005, 0.025, 0.1, 0.25, 0.5, 1.0, 2.0, 4.0, 8.0, 15.0, 30.0, 60.0},
		},
		[]string{"verb"},
	)

	requestRetry = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rest_client_request_retries_total",
			Help: "Number of request retries, partitioned by status code and verb. Intentionally not by host to avoid high cardinality.",
		},
		[]string{"code", "verb"},
	)
)

func init() {
	registerClientMetrics()
}

// registerClientMetrics sets up the client latency metrics from client-go.
func registerClientMetrics() {
	// register the metrics with our registry
	Registry.MustRegister(requestResult)
	Registry.MustRegister(requestLatency)
	Registry.MustRegister(rateLimiterLatency)
	Registry.MustRegister(requestRetry)

	// register the metrics with client-go
	clientmetrics.Register(clientmetrics.RegisterOpts{
		RequestResult:      &resultAdapter{metric: requestResult},
		RequestLatency:     &latencyAdapter{metric: requestLatency},
		RateLimiterLatency: &latencyAdapter{metric: rateLimiterLatency},
		RequestRetry:       &retryAdapter{requestRetry},
	})
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
	metric *prometheus.HistogramVec
}

// Observe increments the request latency metric for the given verb.
// URL is ignored to avoid high cardinality.
func (l *latencyAdapter) Observe(_ context.Context, verb string, _ url.URL, latency time.Duration) {
	l.metric.WithLabelValues(verb).Observe(latency.Seconds())
}

type retryAdapter struct {
	metric *prometheus.CounterVec
}

// IncrementRetry increments the retry metric for the given code and method.
// host is ignored to avoid high cardinality.
func (r *retryAdapter) IncrementRetry(_ context.Context, code, method, _ string) {
	r.metric.WithLabelValues(code, method).Inc()
}
