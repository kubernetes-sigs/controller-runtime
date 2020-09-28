package tracing

import (
	"bytes"
	"encoding/base64"

	ot "github.com/opentracing/opentracing-go"
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

// SpanFromAnnotations takes a map as found in Kubernetes objects and
// returns a Span from the context found there, or nil if not found.
func SpanFromAnnotations(name string, annotations map[string]string) (ot.Span, error) {
	value, found := annotations[TraceAnnotationKey]
	if !found {
		return nil, nil
	}
	return spanFromEmbeddableSpanContext(name, value)
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
