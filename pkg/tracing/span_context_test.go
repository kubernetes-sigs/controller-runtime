package tracing

import (
	"log"
	"testing"

	ot "github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
)

func TestInjectExtract(t *testing.T) {
	tracingCloser, err := SetupJaeger("existingInfra-controller")
	if err != nil {
		log.Fatalf("failed to set up Jaeger: %v", err)
	}
	defer tracingCloser.Close()

	sp := ot.StartSpan("foo")
	val, err := GenerateEmbeddableSpanContext(sp)
	assert.NoError(t, err)
	sp2, err := spanFromEmbeddableSpanContext("bar", val)
	assert.NoError(t, err)
	assert.NotNil(t, sp2)
}
