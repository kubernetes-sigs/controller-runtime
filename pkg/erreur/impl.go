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

package erreur

import (
	"golang.org/x/exp/errors"
)

// HasCaller is an error with a stack trace attached
type HasCaller interface {
	Caller() errors.Frame
}

// errorMixin is the base functionality shared by easyError and errorImpl.
// It contains implementations of methods having to do with message, error wrapping,
// and stack trace capturing.
type errorMixin struct {
	msg string
	wrapped error
	frame errors.Frame
}
func (e *errorMixin) Unwrap() error {
	return e.wrapped
}
func (e *errorMixin) Error() string {
	return e.msg
}
func (e *errorMixin) Caller() errors.Frame {
	return e.frame
}
func (e *errorMixin) CausedBy(err error) error {
	e.wrapped = err
	return e
}

// errorImpl is a wrapper that implements Error from some
// base primitives (Structured, plus potentially a message, caller, etc)
type errorImpl struct {
	structured Structured
	errorMixin
}

func (e *errorImpl) ValuesInto(out ValueReceiver) {
	e.structured.ValuesInto(out)
	if e.wrapped == nil {
		return
	}

	next, isStructured := e.wrapped.(Structured)
	if !isStructured {
		return
	}

	next.ValuesInto(out)
}

func (e *errorImpl) FormatError(p errors.Printer) (next error) {
	p.Print(e.msg)
	e.frame.Format(p)
	e.ValuesInto(PrintingReceiver{Printer: p})
	return e.wrapped
}

// WithStructured returns a new Error with the given message and structured
// data provided by keysAndValues.  Use this is you have some static
// data structure representing your error.
func WithStructured(msg string, keysAndValues Structured) ErrorWithCause {
	return &errorImpl{
		structured: keysAndValues,
		errorMixin: errorMixin{
			msg: msg,
			frame: maybeTrace(),
		},
	}
}
