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

package controllerutil

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// SetControllerReference sets owner as a Controller OwnerReference on owned.
// This is used for garbage collection of the owned object and for
// reconciling the owner object on changes to owned (with a Watch + EnqueueRequestForOwner).
func SetControllerReference(owner, object v1.Object, scheme *runtime.Scheme) error {
	ro, ok := owner.(runtime.Object)
	if !ok {
		return fmt.Errorf("is not a %T a runtime.Object, cannot call SetControllerReference", owner)
	}

	gvk, err := apiutil.GVKForObject(ro, scheme)
	if err != nil {
		return err
	}

	// Create a new ref
	ref := *v1.NewControllerRef(owner, schema.GroupVersionKind{Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind})

	// Add it to the child
	object.SetOwnerReferences(append(object.GetOwnerReferences(), ref))
	return nil
}
