package tracing

import (
	"context"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/api/trace"
	"go.opentelemetry.io/otel/label"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

// FromObject takes a Kubernetes objects and returns a Span from the
// context found in its annotations, or nil if not found;
// also a logger connected to the span and a new Context set up with both.
func FromObject(ctx context.Context, operationName string, obj runtime.Object) (context.Context, trace.Span, logr.Logger) {
	log := ctrl.LoggerFrom(ctx)
	m, err := meta.Accessor(obj)
	if err != nil {
		return nil, nil, log
	}
	ctx, sp := SpanFromAnnotations(ctx, operationName, m.GetAnnotations())
	if sp == nil {
		return nil, nil, log
	}
	sp.SetAttributes(label.String("objectKey", m.GetNamespace()+"/"+m.GetName()))
	log = tracingLogger{Logger: log, Span: sp}
	ctx = ctrl.LoggerInto(ctx, log)
	return ctx, sp, log
}
