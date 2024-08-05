package testhelpers

import (
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/onsi/ginkgo/v2"
)

// GetLogger configures a logger that's suitable for testing
func GetLogger() logr.Logger {
	testOut := zapcore.AddSync(ginkgo.GinkgoWriter)
	enc := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())

	zapLog := zap.New(zapcore.NewCore(enc, testOut, zap.DebugLevel),
		zap.ErrorOutput(testOut), zap.Development(), zap.AddStacktrace(zap.WarnLevel))

	return zapr.NewLogger(zapLog)
}
