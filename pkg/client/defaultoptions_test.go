/*
Copyright The Kubernetes Authors.

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

package client

import (
	"testing"
)

// nopClient is a bare Client used only for identity comparison; its methods are
// never called in this test.
type nopClient struct{ Client }

// TestInjectedOptionsCollapsesWrapping asserts that wrapping an existing
// *clientWithOptions extends it in place rather than nesting another wrapper,
// so the underlying client is reached directly.
func TestInjectedOptionsCollapsesWrapping(t *testing.T) {
	base := &nopClient{}
	c := NewDryRunClient(WithFieldOwner(base, "owner"))

	wrapped, ok := c.(*clientWithOptions)
	if !ok {
		t.Fatalf("got %T, want *clientWithOptions", c)
	}
	if wrapped.client != base {
		t.Errorf("underlying client = %T, want the base client directly (no nested wrapper)", wrapped.client)
	}
}
