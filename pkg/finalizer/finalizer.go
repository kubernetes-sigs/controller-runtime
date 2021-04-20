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

package finalizer

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// NewFinalizers returns the Finalizers interface
func NewFinalizers() Finalizers {
	return finalizers(map[string]Finalizer{})
}

func (f finalizers) Register(key string, finalizer Finalizer) error {
	if _, ok := f[key]; ok {
		return fmt.Errorf("finalizer for key %q already registered", key)
	}
	f[key] = finalizer
	return nil
}

func (f finalizers) Finalize(ctx context.Context, obj client.Object) (bool, error) {
	needsUpdate := false
	for key, finalizer := range f {
		if obj.GetDeletionTimestamp().IsZero() && !controllerutil.ContainsFinalizer(obj, key) {
			controllerutil.AddFinalizer(obj, key)
			needsUpdate = true
		} else if obj.GetDeletionTimestamp() != nil && controllerutil.ContainsFinalizer(obj, key) {
			ret, err := finalizer.Finalize(ctx, obj)
			if err != nil {
				return ret, fmt.Errorf("finalize failed for key %q: %v", key, err)
			}
			controllerutil.RemoveFinalizer(obj, key)
			needsUpdate = true
		}
	}
	return needsUpdate, nil
}
