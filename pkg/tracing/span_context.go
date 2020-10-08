package tracing

import (
	"bytes"
	"context"
	"encoding/base64"

	"github.com/go-logr/logr"
	ot "github.com/opentracing/opentracing-go"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

// TraceAnnotationKey is where we store span contexts in Kubernetes annotations
const TraceAnnotationKey string = "trace.kubernetes.io/context"

// GenerateEmbeddableSpanContext takes a Span and returns a serialized string
func GenerateEmbeddableSpanContext(span ot.Span) (string, error) {
	var buf bytes.Buffer
	if err := ot.GlobalTracer().Inject(span.Context(), ot.Binary, &buf); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// FromObject takes a Kubernetes objects and returns a Span from the
// context found in its annotations, or nil if not found;
// also a logger connected to the span and a new Context set up with both.
func FromObject(ctx context.Context, operationName string, obj runtime.Object) (context.Context, ot.Span, logr.Logger) {
	log := ctrl.LoggerFrom(ctx)
	m, err := meta.Accessor(obj)
	if err != nil {
		return nil, nil, log
	}
	value, found := m.GetAnnotations()[TraceAnnotationKey]
	if !found {
		return nil, nil, log
	}
	sp, err := spanFromEmbeddableSpanContext(operationName, value)
	if err != nil || sp == nil {
		return nil, nil, log
	}
	sp.SetTag("objectKey", m.GetNamespace()+"/"+m.GetName())
	log = tracingLogger{Logger: log, Span: sp}
	ctx = ctrl.LoggerInto(ot.ContextWithSpan(ctx, sp), log)
	return ctx, sp, log
}

func spanFromEmbeddableSpanContext(name, value string) (ot.Span, error) {
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	spanContext, err := ot.GlobalTracer().Extract(ot.Binary, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	return ot.StartSpan(name, ot.FollowsFrom(spanContext)), nil
}
