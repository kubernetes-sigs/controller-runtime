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

	// ViewReconcileTotal counts ReconcileTotal with Controller and Result tags.
	ViewReconcileTotal = view.View{
		Name:        "controller_runtime_reconcile_total",
		Measure:     MeasureReconcileTotal,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{TagController, TagResult},
	}

	// ViewReconcileError counts ReconcileError with a Controller tag.
	ViewReconcileError = view.View{
		Name:        "controller_runtime_reconcile_errors_total",
		Measure:     MeasureReconcileErrors,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{TagController},
	}
	// ViewReconcileTime is a histogram of ReconcileTime with a Controller tag.
	ViewReconcileTime = view.View{
		Name:        "controller_runtime_reconcile_time_seconds",
		Measure:     MeasureReconcileTime,
		Aggregation: DefaultPrometheusDistribution,
		TagKeys:     []tag.Key{TagController},
	}

	// DefaultViews is an array of OpenCensus views that can be registered
	// using view.Register(metrics.DefaultViews...) to export default metrics.
	DefaultViews = []*view.View{
		&ViewReconcileTotal,
		&ViewReconcileError,
		&ViewReconcileTime,
	}
)
