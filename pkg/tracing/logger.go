package tracing

import (
	"github.com/go-logr/logr"
	ot "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

type tracingLogger struct {
	logr.Logger
	ot.Span
}

func (t tracingLogger) Enabled() bool {
	return t.Logger.Enabled()
}

func (t tracingLogger) Info(msg string, keysAndValues ...interface{}) {
	t.Logger.Info(msg, keysAndValues...)
	t.Span.LogKV(append([]interface{}{"info", msg}, keysAndValues...)...)
}

func (t tracingLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	t.Logger.Error(err, msg, keysAndValues...)
	t.Span.LogKV(append([]interface{}{"error", err, "message", msg}, keysAndValues...)...)
	ext.Error.Set(t.Span, true)
}

func (t tracingLogger) V(level int) logr.Logger {
	return tracingLogger{Logger: t.Logger.V(level), Span: t.Span}
}

func (t tracingLogger) WithValues(keysAndValues ...interface{}) logr.Logger {
	for i := 0; i+1 < len(keysAndValues); i += 2 {
		key, ok := keysAndValues[i].(string)
		if !ok {
			continue
		}
		t.Span.SetTag(key, keysAndValues[i+1])
	}
	return tracingLogger{Logger: t.Logger.WithValues(keysAndValues...), Span: t.Span}
}

func (t tracingLogger) WithName(name string) logr.Logger {
	t.Span.SetTag("name", name)
	return tracingLogger{Logger: t.Logger.WithName(name), Span: t.Span}
}
