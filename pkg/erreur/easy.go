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

// CauseSetter is some error that can have a cause set.
// It's like the opposite of errors.Wrapper.
type CauseSetter interface {
	// CausedBy marks that this error is caused by the given
	// error.  It always returns itself for convinience.
	CausedBy(err error) error
}

// ErrorWithCause is an Error whose cause can be set (i.e.
// also a CauseSetter).
type ErrorWithCause interface {
	Error
	CauseSetter
}

// easyError is similar to errorImpl, except that it directly
// takes the set of pairs to save (meaning one less allocation
// per new error).
type easyError struct {
	errorMixin
	pairs []interface{}
}

func (e *easyError) ValuesInto(out ValueReceiver) {
	for i := 0; i < len(e.pairs); i+=2 {
		out.Pair(e.pairs[i].(string), e.pairs[i+1])
	}

	if e.wrapped == nil {
		return
	}

	next, isStructured := e.wrapped.(Structured)
	if !isStructured {
		return
	}

	next.ValuesInto(out)
}

func (e *easyError) FormatError(p errors.Printer) (next error) {
	p.Print(e.msg)
	e.frame.Format(p)
	e.ValuesInto(PrintingReceiver{Printer: p})
	return e.wrapped
}

// WithValues returns a new Error with the given message and key-value
// pairs (like logr).  Wrap with CausedBy(cause, thisError) to set a cause.
func WithValues(msg string, keysAndValues ...interface{}) ErrorWithCause {
	// don't just call from base b/c we'd need an extra caller skip
	return &easyError{
		errorMixin: errorMixin{
			msg: msg,
			frame: maybeTrace(),
		},
		pairs: keysAndValues,
	}
}

// CausedBy sets the cause of the first error to the second error,
// if the first error is a CauseSetter.  It always returns the first
// error.
func CausedBy(newError, cause error) error {
	if setter, canSet := newError.(CauseSetter); canSet {
		setter.CausedBy(cause)
		return newError
	}
	
	return newError
}
