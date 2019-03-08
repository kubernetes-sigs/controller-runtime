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

// Package erreur defines structured errors.
//
// The API is designed to feel very similar to logr,
// and is designed to be easy to handle by structured
// logging implementations (like zapr).
//
// All erreur errors produced by the helpers in this package
// also implement the prototype Go 2 error interfaces from
// golang.org/x/exp/errors (Formatter, Wrapper).
//
// If the CAPTURE_STACK_TRACES environment variable is set to
// `true` or `1`, it will automatically capture stack traces
// unless otherwise explicitly disabled.
package erreur
