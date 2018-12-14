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

package patch_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	jpatch "github.com/evanphx/json-patch"
	"github.com/mattbaird/jsonpatch"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/patch"
)

type testcase struct {
	original    runtime.Object
	current     runtime.Object
	originalRaw []byte
	// expectedPatchesFromRaw is the expected json patches when using originalRaw
	expectedPatchesFromRaw []jsonpatch.JsonPatchOperation
	// expectedPatchesFromRaw is the expected json patches when NOT using originalRaw
	expectedPatches []jsonpatch.JsonPatchOperation
}

var testcases = []testcase{
	{
		original: &appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-deployment",
			},
		},
		current: &appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-deployment",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: func(x int32) *int32 { return &x }(1),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"foo": "bar",
					},
				},
				Template: corev1.PodTemplateSpec{},
				Strategy: appsv1.DeploymentStrategy{
					Type: appsv1.RecreateDeploymentStrategyType,
				},
			},
		},
		originalRaw: []byte(`{"apiVersion":"apps/v1", "kind":"Deployment", "metadata": {"creationTimestamp":null, "name": "test-deployment"}, "status": {}}`),
		expectedPatchesFromRaw: []jsonpatch.JsonPatchOperation{
			{
				Operation: "add",
				Path:      "/spec",
				Value: map[string]interface{}{
					"replicas": 1.0,
					"selector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"foo": "bar",
						},
					},
					"strategy": map[string]interface{}{
						"type": "Recreate",
					},
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"creationTimestamp": nil,
						},
						"spec": map[string]interface{}{
							"containers": nil,
						},
					},
				},
			},
		},
		expectedPatches: []jsonpatch.JsonPatchOperation{
			{
				Operation: "add",
				Path:      "/spec/replicas",
				// float64 will be used by default in the generated JSON patch
				Value: 1.0,
			},
			{
				Operation: "replace",
				Path:      "/spec/selector",
				Value: map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
			{
				Operation: "add",
				Path:      "/spec/strategy/type",
				Value:     "Recreate",
			},
		},
	},
}

func TestJSONPatchFromRaw(t *testing.T) {
	for _, tc := range testcases {
		patches, err := patch.NewJSONPatch(tc.original, tc.current, tc.originalRaw...)
		assertNoError(err, t)

		err = arrayEqual(tc.expectedPatchesFromRaw, patches)
		assertNoError(err, t)

		// Applying the patch to the original raw object should yield the desired object
		patchByte, err := json.Marshal(patches)
		assertNoError(err, t)

		p, err := jpatch.DecodePatch(patchByte)
		assertNoError(err, t)

		modified, err := p.Apply(tc.originalRaw)
		assertNoError(err, t)

		var appliedObject *appsv1.Deployment
		err = json.Unmarshal(modified, &appliedObject)
		assertNoError(err, t)

		if !reflect.DeepEqual(tc.current, appliedObject) {
			t.Fatalf("expect object:\n%#v,\nbut got:\n%#v", tc.current, appliedObject)
		}
	}
}

func TestJSONPatch(t *testing.T) {
	for _, tc := range testcases {
		patches, err := patch.NewJSONPatch(tc.original, tc.current)
		assertNoError(err, t)

		err = arrayEqual(tc.expectedPatches, patches)
		assertNoError(err, t)
	}
}

func arrayEqual(expected, got []jsonpatch.JsonPatchOperation) error {
	if len(expected) != len(got) {
		return fmt.Errorf("expected to have length %d, but got length %d", len(expected), len(got))
	}
	for _, e := range expected {
		found := false
		for _, g := range got {
			if e.Path == g.Path {
				if !reflect.DeepEqual(e, g) {
					return fmt.Errorf("expected: %#v,\nbut got: %#v", e, g)
				}
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("expected entry: %#v but didn't get it", e)
		}
	}
	return nil
}

func assertNoError(err error, t *testing.T) {
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
