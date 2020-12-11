package tracing

import (
	"context"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/api/trace"
	corev1 "k8s.io/api/core/v1"
)

func TestInjectExtract(t *testing.T) {
	tracingCloser, err := SetupOTLP("some-controller")
	if err != nil {
		log.Fatalf("failed to set up Jaeger: %v", err)
	}
	defer tracingCloser.Close()

	var testNode corev1.Node
	ctx, sp := global.Tracer("test").Start(context.Background(), "foo")

	err = AddTraceAnnotationToObject(ctx, &testNode)
	assert.NoError(t, err)
	{
		ctx := spanContextFromAnnotations(context.Background(), testNode.Annotations)
		assert.NoError(t, err)
		sc := trace.RemoteSpanContextFromContext(ctx)
		assert.Equal(t, sp.SpanContext(), sc)
	}

	// Check that adding a different span leaves the original in place
	ctx, sp2 := global.Tracer("test").Start(ctx, "bar")

	err = AddTraceAnnotationToObject(ctx, &testNode)
	assert.NoError(t, err)
	{
		ctx := spanContextFromAnnotations(context.Background(), testNode.Annotations)
		assert.NoError(t, err)
		sc := trace.RemoteSpanContextFromContext(ctx)
		assert.Equal(t, sp.SpanContext(), sc)
		assert.NotEqual(t, sp2.SpanContext(), sc)
	}
}
