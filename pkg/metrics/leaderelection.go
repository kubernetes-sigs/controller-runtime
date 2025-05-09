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
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/tools/leaderelection"
	internalmetrics "sigs.k8s.io/controller-runtime/pkg/internal/metrics"
)

var (
	leaderGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "leader_election_master_status",
		Help: "Gauge of if the reporting system is master of the relevant lease, 0 indicates backup, 1 indicates master. 'name' is the string used to identify the lease. Please make sure to group by name.",
	}, []string{"name"})
	leaderSlowpathCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "leader_election_slowpath_total",
		Help: "Total number of slow path exercised in renewing leader leases. 'name' is the string used to identify the lease. Please make sure to group by name.",
	}, []string{"name"})
)

func init() {
	Registry.MustRegister(leaderGauge, leaderSlowpathCounter)
	SetLeaderElectionMetricsProvider(NewPrometheusLeaderElectionMetricsProvider())
}

var leaderElectionMetricsProvider LeaderElectionMetricsProvider

// SetLeaderElectionMetricsProvider sets the leader election provider leveraged by client-go
func SetLeaderElectionMetricsProvider(provider LeaderElectionMetricsProvider) {
	leaderElectionMetricsProvider = provider
	leaderelection.SetProvider(leaderElectionMetricsInternalProvider{provider: provider})
}

// GetLeaderElectionMetricsProvider returns the leader election metrics provider
func GetLeaderElectionMetricsProvider() LeaderElectionMetricsProvider {
	return leaderElectionMetricsProvider
}

// LeaderElectionMetricsProvider is an interface that provides methods for firing leader election metrics
type LeaderElectionMetricsProvider interface {
	LeaderGauge() internalmetrics.GaugeMetric
	SlowpathExercised() internalmetrics.CounterMetric
}

// PrometheusLeaderElectionMetricsProvider is a metrics.LeaderElectionMetricsProvider
// that fires prometheus metrics in response to leader election and controller events
type PrometheusLeaderElectionMetricsProvider struct {
	leaderGauge           *prometheus.GaugeVec
	leaderSlowpathCounter *prometheus.CounterVec
}

// NewPrometheusLeaderElectionMetricsProvider creates a PrometheusLeaderElectionMetricsProvider
func NewPrometheusLeaderElectionMetricsProvider() *PrometheusLeaderElectionMetricsProvider {
	return &PrometheusLeaderElectionMetricsProvider{
		leaderGauge:           leaderGauge,
		leaderSlowpathCounter: leaderSlowpathCounter,
	}
}

// LeaderGauge returns a Prometheus gauge that fulfills the GaugeMetric interface
func (p PrometheusLeaderElectionMetricsProvider) LeaderGauge() internalmetrics.GaugeMetric {
	return &internalmetrics.PrometheusGaugeAdapter{GaugeVec: p.leaderGauge}
}

// SlowpathExercised returns a Prometheus counter that fulfills the CounterMetric interface
func (p PrometheusLeaderElectionMetricsProvider) SlowpathExercised() internalmetrics.CounterMetric {
	return &internalmetrics.PrometheusCounterAdapter{CounterVec: p.leaderSlowpathCounter}
}

type leaderElectionMetricsInternalProvider struct {
	provider LeaderElectionMetricsProvider
}

func (l leaderElectionMetricsInternalProvider) NewLeaderMetric() leaderelection.LeaderMetric {
	return leaderElectionMetricAdapter(l)
}

type leaderElectionMetricAdapter struct {
	provider LeaderElectionMetricsProvider
}

func (l leaderElectionMetricAdapter) On(name string) {
	l.provider.LeaderGauge().Set(map[string]string{"name": name}, 1)
}

func (l leaderElectionMetricAdapter) Off(name string) {
	l.provider.LeaderGauge().Set(map[string]string{"name": name}, 0)
}

func (l leaderElectionMetricAdapter) SlowpathExercised(name string) {
	l.provider.SlowpathExercised().Inc(map[string]string{"name": name})
}
