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

	"github.com/prometheus/client_golang/prometheus"
)

var (
	cacheSyncDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "controller_runtime_cache_sync_duration_seconds",
		Help: "Duration in seconds for the cache to sync on startup.",
		// Buckets cover the range up to and past the default 2-minute sync timeout.
		Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60, 120, 300},
	})

	cacheSyncTimeouts = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "controller_runtime_cache_sync_timeouts_total",
		Help: "Total number of cache sync operations that timed out waiting for the cache to be ready.",
	})
)

func init() {
	Registry.MustRegister(cacheSyncDuration)
	Registry.MustRegister(cacheSyncTimeouts)
}

// RecordCacheSyncDuration observes the time taken for a cache to sync.
func RecordCacheSyncDuration(d time.Duration) {
	cacheSyncDuration.Observe(d.Seconds())
}

// RecordCacheSyncTimeout increments the timeout counter when a cache sync
// fails due to a deadline exceeded on its context.
func RecordCacheSyncTimeout() {
	cacheSyncTimeouts.Inc()
}
