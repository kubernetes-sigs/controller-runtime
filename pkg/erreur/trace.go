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
	"os"
	"sync/atomic"

	"golang.org/x/exp/errors"
)

var (
	// captureTrace is effectively an "atomic bool"
	captureTrace uint32
	// traceEnvVar is the environment variable that controls
	// stack trace capturing on start-up.
	traceEnvVar = "CAPTURE_STACK_TRACES"
)

func init() {
	envVarVal := os.Getenv(traceEnvVar)
	if envVarVal == "true" || envVarVal == "1" {
		CaptureStackTraces()
	}
}

func CaptureStackTraces() {
	atomic.StoreUint32(&captureTrace, 1)
}

func DontCaptureStackTraces() {
	atomic.StoreUint32(&captureTrace, 0)
}

func maybeTrace() errors.Frame {
	if atomic.LoadUint32(&captureTrace) == 0 {
		return errors.Frame{}
	}

	return errors.Caller(2)
}
