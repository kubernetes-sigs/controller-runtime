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
	"fmt"
	"os"
	"sync"

	"github.com/go-logr/logr"
)

// NB: this is the same as the null logger logr/testing,
// but avoids accidentally adding the testing flags to
// all binaries.

// NullLogger is a logr.Logger that does nothing.
type NullLogger struct {
	warnOnce
}

// warnOnce warn once when NullLogger is called
type warnOnce struct {
	once      *sync.Once
	warnPrint string
}

func (w warnOnce) do() {
	w.once.Do(func() {
		fmt.Fprintln(os.Stdin, w.warnPrint)
	})
}

var _ logr.Logger = NullLogger{}

// DefaultNullLogger a default NullLogger with specified warn print
func DefaultNullLogger() NullLogger {
	w := warnOnce{
		once:      &sync.Once{},
		warnPrint: "warning: log.SetLogger was not called.",
	}
	return NullLogger{warnOnce: w}
}

// Info implements logr.InfoLogger
func (log NullLogger) Info(_ string, _ ...interface{}) {
	log.do()
}

// Enabled implements logr.InfoLogger
func (log NullLogger) Enabled() bool {
	return false
}

// Error implements logr.Logger
func (log NullLogger) Error(_ error, _ string, _ ...interface{}) {
	log.do()
}

// V implements logr.Logger
func (log NullLogger) V(_ int) logr.InfoLogger {
	return log
}

// WithName implements logr.Logger
func (log NullLogger) WithName(_ string) logr.Logger {
	return log
}

// WithValues implements logr.Logger
func (log NullLogger) WithValues(_ ...interface{}) logr.Logger {
	return log
}
