/*
Copyright 2021 The Kubernetes Authors.

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

package envtest

import (
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
)

// NewlineReporter is Reporter that Prints a newline after the default Reporter output so that the results
// are correctly parsed by test automation.
// See issue https://github.com/jstemmer/go-junit-report/issues/31
// It's re-exported here to avoid compatibility breakage/mass rewrites.
type NewlineReporter = printer.NewlineReporter
