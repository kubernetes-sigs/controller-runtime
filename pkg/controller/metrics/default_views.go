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
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

var (
	// DefaultPrometheusDistribution is an OpenCensus Distribution with the same
	// buckets as the default buckets in the Prometheus client.
	DefaultPrometheusDistribution = view.Distribution(.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10)
)

// RegisterDefaultViews registers default OpenCensus views for controller
// measures. It does not create an exporter.
func RegisterDefaultViews() {
	// Count ReconcileTotal with Controller and Result tags
	view.Register(&view.View{
		Name:        MeasureReconcileTotal.Name(),
		Measure:     MeasureReconcileTotal,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{TagController, TagResult},
	})

	// Count ReconcileError with Controller tag
	view.Register(&view.View{
		Name:        MeasureReconcileErrors.Name(),
		Measure:     MeasureReconcileErrors,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{TagController},
	})

	// Histogram of ReconcileTime with Controller tag
	view.Register(&view.View{
		Name:        MeasureReconcileTime.Name(),
		Measure:     MeasureReconcileTime,
		Aggregation: DefaultPrometheusDistribution,
		TagKeys:     []tag.Key{TagController},
	})
}

// UnregisterDefaultViews unregisters the OpenCensus views registered by
// RegisterDefaultViews.
func UnregisterDefaultViews() {
	view.Unregister(view.Find(MeasureReconcileTotal.Name()))
	view.Unregister(view.Find(MeasureReconcileErrors.Name()))
	view.Unregister(view.Find(MeasureReconcileTime.Name()))

}
