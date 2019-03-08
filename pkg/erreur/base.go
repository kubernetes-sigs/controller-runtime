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
// and all erreur errors will automatically inject their
// structured data into logs when using logr.
//
// All erreur errors also implement the prototype Go 2 error
// interfaces from golang.org/x/exp/errors
package erreur

// ValueReceiver is some sink that knows how to receive key-value
// pairs (e.g. from an Error).  It can be used to receive all key-value
// pairs from an Error for logging, to print all key-value pairs, etc.
type ValueReceiver interface {
	Pair(key string, val interface{})
}

// Error is an error with structured data attached.
type Error interface {
	error
	Structured
}

// Structured contains structured key-value pairs, and knows
// how to dump them into a ValueReceiver.
type Structured interface {
	ValuesInto(ValueReceiver)
}
