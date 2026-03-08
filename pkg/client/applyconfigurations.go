/*
Copyright 2025 The Kubernetes Authors.

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
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

func gvkFromApplyConfiguration(ac applyConfiguration) (schema.GroupVersionKind, error) {
	var gvk schema.GroupVersionKind
	gv, err := schema.ParseGroupVersion(ptr.Deref(ac.GetAPIVersion(), ""))
	if err != nil {
		return gvk, fmt.Errorf("failed to parse %q as GroupVersion: %w", ptr.Deref(ac.GetAPIVersion(), ""), err)
	}
	gvk.Group = gv.Group
	gvk.Version = gv.Version
	gvk.Kind = ptr.Deref(ac.GetKind(), "")

	return gvk, nil
}
