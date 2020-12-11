package tracing

import (
	"io"

	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/exporters/trace/jaeger"
	"go.opentelemetry.io/otel/propagators"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// SetupJaeger sets up Jaeger with some defaults
func SetupJaeger(serviceName string) (io.Closer, error) {
	// Create and install Jaeger export pipeline
	flush, err := jaeger.InstallNewPipeline(
		jaeger.WithCollectorEndpoint("http://jaeger-agent.default:14268/api/traces"), // FIXME name?
		jaeger.WithProcess(jaeger.Process{
			ServiceName: serviceName,
		}),
		jaeger.WithSDK(&sdktrace.Config{DefaultSampler: sdktrace.AlwaysSample()}),
	)
	if err != nil {
		return nil, err
	}

	// set global propagator to tracecontext (the default is no-op).
	global.SetTextMapPropagator(propagators.TraceContext{})

	return funcCloser{f: flush}, nil
}

type funcCloser struct {
	f func()
}

func (c funcCloser) Close() error {
	c.f()
	return nil
}
