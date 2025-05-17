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

// HistogramMetric is a metric that stores the set of observed values
type HistogramMetric interface {
	Observe(map[string]string, float64)
}

// GaugeMetric is a metric that gets set and can be changed dynamically at runtime
type GaugeMetric interface {
	Set(map[string]string, float64)
	Add(map[string]string, float64)
}

// CounterMetric is a metric that gets incremented monotonically
type CounterMetric interface {
	Inc(map[string]string)
	Add(map[string]string, float64)
}

type PrometheusCounterAdapter struct {
	*prometheus.CounterVec
}

func (p *PrometheusCounterAdapter) Inc(labels map[string]string) {
	p.With(labels).Inc()
}

func (p *PrometheusCounterAdapter) Add(labels map[string]string, val float64) {
	p.With(labels).Add(val)
}

type PrometheusGaugeAdapter struct {
	*prometheus.GaugeVec
}

func (p *PrometheusGaugeAdapter) Set(labels map[string]string, val float64) {
	p.With(labels).Set(val)
}

func (p *PrometheusGaugeAdapter) Add(labels map[string]string, val float64) {
	p.With(labels).Add(val)
}

type PrometheusHistogramAdapter struct {
	*prometheus.HistogramVec
}

func (p *PrometheusHistogramAdapter) Observe(labels map[string]string, val float64) {
	p.With(labels).Observe(val)
}
