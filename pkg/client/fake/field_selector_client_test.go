/*
Copyright 2020 The Kubernetes Authors.

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

package fake

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestListByFieldSelector(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
		},
		Spec: corev1.PodSpec{
			NodeName:           "node1",
			ServiceAccountName: "sa1",
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
		},
		Spec: corev1.NodeSpec{
			PodCIDR:    "cidr1",
			ProviderID: "provider1",
		},
	}

	fakeClient := NewFakeClientWithScheme(scheme.Scheme, pod, node)

	podIndexers := map[string]client.IndexerFunc{
		"spec.nodeName": func(o runtime.Object) []string {
			po, ok := o.(*corev1.Pod)
			if !ok {
				return nil
			}

			return []string{po.Spec.NodeName}
		},
		"spec.serviceAccountName": func(o runtime.Object) []string {
			po, ok := o.(*corev1.Pod)
			if !ok {
				return nil
			}

			return []string{po.Spec.ServiceAccountName}
		},
	}

	nodeIndexers := map[string]client.IndexerFunc{
		"spec.podCIDR": func(o runtime.Object) []string {
			no, ok := o.(*corev1.Node)
			if !ok {
				return nil
			}

			return []string{no.Spec.PodCIDR}
		},
		"spec.providerID": func(o runtime.Object) []string {
			no, ok := o.(*corev1.Node)
			if !ok {
				return nil
			}

			return []string{no.Spec.ProviderID}
		},
	}

	fieldSelectorClient, err := NewFieldSelectorEnableClient(fakeClient, scheme.Scheme, &corev1.Pod{}, podIndexers)
	if err != nil {
		t.Fatalf("unexpect init client err: %v", err)
	}

	fieldSelectorClient, err = NewFieldSelectorEnableClient(fieldSelectorClient, scheme.Scheme, &corev1.Node{}, nodeIndexers)
	if err != nil {
		t.Fatalf("unexpect init client err: %v", err)
	}

	cases := []struct {
		obj                runtime.Object
		name               string
		opts               client.ListOption
		expectObjectsCount int
	}{
		{
			name:               "list Pod using nil fieldSelector",
			obj:                &corev1.PodList{},
			opts:               nil,
			expectObjectsCount: 1,
		},
		{
			name:               "list Node using nil fieldSelector",
			obj:                &corev1.NodeList{},
			opts:               nil,
			expectObjectsCount: 1,
		},
		{
			name: "list Pod does not match fieldSelector name",
			obj:  &corev1.PodList{},
			opts: client.MatchingFields(fields.Set{
				"spec.nodeName1": "node1",
			}),
			expectObjectsCount: 0,
		},
		{
			name: "list Node does not match fieldSelector name",
			obj:  &corev1.NodeList{},
			opts: client.MatchingFields(fields.Set{
				"spec.podCIDR1": "node1",
			}),
			expectObjectsCount: 0,
		},
		{
			name: "list Pod does not match fieldSelector value",
			obj:  &corev1.PodList{},
			opts: client.MatchingFields(fields.Set{
				"spec.nodeName": "node2",
			}),
			expectObjectsCount: 0,
		},
		{
			name: "list Node does not match fieldSelector value",
			obj:  &corev1.NodeList{},
			opts: client.MatchingFields(fields.Set{
				"spec.podCIDR": "bar2",
			}),
			expectObjectsCount: 0,
		},
		{
			name: "list Pod match fieldSelector one name and value",
			obj:  &corev1.PodList{},
			opts: client.MatchingFields(fields.Set{
				"spec.nodeName": "node1",
			}),
			expectObjectsCount: 1,
		},
		{
			name: "list Node match fieldSelector one name and value",
			obj:  &corev1.NodeList{},
			opts: client.MatchingFields(fields.Set{
				"spec.podCIDR": "cidr1",
			}),
			expectObjectsCount: 1,
		},
		{
			name: "list Pod match fieldSelector one name and value, and does not match one value",
			obj:  &corev1.PodList{},
			opts: client.MatchingFields(fields.Set{
				"spec.nodeName":           "node1",
				"spec.serviceAccountName": "sa2",
			}),
			expectObjectsCount: 0,
		},
		{
			name: "list Node match fieldSelector one name and value, and does not match one value",
			obj:  &corev1.NodeList{},
			opts: client.MatchingFields(fields.Set{
				"spec.podCIDR":    "cidr1",
				"spec.providerID": "provider2",
			}),
			expectObjectsCount: 0,
		},
		{
			name: "list Pod match fieldSelector two name and value",
			obj:  &corev1.PodList{},
			opts: client.MatchingFields(fields.Set{
				"spec.nodeName":           "node1",
				"spec.serviceAccountName": "sa1",
			}),
			expectObjectsCount: 1,
		},
		{
			name: "list Node match fieldSelector two name and value",
			obj:  &corev1.NodeList{},
			opts: client.MatchingFields(fields.Set{
				"spec.podCIDR":    "cidr1",
				"spec.providerID": "provider1",
			}),
			expectObjectsCount: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.opts != nil {
				if err := fieldSelectorClient.List(context.TODO(), tc.obj, tc.opts); err != nil {
					t.Fatalf("unexpect list err: %v", err)
				}
			} else {
				if err := fieldSelectorClient.List(context.TODO(), tc.obj); err != nil {
					t.Fatalf("unexpect list err: %v", err)
				}
			}

			objs, err := meta.ExtractList(tc.obj)
			if err != nil {
				t.Fatalf("unexpect extra list err: %v", err)
			}

			if len(objs) != tc.expectObjectsCount {
				t.Errorf("expect list object result count: %d, got: %d", tc.expectObjectsCount, len(objs))
			}
		})
	}
}
