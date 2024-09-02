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
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	clientmetrics "k8s.io/client-go/tools/metrics"
)

// this file contains setup logic to initialize the myriad of places
// that client-go registers metrics.  We copy the names and formats
// from Kubernetes so that we match the core controllers.

var (
	// client metrics.

	requestResult = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rest_client_requests_total",
			Help: "Number of HTTP requests, partitioned by status code, method, and host.",
		},
		[]string{"code", "method", "host"},
	)

	clientMetricsRegisterOnce = sync.Once{}
)

func init() {
	// register the metrics with our registry
	Registry.MustRegister(requestResult)

	time.AfterFunc(30*time.Second, func() {
		RegisterClientMetrics(clientmetrics.RegisterOpts{})
	})
}

// RegisterClientMetrics sets up the client latency metrics from client-go. Since clientmetrics.Register can only be
// called once, you MUST call this method if you want to register other client-go metrics within the first 30 seconds
// of a binaries lifetime, or it will get called without other metrics.
func RegisterClientMetrics(opts clientmetrics.RegisterOpts) {
	clientMetricsRegisterOnce.Do(func() {
		opts.RequestResult = &resultAdapter{
			metric:       requestResult,
			customMetric: opts.RequestResult,
		}
		// register the metrics with client-go
		clientmetrics.Register(opts)
	})
}

// this section contains adapters, implementations, and other sundry organic, artisanally
// hand-crafted syntax trees required to convince client-go that it actually wants to let
// someone use its metrics.

// Client metrics adapters (method #1 for client-go metrics),
// copied (more-or-less directly) from k8s.io/kubernetes setup code
// (which isn't anywhere in an easily-importable place).

type resultAdapter struct {
	metric       *prometheus.CounterVec
	customMetric clientmetrics.ResultMetric
}

func (r *resultAdapter) Increment(ctx context.Context, code, method, host string) {
	r.metric.WithLabelValues(code, method, host).Inc()
	if r.customMetric != nil {
		r.customMetric.Increment(ctx, code, method, host)
	}
}
