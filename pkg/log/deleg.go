/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package log

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
)

// loggerPromise knows how to populate a concrete logr.Logger
// with options, given an actual base logger later on down the line.
type loggerPromise struct {
	delegating    *delegatingLogSink
	childPromises []*loggerPromise
	promisesLock  sync.Mutex

	name *string
	tags []interface{}
}

func (p *loggerPromise) WithName(l *delegatingLogSink, name string) *loggerPromise {
	res := &loggerPromise{
		delegating: l,
		name:       &name,
	}

	p.promisesLock.Lock()
	defer p.promisesLock.Unlock()
	p.childPromises = append(p.childPromises, res)
	return res
}

// WithValues provides a new Logger with the tags appended.
func (p *loggerPromise) WithValues(l *delegatingLogSink, tags ...interface{}) *loggerPromise {
	res := &loggerPromise{
		delegating: l,
		tags:       tags,
	}

	p.promisesLock.Lock()
	defer p.promisesLock.Unlock()
	p.childPromises = append(p.childPromises, res)
	return res
}

// Fulfill instantiates the Logger with the provided logger.
func (p *loggerPromise) Fulfill(parentLogSink logr.LogSink) {
	sink := parentLogSink
	if p.name != nil {
		sink = sink.WithName(*p.name)
	}

	if p.tags != nil {
		sink = sink.WithValues(p.tags...)
	}

	p.delegating.logger.Store(&sink)
	if withCallDepth, ok := sink.(logr.CallDepthLogSink); ok {
		sinkWithCallDepth := withCallDepth.WithCallDepth(1)
		p.delegating.logger.Store(&sinkWithCallDepth)
	}
	p.delegating.promise.Store(nil)

	p.promisesLock.Lock()
	defer p.promisesLock.Unlock()
	for _, childPromise := range p.childPromises {
		childPromise.Fulfill(sink)
	}
}

// delegatingLogSink is a logsink that delegates to another logr.LogSink.
// If the underlying promise is not nil, it registers calls to sub-loggers with
// the logging factory to be populated later, and returns a new delegating
// logger.  It expects to have *some* logr.Logger set at all times (generally
// a no-op logger before the promises are fulfilled).
type delegatingLogSink struct {
	created time.Time

	info   atomic.Pointer[logr.RuntimeInfo]
	logger atomic.Pointer[logr.LogSink]

	fulfilled atomic.Bool
	promise   atomic.Pointer[loggerPromise]
}

// Init implements logr.LogSink.
func (l *delegatingLogSink) Init(info logr.RuntimeInfo) {
	eventuallyFulfillRoot()
	l.info.Store(&info)
}

// Enabled tests whether this Logger is enabled.  For example, commandline
// flags might be used to set the logging verbosity and disable some info
// logs.
func (l *delegatingLogSink) Enabled(level int) bool {
	eventuallyFulfillRoot()
	return (*l.logger.Load()).Enabled(level)
}

// Info logs a non-error message with the given key/value pairs as context.
//
// The msg argument should be used to add some constant description to
// the log line.  The key/value pairs can then be used to add additional
// variable information.  The key/value pairs should alternate string
// keys and arbitrary values.
func (l *delegatingLogSink) Info(level int, msg string, keysAndValues ...interface{}) {
	eventuallyFulfillRoot()
	(*l.logger.Load()).Info(level, msg, keysAndValues...)
}

// Error logs an error, with the given message and key/value pairs as context.
// It functions similarly to calling Info with the "error" named value, but may
// have unique behavior, and should be preferred for logging errors (see the
// package documentations for more information).
//
// The msg field should be used to add context to any underlying error,
// while the err field should be used to attach the actual error that
// triggered this log line, if present.
func (l *delegatingLogSink) Error(err error, msg string, keysAndValues ...interface{}) {
	eventuallyFulfillRoot()
	(*l.logger.Load()).Error(err, msg, keysAndValues...)
}

// WithName provides a new Logger with the name appended.
func (l *delegatingLogSink) WithName(name string) logr.LogSink {
	eventuallyFulfillRoot()

	switch {
	case l.promise.Load() != nil:
		res := &delegatingLogSink{}
		res.logger.Store(l.logger.Load())
		promise := l.promise.Load().WithName(res, name)
		res.promise.Store(promise)
		return res
	default:
		res := (*l.logger.Load()).WithName(name)
		if withCallDepth, ok := res.(logr.CallDepthLogSink); ok {
			res = withCallDepth.WithCallDepth(-1)
		}
		return res
	}
}

// WithValues provides a new Logger with the tags appended.
func (l *delegatingLogSink) WithValues(tags ...interface{}) logr.LogSink {
	eventuallyFulfillRoot()

	switch {
	case l.promise.Load() != nil:
		res := &delegatingLogSink{}
		res.logger.Store(l.logger.Load())
		promise := l.promise.Load().WithValues(res, tags...)
		res.promise.Store(promise)
		return res
	default:
		res := (*l.logger.Load()).WithValues(tags...)
		if withCallDepth, ok := res.(logr.CallDepthLogSink); ok {
			res = withCallDepth.WithCallDepth(-1)
		}
		return res
	}
}

// Fulfill switches the logger over to use the actual logger
// provided, instead of the temporary initial one, if this method
// has not been previously called.
func (l *delegatingLogSink) Fulfill(actual logr.LogSink) {
	if promise := l.promise.Load(); promise != nil {
		promise.Fulfill(actual)
		l.fulfilled.Store(true)
	}
}

// newDelegatingLogSink constructs a new DelegatingLogSink which uses
// the given logger before its promise is fulfilled.
func newDelegatingRoot(initial logr.LogSink) *delegatingLogSink {
	root := &delegatingLogSink{created: time.Now()}
	root.logger.Store(&initial)
	root.promise.Store(&loggerPromise{delegating: root})
	return root
}
