// Package log contains utilities for fetching a new logger
// when one is not already available.
package log

import (
	"log"

	"github.com/thockin/logr"
	"github.com/thockin/logr/impls/zaplogr"
	"go.uber.org/zap"
)

func ZapLogger(development bool) logr.Logger {
	var zapLog *zap.Logger
	var err error
	if development {
		zapLogCfg := zap.NewDevelopmentConfig()
		zapLog, err = zapLogCfg.Build(zap.AddCallerSkip(1))
	} else {
		zapLogCfg := zap.NewProductionConfig()
		zapLog, err = zapLogCfg.Build(zap.AddCallerSkip(1))
	}
	if err != nil {
		// who watches the watchmen?
		log.Fatalf("unable to construct the logger: %v", err)
	}
	return zaplogr.NewLogger(zapLog)
}

// SetLogger configures the base logger used by kubebuilder
func SetLogger(l logr.Logger) {
	baseLogger = l
}

// BaseLogger returns the base logger used by kubebuilder.
// The default is a Zap-based one in a development configuration.
// It should not be called from package level (instead, its result
// should be captured when used).
func BaseLogger() logr.Logger {
	if baseLogger == nil {
		baseLogger = ZapLogger(true).WithName("kubebuilder")
	}

	return baseLogger
}

// BaseLogger sets the  the base logger
var baseLogger logr.Logger = nil
