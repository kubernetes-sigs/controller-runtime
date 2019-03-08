// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package errors implements functions to manipulate errors.
//
// This package implements the Go 2 draft designs for error inspection and printing:
//   https://go.googlesource.com/proposal/+/master/design/go2draft.md
//
// This is an EXPERIMENTAL package, and may change in arbitrary ways without notice.
package errors

// errorString is a trivial implementation of error.
type errorString struct {
	s     string
	frame Frame
}

// New returns an error that formats as the given text.
//
// The returned error embeds a Frame set to the caller's location and implements
// Formatter to show this information when printed with details.
func New(text string) error {
	return &errorString{text, Caller(1)}
}

func (e *errorString) Error() string {
	return e.s
}

func (e *errorString) FormatError(p Printer) (next error) {
	p.Print(e.s)
	e.frame.Format(p)
	return nil
}
