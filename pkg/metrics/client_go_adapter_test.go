/*
Copyright 2026 The Kubernetes Authors.

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
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	clientmetrics "k8s.io/client-go/tools/metrics"
)

func TestMetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metrics Suite")
}

var optInMetricNames = []string{
	"rest_client_request_duration_seconds",
	"rest_client_dns_resolution_duration_seconds",
	"rest_client_request_size_bytes",
	"rest_client_response_size_bytes",
	"rest_client_rate_limiter_duration_seconds",
	"rest_client_request_retries_total",
}

func observeAllRESTClientMetrics(ctx context.Context) {
	clientmetrics.RequestResult.Increment(ctx, "200", "GET", "example.com")
	clientmetrics.RequestLatency.Observe(ctx, "GET", url.URL{Host: "example.com"}, 1*time.Second)
	clientmetrics.ResolverLatency.Observe(ctx, "example.com", 1*time.Second)
	clientmetrics.RequestSize.Observe(ctx, "GET", "example.com", 1024)
	clientmetrics.ResponseSize.Observe(ctx, "GET", "example.com", 1024)
	clientmetrics.RateLimiterLatency.Observe(ctx, "GET", url.URL{Host: "example.com"}, 1*time.Second)
	clientmetrics.RequestRetry.IncrementRetry(ctx, "200", "GET", "example.com")
}

func gatheredNames() map[string]struct{} {
	mfs, err := Registry.Gather()
	Expect(err).NotTo(HaveOccurred())

	names := make(map[string]struct{})
	for _, mf := range mfs {
		names[mf.GetName()] = struct{}{}
	}
	return names
}

var _ = Describe("RESTClientMetrics", func() {
	It("should expose default metrics and opt-in metrics when registered", func(ctx SpecContext) {
		observeAllRESTClientMetrics(ctx)

		names := gatheredNames()
		Expect(names).To(HaveKey("rest_client_requests_total"), "metric rest_client_requests_total should be exposed by default")
		for _, name := range optInMetricNames {
			Expect(names).NotTo(HaveKey(name), "metric %s should not be found before calling RegisterRESTClientMetrics", name)
		}

		RegisterRESTClientMetrics(
			MetricRequestLatency,
			MetricDNSResolutionLatency,
			MetricRequestSize,
			MetricResponseSize,
			MetricRateLimiterLatency,
			MetricRequestRetry,
		)
		observeAllRESTClientMetrics(ctx)

		names = gatheredNames()
		for _, name := range optInMetricNames {
			Expect(names).To(HaveKey(name), "metric %s should be found after calling RegisterRESTClientMetrics", name)
		}

		Expect(histogramBounds(namesToFamily("rest_client_request_duration_seconds"))).To(Equal(defaultRESTClientDurationBuckets))
		Expect(histogramBounds(namesToFamily("rest_client_rate_limiter_duration_seconds"))).To(Equal(defaultRESTClientDurationBuckets))
		Expect(histogramBounds(namesToFamily("rest_client_dns_resolution_duration_seconds"))).To(Equal(defaultRESTClientDurationBuckets))
	})
})

func namesToFamily(name string) *dto.MetricFamily {
	mfs, err := Registry.Gather()
	Expect(err).NotTo(HaveOccurred())
	for _, mf := range mfs {
		if mf.GetName() == name {
			return mf
		}
	}
	Fail("metric family " + name + " not found")
	return nil
}

func histogramBounds(mf *dto.MetricFamily) []float64 {
	Expect(mf.GetMetric()).NotTo(BeEmpty())
	buckets := mf.GetMetric()[0].GetHistogram().GetBucket()
	bounds := make([]float64, 0, len(buckets))
	for _, b := range buckets {
		bounds = append(bounds, b.GetUpperBound())
	}
	return bounds
}

func TestRESTClientDurationHistogramBuckets(t *testing.T) {
	t.Parallel()

	t.Run("default buckets start at 5ms", func(t *testing.T) {
		t.Parallel()
		h := restClientDurationHistogram(
			"test_rest_client_request_duration_seconds",
			"test",
			[]string{verbLabel, hostLabel},
			nil,
		)
		bounds := gatherHistogramBounds(t, h, "GET", "example.com", 0.001)
		if len(bounds) != len(defaultRESTClientDurationBuckets) {
			t.Fatalf("got %d buckets, want %d: %v", len(bounds), len(defaultRESTClientDurationBuckets), bounds)
		}
		for i := range defaultRESTClientDurationBuckets {
			if bounds[i] != defaultRESTClientDurationBuckets[i] {
				t.Fatalf("bucket[%d]=%v, want %v", i, bounds[i], defaultRESTClientDurationBuckets[i])
			}
		}
	})

	t.Run("custom buckets capture sub-5ms observations", func(t *testing.T) {
		t.Parallel()
		custom := []float64{0.001, 0.0025, 0.005, 0.025, 0.1}
		h := restClientDurationHistogram(
			"test_rest_client_request_duration_seconds_custom",
			"test",
			[]string{verbLabel, hostLabel},
			custom,
		)
		reg := prometheus.NewPedanticRegistry()
		if err := reg.Register(h); err != nil {
			t.Fatal(err)
		}
		h.WithLabelValues("PATCH", "example.com").Observe(0.001)

		mfs, err := reg.Gather()
		if err != nil {
			t.Fatal(err)
		}
		if len(mfs) != 1 || len(mfs[0].GetMetric()) != 1 {
			t.Fatalf("expected one metric family with one series, got %#v", mfs)
		}
		buckets := mfs[0].GetMetric()[0].GetHistogram().GetBucket()
		if len(buckets) != len(custom) {
			t.Fatalf("got %d buckets, want %d", len(buckets), len(custom))
		}
		for i, want := range custom {
			if buckets[i].GetUpperBound() != want {
				t.Fatalf("bucket[%d] le=%v, want %v", i, buckets[i].GetUpperBound(), want)
			}
		}
		// 1ms observation should land in the 0.001s bucket, not collapse into 5ms.
		if buckets[0].GetCumulativeCount() != 1 {
			t.Fatalf("le=0.001 count=%d, want 1", buckets[0].GetCumulativeCount())
		}
	})
}

func gatherHistogramBounds(t *testing.T, h *prometheus.HistogramVec, verb, host string, observe float64) []float64 {
	t.Helper()
	reg := prometheus.NewPedanticRegistry()
	if err := reg.Register(h); err != nil {
		t.Fatal(err)
	}
	h.WithLabelValues(verb, host).Observe(observe)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	if len(mfs) != 1 || len(mfs[0].GetMetric()) != 1 {
		t.Fatalf("expected one metric family with one series, got %#v", mfs)
	}
	buckets := mfs[0].GetMetric()[0].GetHistogram().GetBucket()
	bounds := make([]float64, 0, len(buckets))
	for _, b := range buckets {
		bounds = append(bounds, b.GetUpperBound())
	}
	return bounds
}
