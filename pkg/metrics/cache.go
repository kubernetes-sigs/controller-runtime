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
	"github.com/prometheus/client_golang/prometheus"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// CacheResourceCount is a prometheus metric which counts the number of resources
	// cached in the local controller-runtime cache, broken down by resource GVK.
	CacheResourceCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "controller_runtime_cache_resources",
		Help: "Number of resources cached in the controller-runtime local cache, broken down by group, version, kind",
	}, []string{"group", "version", "kind"})
)

func init() {
	Registry.MustRegister(CacheResourceCount)
}

// RecordCacheResourceCount records the count of a specific resource type in the cache
func RecordCacheResourceCount(gvk schema.GroupVersionKind, count int) {
	CacheResourceCount.WithLabelValues(gvk.Group, gvk.Version, gvk.Kind).Set(float64(count))
}
