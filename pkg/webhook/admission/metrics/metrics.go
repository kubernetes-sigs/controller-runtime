/*
Copyright 2024 The Kubernetes Authors.

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
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	internalmetrics "sigs.k8s.io/controller-runtime/pkg/webhook/internal/metrics"
)

var (
	// WebhookPanics is a prometheus counter metrics which holds the total
	// number of panics from webhooks.
	WebhookPanics = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "controller_runtime_webhook_panics_total",
		Help: "Total number of webhook panics",
	}, []string{})

	// AdmissionResponseTotal is a prometheus counter metric which holds the
	// total number of admission responses by webhook, allowed status, and
	// admission response status code.
	AdmissionResponseTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "controller_runtime_webhook_admission_responses_total",
		Help: "Total number of admission responses by status code and allowed status.",
	}, []string{"webhook", "allowed", "admission_code"})
)

func init() {
	metrics.Registry.MustRegister(
		WebhookPanics,
		AdmissionResponseTotal,
	)
	// Init metric.
	WebhookPanics.WithLabelValues().Add(0)
}

// InitializeAdmissionResponseTotal initializes the most common admission
// response label combinations for the given webhook path.
func InitializeAdmissionResponseTotal(path string) {
	for _, labels := range [][2]string{
		{"true", "200"},
		{"false", "400"},
		{"false", "403"},
		{"false", "429"},
		{"false", "500"},
	} {
		AdmissionResponseTotal.WithLabelValues(path, labels[0], labels[1]).Add(0)
	}
}

// ObserveAdmissionResponse records an admission response if the webhook is
// instrumented.
func ObserveAdmissionResponse(ctx context.Context, allowed bool, code int32) {
	path, ok := internalmetrics.WebhookPathFromContext(ctx)
	if !ok {
		return
	}

	AdmissionResponseTotal.WithLabelValues(
		path,
		strconv.FormatBool(allowed),
		strconv.FormatInt(int64(code), 10),
	).Inc()
}
