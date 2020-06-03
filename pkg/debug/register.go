/*
Copyright 2020 The Kubernetes Authors.

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

package debug

import (
	"net/http/pprof"

	"sigs.k8s.io/controller-runtime/pkg/httpserver"
)

// Options use to provide configuration option
type Options struct {
	CmdLine bool
	Profile bool
	Symbol  bool
	Trace   bool
}

// DefaultOptions returns default options configuration
func DefaultOptions() *Options {
	return &Options{
		CmdLine: true,
		Profile: true,
		Symbol:  true,
		Trace:   true,
	}
}

// Register use to register the different debug endpoint
func Register(mux httpserver.Server, options *Options) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	if options == nil {
		options = DefaultOptions()
	}
	if options.CmdLine {
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	}
	if options.Profile {
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	}
	if options.Symbol {
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	}
	if options.Trace {
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}
}
