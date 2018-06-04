// Package log contains utilities for fetching a new logger
// when one is not already available.
package log

import (
	"log"

	"github.com/thockin/logr"
	tlogr "github.com/thockin/logr/testing"
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

func SetLogger(l logr.Logger) {
	Log.promise.Fulfill(l)
}

// Log is the base logger used by kubebuilder.  It delegates
// to another logr.Logger.  You *must* call SetLogger to
// get any actual logging.
var Log = &DelegatingLogger{
	Logger: tlogr.NullLogger{},
	promise: &loggerPromise{},
}

var KBLog logr.Logger

func init() {
	Log.promise.logger = Log
	KBLog = Log.WithName("kubebuilder")
}
