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
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ = Describe("Cache Metrics", func() {

	Describe("RecordCacheResourceCount", func() {
		BeforeEach(func() {
			// Reset Registry to ensure tests don't affect each other
			Registry = prometheus.NewRegistry()
			Registry.MustRegister(CacheResourceCount)
		})

		DescribeTable("recording resource counts",
			func(gvk schema.GroupVersionKind, count int, wantCount float64) {
				// Call the function being tested
				RecordCacheResourceCount(gvk, count)

				// Build metric validation function
				gauge := CacheResourceCount.WithLabelValues(gvk.Group, gvk.Version, gvk.Kind)
				var metric dto.Metric
				err := gauge.Write(&metric)
				Expect(err).NotTo(HaveOccurred(), "Failed to write metric")

				// Verify the metric value matches the expected value
				actualValue := metric.GetGauge().GetValue()
				Expect(actualValue).To(Equal(wantCount))
			},
			Entry("record pod count",
				schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
				10,
				float64(10)),
			Entry("record deployment count",
				schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
				5,
				float64(5)),
		)

		It("should update existing metric values", func() {
			gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}

			// First record
			RecordCacheResourceCount(gvk, 10)
			gauge := CacheResourceCount.WithLabelValues(gvk.Group, gvk.Version, gvk.Kind)
			var metric dto.Metric
			err := gauge.Write(&metric)
			Expect(err).NotTo(HaveOccurred())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(10)))

			// Update
			RecordCacheResourceCount(gvk, 15)
			err = gauge.Write(&metric)
			Expect(err).NotTo(HaveOccurred())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(15)))
		})
	})

	Describe("CacheResourceCount metric configuration", func() {
		It("should have the correct configuration", func() {
			// Create a new registry to avoid effects from previous tests
			Registry = prometheus.NewRegistry()

			// Register our metric
			CacheResourceCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: "controller_runtime_cache_resources",
				Help: "Number of resources cached in the controller-runtime local cache, broken down by group, version, kind",
			}, []string{"group", "version", "kind"})

			Registry.MustRegister(CacheResourceCount)

			expected := `
				# HELP controller_runtime_cache_resources Number of resources cached in the controller-runtime local cache, broken down by group, version, kind
				# TYPE controller_runtime_cache_resources gauge
			`

			err := testutil.GatherAndCompare(Registry, strings.NewReader(expected),
				"controller_runtime_cache_resources")
			Expect(err).NotTo(HaveOccurred(), "Metrics configuration is incorrect")
		})
	})

	Describe("Multiple Resource Counts", func() {
		var gvks []struct {
			gvk   schema.GroupVersionKind
			count int
		}

		BeforeEach(func() {
			Registry = prometheus.NewRegistry()
			Registry.MustRegister(CacheResourceCount)

			// Define test data for multiple GVKs
			gvks = []struct {
				gvk   schema.GroupVersionKind
				count int
			}{
				{
					gvk: schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Pod",
					},
					count: 10,
				},
				{
					gvk: schema.GroupVersionKind{
						Group:   "apps",
						Version: "v1",
						Kind:    "Deployment",
					},
					count: 5,
				},
				{
					gvk: schema.GroupVersionKind{
						Group:   "networking.k8s.io",
						Version: "v1",
						Kind:    "Ingress",
					},
					count: 3,
				},
			}

			// Record all metrics
			for _, g := range gvks {
				RecordCacheResourceCount(g.gvk, g.count)
			}
		})

		It("should store multiple GVK resource counts correctly", func() {
			// Collect and validate all metrics
			metrics, err := Registry.Gather()
			Expect(err).NotTo(HaveOccurred(), "Failed to gather metrics")

			// There should be only one metric family (controller_runtime_cache_resources)
			Expect(metrics).To(HaveLen(1), "Expected 1 metric family")

			// Verify counter count matches the number of GVKs we set
			metricFamily := metrics[0]
			Expect(metricFamily.Metric).To(HaveLen(len(gvks)), "Expected metrics count to match GVK count")

			// Create a map of expected values for easier lookup and verification
			expected := make(map[string]int)
			for _, g := range gvks {
				key := fmt.Sprintf("%s/%s/%s", g.gvk.Group, g.gvk.Version, g.gvk.Kind)
				if g.gvk.Group == "" {
					key = fmt.Sprintf("/%s/%s", g.gvk.Version, g.gvk.Kind)
				}
				expected[key] = g.count
			}

			// Verify each metric value
			for _, m := range metricFamily.Metric {
				// Build key from labels
				var group, version, kind string
				for _, l := range m.Label {
					switch l.GetName() {
					case "group":
						group = l.GetValue()
					case "version":
						version = l.GetValue()
					case "kind":
						kind = l.GetValue()
					}
				}

				key := fmt.Sprintf("%s/%s/%s", group, version, kind)
				if group == "" {
					key = fmt.Sprintf("/%s/%s", version, kind)
				}

				// Verify value matches expected
				expectedCount, ok := expected[key]
				Expect(ok).To(BeTrue(), "Unexpected metric with labels %s", key)

				actualValue := m.GetGauge().GetValue()
				Expect(actualValue).To(Equal(float64(expectedCount)),
					"For %s: expected value %d but got %f", key, expectedCount, actualValue)
			}
		})
	})
})
