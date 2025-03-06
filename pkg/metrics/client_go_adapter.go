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

// this file contains setup logic to initialize the myriad of places
// that client-go registers metrics.  We copy the names and formats
// from Kubernetes so that we match the core controllers.

var (
	// client metrics.

	requestLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "rest_client_request_duration_seconds",
			Help: "Request latency in seconds. Broken down by verb and URL.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0,
				1.25, 1.5, 1.75, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5, 5, 6, 7, 8, 9, 10, 15, 20, 25, 30, 40, 50, 60},
		},
		[]string{"verb", "host", "path"},
	)

	rateLimiterLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "rest_client_rate_limiter_duration_seconds",
			Help: "Client side rate limiter latency in seconds. Broken down by verb and URL.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0,
				1.25, 1.5, 1.75, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5, 5, 6, 7, 8, 9, 10, 15, 20, 25, 30, 40, 50, 60},
		},
		[]string{"verb", "host", "path"},
	)

	requestResult = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rest_client_requests_total",
			Help: "Number of HTTP requests, partitioned by status code, method, and host.",
		},
		[]string{"code", "method", "host"},
	)
)

func init() {
	registerClientMetrics()
}

// registerClientMetrics sets up the client latency metrics from client-go.
func registerClientMetrics() {
	// register the metrics with our registry
	Registry.MustRegister(requestResult)
	Registry.MustRegister(rateLimiterLatency)
	Registry.MustRegister(requestLatency)

	// register the metrics with client-go
	clientmetrics.Register(clientmetrics.RegisterOpts{
		RequestResult:      &resultAdapter{metric: requestResult},
		RateLimiterLatency: &latencyAdapter{metric: rateLimiterLatency},
		RequestLatency:     &latencyAdapter{metric: requestLatency},
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

func (l *latencyAdapter) Observe(_ context.Context, verb string, u url.URL, latency time.Duration) {
	l.metric.WithLabelValues(verb, u.Host, u.Path).Observe(latency.Seconds())
}
