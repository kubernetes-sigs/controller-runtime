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

package predicate_test

import (
	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var p predicate.Predicate[*corev1.Pod]

// This example creates a new Predicate to drop Update Events where the Generation has not changed.
func ExampleFuncs() {
	p = predicate.Funcs[*corev1.Pod]{
		UpdateFunc: func(e event.UpdateEvent[*corev1.Pod]) bool {
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
		},
	}
}

func ExampleNewPredicateFuncs() {
	predicate.NewPredicateFuncs(func(obj *corev1.Pod) bool {
		// example ignoring deleted pods
		return obj.DeletionTimestamp.IsZero()
	})
}
