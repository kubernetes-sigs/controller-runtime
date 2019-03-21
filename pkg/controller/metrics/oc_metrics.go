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
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
)

var (
	// MeasureReconcileTotal is a counter which records the total number of reconciliations
	// per controller. It has two tags: controller refers
	// to the controller name and result refers to the reconcile result e.g.
	// success, error, requeue, requeue_after.
	MeasureReconcileTotal = stats.Int64(
		"sigs.kubernetes.io/controller-runtime/measures/reconcile_total",
		"Total number of reconciliations per controller",
		stats.UnitNone,
	)

	// MeasureReconcileErrors is a counter which records the total
	// number of errors from the Reconciler. It has one tag: controller refers
	// to the controller name. TODO is this necessary when we have the above with
	// error tag?
	MeasureReconcileErrors = stats.Int64(
		"sigs.kubernetes.io/controller-runtime/measures/reconcile_errors_total",
		"Total number of reconciliation errors per controller",
		stats.UnitNone,
	)

	// MeasureReconcileTime is a measure which keeps track of the duration
	// of reconciliations. It has one tag: controller refers
	// to the controller name. // TODO should this be milliseconds?
	MeasureReconcileTime = stats.Float64(
		"sigs.kubernetes.io/controller-runtime/measures/reconcile_time_seconds",
		"Length of time per reconciliation per controller",
		"s",
	)

	// Tag keys must conform to the restrictions described in
	// go.opencensus.io/tag/validate.go. Currently those restrictions are:
	// - length between 1 and 255 inclusive
	// - characters are printable US-ASCII

	// TagController is a tag referring to the controller name that produced a
	// measurement.
	TagController = mustNewTagKey("controller")

	// TagResult is a tag referring to the reconcile result of a reconciler.
	TagResult = mustNewTagKey("result")
)

func mustNewTagKey(k string) tag.Key {
	tagKey, err := tag.NewKey(k)
	if err != nil {
		panic(err)
	}
	return tagKey
}
