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
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	testHost   = "test-host:8080"
	testVerb   = "GET"
	testMethod = "GET"
	testCode   = "200"
)

var (
	testURL = &url.URL{Host: testHost}
)

func setupTest(metric prometheus.Collector) prometheus.Gatherer {
	reg := prometheus.NewRegistry()
	reg.MustRegister(metric)
	return reg
}

var _ = Describe("client-go metrics", func() {
	Describe("RequestResultAdapter", func() {
		It("increments the counter correctly", func() {
			metric := prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Name: "test_metric",
					Help: "foo",
				},
				[]string{"code", "method", "host"},
			)
			reg := setupTest(metric)
			adapter := &requestResultAdapter{metric: metric}

			adapter.Increment(context.TODO(), testCode, testMethod, testHost)
			adapter.Increment(context.TODO(), testCode, testMethod, testHost)

			Expect(testutil.GatherAndCompare(reg, strings.NewReader(`
# HELP test_metric foo
# TYPE test_metric counter
test_metric{code="200",host="test-host:8080",method="GET"} 2
`))).To(Succeed())
		})
	})

	Describe("RequestSizeAdapter", func() {
		It("records histogram observations", func() {
			metric := prometheus.NewHistogramVec(
				prometheus.HistogramOpts{
					Name:    "test_metric",
					Help:    "foo",
					Buckets: []float64{100, 1000, 10000},
				},
				[]string{"verb", "host"},
			)
			reg := setupTest(metric)
			adapter := &requestSizeAdapter{metric: metric}

			adapter.Observe(context.TODO(), testVerb, testHost, 500)
			adapter.Observe(context.TODO(), testVerb, testHost, 5000)

			Expect(testutil.GatherAndCompare(reg, strings.NewReader(`
# HELP test_metric foo
# TYPE test_metric histogram
test_metric_bucket{host="test-host:8080",verb="GET",le="100"} 0
test_metric_bucket{host="test-host:8080",verb="GET",le="1000"} 1
test_metric_bucket{host="test-host:8080",verb="GET",le="10000"} 2
test_metric_bucket{host="test-host:8080",verb="GET",le="+Inf"} 2
test_metric_sum{host="test-host:8080",verb="GET"} 5500
test_metric_count{host="test-host:8080",verb="GET"} 2
`))).To(Succeed())
		})
	})

	Describe("ResponseSizeAdapter", func() {
		It("records histogram observations", func() {
			metric := prometheus.NewHistogramVec(
				prometheus.HistogramOpts{
					Name:    "test_metric",
					Help:    "foo",
					Buckets: []float64{100, 1000, 10000},
				},
				[]string{"verb", "host"},
			)
			reg := setupTest(metric)
			adapter := &responseSizeAdapter{metric: metric}

			adapter.Observe(context.TODO(), testVerb, testHost, 750)
			adapter.Observe(context.TODO(), testVerb, testHost, 7500)

			Expect(testutil.GatherAndCompare(reg, strings.NewReader(`
# HELP test_metric foo
# TYPE test_metric histogram
test_metric_bucket{host="test-host:8080",verb="GET",le="100"} 0
test_metric_bucket{host="test-host:8080",verb="GET",le="1000"} 1
test_metric_bucket{host="test-host:8080",verb="GET",le="10000"} 2
test_metric_bucket{host="test-host:8080",verb="GET",le="+Inf"} 2
test_metric_sum{host="test-host:8080",verb="GET"} 8250
test_metric_count{host="test-host:8080",verb="GET"} 2
`))).To(Succeed())
		})
	})

	Describe("RateLimiterLatencyAdapter", func() {
		It("records latency in histogram", func() {
			metric := prometheus.NewHistogramVec(
				prometheus.HistogramOpts{
					Name:    "test_metric",
					Help:    "foo",
					Buckets: []float64{0.1, 0.2, 0.4},
				},
				[]string{"verb", "host"},
			)
			reg := setupTest(metric)
			adapter := &rateLimiterLatencyAdapter{metric: metric}

			adapter.Observe(context.TODO(), testVerb, *testURL, 300*time.Millisecond)

			Expect(testutil.GatherAndCompare(reg, strings.NewReader(`
# HELP test_metric foo
# TYPE test_metric histogram
test_metric_bucket{host="test-host:8080",verb="GET",le="0.1"} 0
test_metric_bucket{host="test-host:8080",verb="GET",le="0.2"} 0
test_metric_bucket{host="test-host:8080",verb="GET",le="0.4"} 1
test_metric_bucket{host="test-host:8080",verb="GET",le="+Inf"} 1
test_metric_sum{host="test-host:8080",verb="GET"} 0.3
test_metric_count{host="test-host:8080",verb="GET"} 1
`))).To(Succeed())
		})
	})

	Describe("ResolverLatencyAdapter", func() {
		It("records latency in histogram", func() {
			metric := prometheus.NewHistogramVec(
				prometheus.HistogramOpts{
					Name:    "test_metric",
					Help:    "foo",
					Buckets: []float64{0.1, 0.2, 0.4},
				},
				[]string{"host"},
			)
			reg := setupTest(metric)
			adapter := &resolverLatencyAdapter{metric: metric}

			adapter.Observe(context.TODO(), testHost, 120*time.Millisecond)
			adapter.Observe(context.TODO(), testHost, 300*time.Millisecond)

			Expect(testutil.GatherAndCompare(reg, strings.NewReader(`
# HELP test_metric foo
# TYPE test_metric histogram
test_metric_bucket{host="test-host:8080",le="0.1"} 0
test_metric_bucket{host="test-host:8080",le="0.2"} 1
test_metric_bucket{host="test-host:8080",le="0.4"} 2
test_metric_bucket{host="test-host:8080",le="+Inf"} 2
test_metric_sum{host="test-host:8080"} 0.42
test_metric_count{host="test-host:8080"} 2
`))).To(Succeed())
		})
	})

	Describe("RequestRetryAdapter", func() {
		It("increments retry counter", func() {
			metric := prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Name: "test_metric",
					Help: "foo",
				},
				[]string{"code", "verb", "host"},
			)
			reg := setupTest(metric)
			adapter := &requestRetryAdapter{metric: metric}

			adapter.IncrementRetry(context.TODO(), testCode, testVerb, testHost)
			adapter.IncrementRetry(context.TODO(), testCode, testVerb, testHost)

			Expect(testutil.GatherAndCompare(reg, strings.NewReader(`
# HELP test_metric foo
# TYPE test_metric counter
test_metric{code="200",host="test-host:8080",verb="GET"} 2
`))).To(Succeed())
		})
	})

	Describe("TransportCacheEntriesAdapter", func() {
		It("sets gauge value", func() {
			metric := prometheus.NewGauge(
				prometheus.GaugeOpts{
					Name: "test_metric",
					Help: "foo",
				},
			)
			reg := setupTest(metric)
			adapter := &transportCacheEntriesAdapter{metric: metric}

			adapter.Observe(5)

			Expect(testutil.GatherAndCompare(reg, strings.NewReader(`
# HELP test_metric foo
# TYPE test_metric gauge
test_metric 5
`))).To(Succeed())
		})
	})

	Describe("TransportCreateCallsAdapter", func() {
		It("increments counter for results", func() {
			metric := prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Name: "test_metric",
					Help: "foo",
				},
				[]string{"result"},
			)
			reg := setupTest(metric)
			adapter := &transportCreateCallsAdapter{metric: metric}

			adapter.Increment("hit")
			adapter.Increment("miss")
			adapter.Increment("hit")

			Expect(testutil.GatherAndCompare(reg, strings.NewReader(`
# HELP test_metric foo
# TYPE test_metric counter
test_metric{result="hit"} 2
test_metric{result="miss"} 1
`))).To(Succeed())
		})
	})

	Describe("RequestLatencyAdapter", func() {
		It("records request latency in histogram", func() {
			metric := prometheus.NewHistogramVec(
				prometheus.HistogramOpts{
					Name:    "test_metric",
					Help:    "foo",
					Buckets: []float64{0.1, 0.2, 0.4},
				},
				[]string{"verb", "host"},
			)
			reg := setupTest(metric)
			adapter := &requestLatencyAdapter{metric: metric}

			adapter.Observe(context.TODO(), testVerb, *testURL, 300*time.Millisecond)

			Expect(testutil.GatherAndCompare(reg, strings.NewReader(`
# HELP test_metric foo
# TYPE test_metric histogram
test_metric_bucket{host="test-host:8080",verb="GET",le="0.1"} 0
test_metric_bucket{host="test-host:8080",verb="GET",le="0.2"} 0
test_metric_bucket{host="test-host:8080",verb="GET",le="0.4"} 1
test_metric_bucket{host="test-host:8080",verb="GET",le="+Inf"} 1
test_metric_sum{host="test-host:8080",verb="GET"} 0.3
test_metric_count{host="test-host:8080",verb="GET"} 1
`))).To(Succeed())
		})
	})
})
