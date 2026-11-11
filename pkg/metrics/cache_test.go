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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	dto "github.com/prometheus/client_model/go"
)

var _ = Describe("CacheMetrics", func() {
	It("should register controller_runtime_cache_sync_duration_seconds", func() {
		names := gatheredNames()
		Expect(names).To(HaveKey("controller_runtime_cache_sync_duration_seconds"))
	})

	It("should register controller_runtime_cache_sync_timeouts_total", func() {
		names := gatheredNames()
		Expect(names).To(HaveKey("controller_runtime_cache_sync_timeouts_total"))
	})

	It("should observe duration into the histogram", func() {
		// Gather baseline sample count.
		before := histogramSampleCount("controller_runtime_cache_sync_duration_seconds")

		RecordCacheSyncDuration(500 * time.Millisecond)

		after := histogramSampleCount("controller_runtime_cache_sync_duration_seconds")
		Expect(after).To(Equal(before + 1))
	})

	It("should increment timeout counter", func() {
		before := counterValue("controller_runtime_cache_sync_timeouts_total")

		RecordCacheSyncTimeout()

		after := counterValue("controller_runtime_cache_sync_timeouts_total")
		Expect(after).To(Equal(before + 1))
	})
})

// histogramSampleCount returns the total sample count across all metrics in the named family.
func histogramSampleCount(name string) uint64 {
	mf := namesToFamily(name)
	var total uint64
	for _, m := range mf.GetMetric() {
		total += m.GetHistogram().GetSampleCount()
	}
	return total
}

// counterValue returns the value of a counter metric family (no labels).
func counterValue(name string) float64 {
	mfs, err := Registry.Gather()
	Expect(err).NotTo(HaveOccurred())
	for _, mf := range mfs {
		if mf.GetName() == name && mf.GetType() == dto.MetricType_COUNTER {
			metrics := mf.GetMetric()
			if len(metrics) > 0 {
				return metrics[0].GetCounter().GetValue()
			}
		}
	}
	Fail("counter metric " + name + " not found")
	return 0
}
