/*
Copyright 2025 The Kubernetes Authors.

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

// Package logr contains helpers for setting up a Kubernetes object aware logr.Logger.
package logr

import (
	"reflect"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ logr.LogSink = (*KubeAwareSink)(nil)

// KubeAwareSink is a logr.LogSink that understands Kubernetes objects.
type KubeAwareSink struct {
	sink logr.LogSink
}

// KubeAware wraps a logr.logger to make it aware of Kubernetes objects.
func KubeAware(logger logr.Logger) logr.Logger {
	return logr.New(
		&KubeAwareSink{logger.GetSink()},
	)
}

// Init implements logr.LogSink.
func (k *KubeAwareSink) Init(info logr.RuntimeInfo) {
	k.sink.Init(info)
}

// Enabled implements logr.LogSink.
func (k *KubeAwareSink) Enabled(level int) bool {
	return k.sink.Enabled(level)
}

// Info implements logr.LogSink.
func (k *KubeAwareSink) Info(level int, msg string, keysAndValues ...interface{}) {
	k.sink.Info(level, msg, k.wrapKeyAndValues(keysAndValues)...)
}

// Error implements logr.LogSink.
func (k *KubeAwareSink) Error(err error, msg string, keysAndValues ...interface{}) {
	k.sink.Error(err, msg, k.wrapKeyAndValues(keysAndValues)...)
}

// WithValues implements logr.LogSink.
func (k *KubeAwareSink) WithValues(keysAndValues ...interface{}) logr.LogSink {
	return &KubeAwareSink{
		sink: k.sink.WithValues(k.wrapKeyAndValues(keysAndValues)...),
	}
}

// WithName implements logr.LogSink.
func (k *KubeAwareSink) WithName(name string) logr.LogSink {
	return &KubeAwareSink{
		sink: k.sink.WithName(name),
	}
}

// wrapKeyAndValues replaces Kubernetes objects with [kubeObjectWrapper].
func (k *KubeAwareSink) wrapKeyAndValues(keysAndValues []interface{}) []interface{} {
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
		default:
			result[i] = item
		}
	}
	return result
}

var _ logr.Marshaler = (*kubeObjectWrapper)(nil)

// kubeObjectWrapper is a wrapper around runtime.Object that implements logr.Marshaler.
type kubeObjectWrapper struct {
	obj runtime.Object
}

// MarshalLog implements logr.Marshaler.
// The implementation mirrors the behavior of kubeObjectWrapper.MarshalLogObject.
func (w *kubeObjectWrapper) MarshalLog() interface{} {
	result := make(map[string]string)

	if reflect.ValueOf(w.obj).IsNil() {
		return "got nil for runtime.Object"
	}

	if gvk := w.obj.GetObjectKind().GroupVersionKind(); gvk.Version != "" {
		result["apiVersion"] = gvk.GroupVersion().String()
		result["kind"] = gvk.Kind
	}

	objMeta, err := meta.Accessor(w.obj)
	if err != nil {
		return result
	}

	if ns := objMeta.GetNamespace(); ns != "" {
		result["namespace"] = ns
	}
	result["name"] = objMeta.GetName()
	return result
}
