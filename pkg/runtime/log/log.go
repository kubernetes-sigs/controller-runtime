// Package log contains utilities for fetching a new logger
// when one is not already available.
package log

import (
	"log"

	"github.com/go-logr/logr"
	tlogr "github.com/go-logr/logr/testing"
	"github.com/go-logr/zapr"
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
	// who watches the watchmen?
	fatalIfErr(err, log.Fatalf)
	return zapr.NewLogger(zapLog)
}

func fatalIfErr(err error, f func(format string, v ...interface{})) {
	if err != nil {
		f("unable to construct the logger: %v", err)
	}
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
