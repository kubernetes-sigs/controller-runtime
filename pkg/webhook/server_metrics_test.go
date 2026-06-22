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

package webhook

import (
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

func TestRegisterInitializesAdmissionResponseMetrics(t *testing.T) {
	const path = "/server-admission-response-metrics-test"
	NewServer(Options{}).Register(path, &Admission{})

	metricFamilies, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	want := map[string]bool{
		"true/200":  false,
		"false/400": false,
		"false/403": false,
		"false/500": false,
	}
	for _, family := range metricFamilies {
		if family.GetName() != "controller_runtime_webhook_admission_responses_total" {
			continue
		}
		for _, metric := range family.Metric {
			labels := map[string]string{}
			for _, label := range metric.Label {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["webhook"] != path {
				continue
			}

			key := labels["allowed"] + "/" + labels["admission_code"]
			if _, ok := want[key]; !ok {
				t.Errorf("unexpected initialized label combination %q", key)
				continue
			}
			if value := metric.GetCounter().GetValue(); value != 0 {
				t.Errorf("expected initialized metric %q to be zero, got %v", key, value)
			}
			want[key] = true
		}
	}

	for labels, found := range want {
		if !found {
			t.Errorf("expected initialized metric labels %q", labels)
		}
	}
}
