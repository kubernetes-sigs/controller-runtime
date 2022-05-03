/*
Copyright 2022 The Kubernetes Authors.

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

package logr

import "k8s.io/apimachinery/pkg/types"

type namespacedNameWrapper struct {
	types.NamespacedName
}

func (w *namespacedNameWrapper) MarshalLog() interface{} {
	result := make(map[string]string)
	if w.Namespace != "" {
		result["namespace"] = w.Namespace
	}
	result["name"] = w.Name
	return result
}
