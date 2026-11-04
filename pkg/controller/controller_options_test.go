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

package controller_test

import (
	"fmt"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	internalcontroller "sigs.k8s.io/controller-runtime/pkg/internal/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// TestNewUnmanagedClampsMaxConcurrentReconciles pins the defensive clamp for
// callers that build a controller without a manager.
//
// Through controller.New the value is filled from the manager config before it
// gets here, so the clamp is only reachable via NewUnmanaged. It matters
// because Start spawns exactly MaxConcurrentReconciles workers: at zero the
// controller starts, logs its worker count, and reconciles nothing, and at a
// negative count WaitGroup.Add panics.
func TestNewUnmanagedClampsMaxConcurrentReconciles(t *testing.T) {
	for i, requested := range []int{0, -1, -7} {
		// Controller names are unique per process, hence the index.
		c, err := controller.NewUnmanaged(fmt.Sprintf("unmanaged-clamp-%d", i), controller.Options{
			Reconciler:              reconcile.Func(nil),
			MaxConcurrentReconciles: requested,
		})
		if err != nil {
			t.Fatalf("NewUnmanaged(%d): %v", requested, err)
		}

		internal, ok := c.(*internalcontroller.Controller[reconcile.Request])
		if !ok {
			t.Fatalf("NewUnmanaged(%d): unexpected controller type %T", requested, c)
		}
		if internal.MaxConcurrentReconciles != 1 {
			t.Errorf("MaxConcurrentReconciles = %d after requesting %d, want 1",
				internal.MaxConcurrentReconciles, requested)
		}
	}
}
