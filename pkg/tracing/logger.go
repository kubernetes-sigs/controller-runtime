package tracing

import (
	"context"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/api/trace"
	"go.opentelemetry.io/otel/label"
)

type tracingLogger struct {
	logr.Logger
	trace.Span
}

func (t tracingLogger) Enabled() bool {
	return t.Logger.Enabled()
}

func (t tracingLogger) Info(msg string, keysAndValues ...interface{}) {
	t.Logger.Info(msg, keysAndValues...)
	t.Span.AddEvent(context.Background(), "info", keyValues(keysAndValues...)...)
}

func (t tracingLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	t.Logger.Error(err, msg, keysAndValues...)
	kvs := append([]label.KeyValue{label.String("message", msg)}, keyValues(keysAndValues...)...)
	t.Span.AddEvent(context.Background(), "error", kvs...)
	t.Span.RecordError(context.Background(), err)
}

func (t tracingLogger) V(level int) logr.Logger {
	return tracingLogger{Logger: t.Logger.V(level), Span: t.Span}
}

func keyValues(keysAndValues ...interface{}) []label.KeyValue {
	attrs := make([]label.KeyValue, 0, len(keysAndValues)/2)
	for i := 0; i+1 < len(keysAndValues); i += 2 {
		key, ok := keysAndValues[i].(string)
		if !ok {
			key = "non-string"
		}
		attrs = append(attrs, label.Any(key, keysAndValues[i+1]))
	}
	return attrs
}

func (t tracingLogger) WithValues(keysAndValues ...interface{}) logr.Logger {
	t.Span.SetAttributes(keyValues(keysAndValues...)...)
	return tracingLogger{Logger: t.Logger.WithValues(keysAndValues...), Span: t.Span}
}

func (t tracingLogger) WithName(name string) logr.Logger {
	t.Span.SetAttributes(label.String("name", name))
	return tracingLogger{Logger: t.Logger.WithName(name), Span: t.Span}
}
