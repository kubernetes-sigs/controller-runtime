/*
Copyright 2022 The Kubernetes Authors.

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

package zap

import (
	"github.com/go-logr/logr"
	"go.uber.org/atomic"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

var _ logr.LogSink = (*KubeAwareLogSink)(nil)

// KubeAwareLogSink is a Kubernetes-aware logr.LogSink.
// zapcore.ObjectMarshaler would be bypassed when using zapr and WithValues.
// It would use a wrapper implements logr.Marshaler instead of using origin Kubernetes objects.
type KubeAwareLogSink struct {
	sink             logr.LogSink
	kubeAwareEnabled *atomic.Bool
}

// NewKubeAwareLogger return the wrapper with existed logr.Logger.
// logger is the backend logger.
// kubeAwareEnabled is the flag to enable kube aware logging.
func NewKubeAwareLogger(logger logr.Logger, kubeAwareEnabled bool) logr.Logger {
	return logr.New(NewKubeAwareLogSink(logger.GetSink(), kubeAwareEnabled))
}

// NewKubeAwareLogSink return the wrapper with existed logr.LogSink.
// sink is the backend logr.LogSink.
// kubeAwareEnabled is the flag to enable kube aware logging.
func NewKubeAwareLogSink(logSink logr.LogSink, kubeAwareEnabled bool) *KubeAwareLogSink {
	return &KubeAwareLogSink{sink: logSink, kubeAwareEnabled: atomic.NewBool(kubeAwareEnabled)}
}

// Init implements logr.LogSink.
func (k *KubeAwareLogSink) Init(info logr.RuntimeInfo) {
	k.sink.Init(info)
}

// Enabled implements logr.LogSink.
func (k *KubeAwareLogSink) Enabled(level int) bool {
	return k.sink.Enabled(level)
}

// Info implements logr.LogSink.
func (k *KubeAwareLogSink) Info(level int, msg string, keysAndValues ...interface{}) {
	if !k.KubeAwareEnabled() {
		k.sink.Info(level, msg, keysAndValues...)
		return
	}

	k.sink.Info(level, msg, k.wrapKeyAndValues(keysAndValues)...)
}

// Error implements logr.LogSink.
func (k *KubeAwareLogSink) Error(err error, msg string, keysAndValues ...interface{}) {
	if !k.KubeAwareEnabled() {
		k.sink.Error(err, msg, keysAndValues...)
		return
	}
	k.sink.Error(err, msg, k.wrapKeyAndValues(keysAndValues)...)
}

// WithValues implements logr.LogSink.
func (k *KubeAwareLogSink) WithValues(keysAndValues ...interface{}) logr.LogSink {
	return &KubeAwareLogSink{
		kubeAwareEnabled: k.kubeAwareEnabled,
		sink:             k.sink.WithValues(k.wrapKeyAndValues(keysAndValues)...),
	}
}

// WithName implements logr.LogSink.
func (k *KubeAwareLogSink) WithName(name string) logr.LogSink {
	return &KubeAwareLogSink{
		kubeAwareEnabled: k.kubeAwareEnabled,
		sink:             k.sink.WithName(name),
	}
}

// KubeAwareEnabled return kube aware logging is enabled or not.
func (k *KubeAwareLogSink) KubeAwareEnabled() bool {
	return k.kubeAwareEnabled.Load()
}

// SetKubeAwareEnabled could update the kube aware logging flag.
func (k *KubeAwareLogSink) SetKubeAwareEnabled(enabled bool) {
	k.kubeAwareEnabled.Store(enabled)
}

// wrapKeyAndValues would replace the kubernetes objects with wrappers.
func (k *KubeAwareLogSink) wrapKeyAndValues(keysAndValues []interface{}) []interface{} {
	result := make([]interface{}, len(keysAndValues))
	for i, item := range keysAndValues {
		if i%2 == 0 {
			// item is key, no need to resolve
			result[i] = item
			continue
		}

		switch val := item.(type) {
		case runtime.Object:
			result[i] = &kubeObjectWrapper{obj: val}
		case types.NamespacedName:
			result[i] = &namespacedNameWrapper{NamespacedName: val}
		default:
			result[i] = item
		}
	}
	return result
}
