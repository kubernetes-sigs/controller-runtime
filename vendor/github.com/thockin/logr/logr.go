// Package logr defines abstract interfaces for logging.  Packages can depend on
// these interfaces and callers can implement logging in whatever way is
// appropriate.
//
// This design derives from Dave Cheney's blog:
//     http://dave.cheney.net/2015/11/05/lets-talk-about-logging
//
// This is a BETA grade API.  Until there is a significant 2nd implementation,
// I don't really know how it will change.
package logr

// TODO: consider adding back in format strings if they're really needed
// TODO: consider other bits of zap/zapcore functionality like ObjectMarshaller (for arbitrary objects)
// TODO: consider other bits of glog functionality like Flush, InfoDepth, OutputStats

// InfoLogger represents the ability to log non-error messages.
type InfoLogger interface {
	// Info logs a non-error message with the given key/value pairs as context.
	Info(msg string, keysAndValues ...interface{})

	// Enabled test whether this InfoLogger is enabled.  For example,
	// commandline flags might be used to set the logging verbosity and disable
	// some info logs.
	Enabled() bool
}

// Logger represents the ability to log messages, both errors and not.
type Logger interface {
	// All Loggers implement InfoLogger.  Calling InfoLogger methods directly on
	// a Logger value is equivalent to calling them on a V(0) InfoLogger.  For
	// example, logger.Info() produces the same result as logger.V(0).Info.
	InfoLogger

	// Error logs an error, with the given message and key/value pairs as context.
	Error(err error, msg string, keysAndValues ...interface{})

	// V returns an InfoLogger value for a specific verbosity level.  A higher
	// verbosity level means a log message is less important.
	V(level int) InfoLogger

	// WithTags adds some key-value pairs of context to a logger.
	WithTags(keysAndValues ...interface{}) Logger

	// WithName adds a new prefix to the logger's name.
	WithName(name string) Logger
}
