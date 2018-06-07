package zaplogr

import (
	"github.com/thockin/logr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// noopInfoLogger is a logr.InfoLogger that's always disabled, and does nothing
type noopInfoLogger struct{}

func (l *noopInfoLogger) Enabled() bool                   { return false }
func (l *noopInfoLogger) Info(_ string, _ ...interface{}) {}

var disabledInfoLogger = &noopInfoLogger{}

// TODO: special core to allow tag-based filtering
// TODO: non-sugared logger?
type infoLogger struct {
	lvl zapcore.Level
	l   *zap.Logger
}

func (l *infoLogger) Enabled() bool { return true }
func (l *infoLogger) Info(msg string, keysAndVals ...interface{}) {
	if checkedEntry := l.l.Check(l.lvl, msg); checkedEntry != nil {
		checkedEntry.Write(handleFields(l.l, keysAndVals)...)
	}
}

type zapLogger struct {
	// NB: this looks very similar to zap.SugaredLogger, but
	// deals with our desire to have multiple verbosity levels.
	l *zap.Logger
	infoLogger
}

func handleFields(l *zap.Logger, args []interface{}, additional ...zap.Field) []zap.Field {
	// a slightly modified version of zap.SugaredLogger.sweetenFields
	if len(args) == 0 {
	}

	// unlike zap, we can be pretty sure users aren't passing structured
	// fields (since logr has no concept of that), so guess that we need a
	// little less space.
	fields := make([]zap.Field, 0, len(args)/2+len(additional))
	for i := 0; i < len(args); {
		// check just in case for strongly-typed fields
		if asField, ok := args[i].(zap.Field); ok {
			fields = append(fields, asField)
			i++
			continue
		}

		// make sure this isn't a mismatched key
		if i == len(args)-1 {
			l.DPanic("odd number of arguments passed as key-value pairs for logging", zap.Any("ignored key", args[i]))
			break
		}

		// process a key-value pair,
		// ensuring that the key is a string
		key, val := args[i], args[i+1]
		keyStr, isString := key.(string)
		if !isString {
			// if the key isn't a string, DPanic and stop logging
			l.DPanic("non-string key argument passed to logging, ignoring all later arguments", zap.Any("invalid key", key))
			break
		}

		fields = append(fields, zap.Any(keyStr, val))
		i += 2
	}

	return append(fields, additional...)
}

func (l *zapLogger) Error(err error, msg string, keysAndVals ...interface{}) {
	// TODO: avoid the extra with without re-allocating the slice?
	if checkedEntry := l.l.Check(zap.ErrorLevel, msg); checkedEntry != nil {
		checkedEntry.Write(handleFields(l.l, keysAndVals, zap.Error(err))...)
	}
}

func (l *zapLogger) V(level int) logr.InfoLogger {
	lvl := zapcore.Level(-1 * level)
	if l.l.Core().Enabled(lvl) {
		return &infoLogger{
			lvl: lvl,
			l:   l.l,
		}
	}
	return disabledInfoLogger
}

func (l *zapLogger) WithTags(keysAndValues ...interface{}) logr.Logger {
	newLogger := l.l.With(handleFields(l.l, keysAndValues)...)
	return NewLogger(newLogger)
}

func (l *zapLogger) WithName(name string) logr.Logger {
	newLogger := l.l.Named(name)
	return NewLogger(newLogger)
}

func NewLogger(l *zap.Logger) logr.Logger {
	return &zapLogger{
		l: l,
		infoLogger: infoLogger{
			l:   l,
			lvl: zap.InfoLevel,
		},
	}
}
