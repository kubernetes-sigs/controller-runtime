package store_test

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var testLog logr.Logger

func zapLogger() logr.Logger {
	testOut := zapcore.AddSync(GinkgoWriter)
	enc := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
	// bleh setting up logging to the ginkgo writer is annoying
	zapLog := zap.New(zapcore.NewCore(enc, testOut, zap.DebugLevel),
		zap.ErrorOutput(testOut), zap.Development(), zap.AddStacktrace(zap.WarnLevel))
	return zapr.NewLogger(zapLog)
}

func logCtx() context.Context {
	return logr.NewContext(context.Background(), testLog)
}

func TestStore(t *testing.T) {
	testLog = zapLogger()
	RegisterFailHandler(Fail)
	RunSpecs(t, "Store Suite")
}
