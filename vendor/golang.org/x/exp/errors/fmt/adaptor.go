// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fmt

import "golang.org/x/exp/errors"

// The functionality in this file is to provide adaptors only. It will not
// be included in the standard library.

// FormatError calls the FormatError method of err with a errors.Printer
// configured according to s and verb and writes the result to s.
func FormatError(s State, verb rune, err errors.Formatter) {
	// Assuming this function is only called from the Format method, and given
	// that FormatError takes precedence over Format, it cannot be called from
	// any package that supports errors.Formatter. It is therefore safe to
	// disregard that State may be a specific printer implementation and use one
	// of our choice instead.
	p := newPrinter()
	if verb == 'v' {
		if s.Flag('#') {
			p.fmt.sharpV = true
		}
		if s.Flag('+') {
			p.fmt.plusV = true
		}
	}
	fmtError(p, verb, err)
	s.Write(p.buf)
}
