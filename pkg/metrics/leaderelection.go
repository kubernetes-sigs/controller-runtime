package metrics

import (
	"k8s.io/client-go/tools/leaderelection"
)

// SetLeaderElectionProvider sets the leader election provider leveraged by client-go
func SetLeaderElectionProvider(provider LeaderElectionMetricsProvider) {
	leaderelection.SetProvider(leaderElectionMetricsProvider{provider: provider})
}

type leaderElectionMetricsProvider struct {
	provider LeaderElectionMetricsProvider
}

func (l leaderElectionMetricsProvider) NewLeaderMetric() leaderelection.LeaderMetric {
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
