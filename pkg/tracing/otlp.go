package tracing

import (
	"context"
	"io"

	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/exporters/otlp"
	"go.opentelemetry.io/otel/propagators"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv"
)

// SetupOTLP sets up a global trace provider sending to OpenTelemetry with some defaults
func SetupOTLP(serviceName string) (io.Closer, error) {
	exp, err := otlp.NewExporter(
		otlp.WithInsecure(),
		otlp.WithAddress("otlp-collector.default:55680"),
	)
	if err != nil {
		return nil, err
	}

	bsp := sdktrace.NewBatchSpanProcessor(exp)
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(resource.New(semconv.ServiceNameKey.String(serviceName))),
		sdktrace.WithSpanProcessor(bsp),
	)

	// set global propagator to tracecontext (the default is no-op).
	global.SetTextMapPropagator(propagators.TraceContext{})
	global.SetTracerProvider(tracerProvider)

	return otlpCloser{exp: exp, bsp: bsp}, nil
}

type otlpCloser struct {
	exp *otlp.Exporter
	bsp *sdktrace.BatchSpanProcessor
}

func (s otlpCloser) Close() error {
	s.bsp.Shutdown() // shutdown the processor
	return s.exp.Shutdown(context.Background())
}
