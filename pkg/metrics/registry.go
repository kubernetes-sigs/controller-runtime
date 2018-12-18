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

import "github.com/prometheus/client_golang/prometheus"

// DefaultMetrics are the default metrics provided by controller-runtime.
var DefaultMetrics = []prometheus.Collector{
	// expose process metrics like CPU, Memory, file descriptor usage etc.
	prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
	// expose Go runtime metrics like GC stats, memory stats etc.
	prometheus.NewGoCollector(),
}

// Registry is a prometheus registry for storing metrics within the
// controller-runtime
var Registry = prometheus.NewRegistry()
