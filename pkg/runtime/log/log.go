// Package log contains utilities for fetching a new logger
// when one is not already available.
package log

import (
	"log"

	"github.com/thockin/logr"
	"github.com/thockin/logr/impls/zaplogr"
	tlogr "github.com/thockin/logr/testing"
	"go.uber.org/zap"
)

// ZapLogger is a Logger implementation.
// if development is true stack traces will be printed for errors
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

// SetLogger sets a concrete logging implementation for all deferred Loggers.
func SetLogger(l logr.Logger) {
	if Log.promise != nil {
		Log.promise.Fulfill(l)
	}
}

// Log is the base logger used by kubebuilder.  It delegates
// to another logr.Logger.  You *must* call SetLogger to
// get any actual logging.
var Log = &DelegatingLogger{
	Logger:  tlogr.NullLogger{},
	promise: &loggerPromise{},
}

// KBLog is a base parent logger.
var KBLog logr.Logger

func init() {
	Log.promise.logger = Log
	KBLog = Log.WithName("kubebuilder")
}
