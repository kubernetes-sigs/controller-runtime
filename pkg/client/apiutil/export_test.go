/*
Copyright 2026 The Kubernetes Authors.

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

package apiutil

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/utils/clock"
)

// The purpose of this file is to export a few private identifiers from package
// apiutil into package apiutil_test, so the envtest based RESTMapper tests can
// control when group versions that lookups missed in are fetched again.

// MissBackoffInitial is how long the mapper waits after the first miss in a
// group version before it fetches that group version again.
const MissBackoffInitial = missBackoffInitial

// SetMissBackoffClock sets the clock that a mapper returned by NewDynamicRESTMapper
// uses to back off group versions that lookups missed in. It must be called before
// the mapper is used.
func SetMissBackoffClock(m meta.RESTMapper, clk clock.Clock) {
	m.(*mapper).missBackoff.Clock = clk
}
