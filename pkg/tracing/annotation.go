package tracing

import (
	"context"

	ot "github.com/opentracing/opentracing-go"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// AddTraceAnnotationToUnstructured adds an annotation encoding span's context to all objects
// Objects are modified in-place.
func AddTraceAnnotationToUnstructured(span ot.Span, objs []unstructured.Unstructured) error {
	spanContext, err := GenerateEmbeddableSpanContext(span)
	if err != nil {
		return err
	}

	for _, o := range objs {
		a := o.GetAnnotations()
		if a == nil {
			a = make(map[string]string)
		}
		a[TraceAnnotationKey] = spanContext
		o.SetAnnotations(a)
	}

	return nil
}

// AddTraceAnnotationToObject - if there is a span for the current context, and
// the object doesn't already have one set, adds it as an annotation
func AddTraceAnnotationToObject(ctx context.Context, obj runtime.Object) error {
	sp := ot.SpanFromContext(ctx)
	if sp == nil {
		return nil
	}
	spanContext, err := GenerateEmbeddableSpanContext(sp)
	if err != nil {
		return err
	}
	m, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	annotations := m.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{TraceAnnotationKey: spanContext}
	} else {
		// Don't overwrite if the caller already set one.
		if _, exists := annotations[TraceAnnotationKey]; !exists {
			annotations[TraceAnnotationKey] = spanContext
		}
	}
	m.SetAnnotations(annotations)
	return nil
}
