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
			MetricResolverLatency,
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
	})
})
