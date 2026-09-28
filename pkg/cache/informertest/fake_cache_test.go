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

package informertest

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	toolscache "k8s.io/client-go/tools/cache"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllertest"
)

func TestFakeInformersIndexField(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	informers := &FakeInformers{}
	informer, err := informers.FakeInformerFor(ctx, &corev1.Pod{})
	if err != nil {
		t.Fatalf("getting fake informer: %v", err)
	}
	informer.Add(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "one"}, Spec: corev1.PodSpec{NodeName: "node-a"}})
	informer.Add(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-b", Namespace: "two"}, Spec: corev1.PodSpec{NodeName: "node-a"}})
	informer.Add(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-c", Namespace: "one"}, Spec: corev1.PodSpec{NodeName: "node-b"}})

	index := func(obj client.Object) []string {
		return []string{obj.(*corev1.Pod).Spec.NodeName}
	}
	if err := informers.IndexField(ctx, &corev1.Pod{}, "spec.nodeName", index); err != nil {
		t.Fatalf("adding field index: %v", err)
	}

	var namespaced corev1.PodList
	if err := informers.List(ctx, &namespaced,
		client.InNamespace("one"), client.MatchingFields{"spec.nodeName": "node-a"}); err != nil {
		t.Fatalf("listing by namespaced field index: %v", err)
	}
	if len(namespaced.Items) != 1 || namespaced.Items[0].Name != "pod-a" {
		t.Fatalf("expected pod-a, got %#v", namespaced.Items)
	}

	var allNamespaces corev1.PodList
	if err := informers.List(ctx, &allNamespaces, client.MatchingFields{"spec.nodeName": "node-a"}); err != nil {
		t.Fatalf("listing by field index: %v", err)
	}
	if len(allNamespaces.Items) != 2 {
		t.Fatalf("expected two pods, got %#v", allNamespaces.Items)
	}
}

func TestFakeInformersIndexFieldRejectsDuplicate(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	informers := &FakeInformers{}
	index := func(client.Object) []string { return []string{"value"} }
	if err := informers.IndexField(ctx, &corev1.Pod{}, "spec.nodeName", index); err != nil {
		t.Fatalf("adding first field index: %v", err)
	}
	if err := informers.IndexField(ctx, &corev1.Pod{}, "spec.nodeName", index); err == nil {
		t.Fatal("expected duplicate field index to fail")
	}
}

func TestFakeInformersGetClusterScopedObject(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	informers := &FakeInformers{}
	informer, err := informers.FakeInformerFor(ctx, &corev1.Namespace{})
	if err != nil {
		t.Fatalf("getting fake informer: %v", err)
	}
	informer.Add(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "cluster-object"}})
	if err := informers.IndexField(ctx, &corev1.Namespace{}, "metadata.name", func(obj client.Object) []string {
		return []string{obj.GetName()}
	}); err != nil {
		t.Fatalf("adding cluster-scoped field index: %v", err)
	}

	var got corev1.Namespace
	if err := informers.Get(ctx, client.ObjectKey{Namespace: "ignored", Name: "cluster-object"}, &got); err != nil {
		t.Fatalf("getting cluster-scoped object: %v", err)
	}
	if got.Name != "cluster-object" {
		t.Fatalf("expected cluster-object, got %q", got.Name)
	}

	var list corev1.NamespaceList
	if err := informers.List(ctx, &list); err != nil {
		t.Fatalf("listing cluster-scoped objects: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Name != "cluster-object" {
		t.Fatalf("expected cluster-object in list, got %#v", list.Items)
	}

	var indexedList corev1.NamespaceList
	if err := informers.List(ctx, &indexedList, client.MatchingFields{"metadata.name": "cluster-object"}); err != nil {
		t.Fatalf("listing cluster-scoped objects by field: %v", err)
	}
	if len(indexedList.Items) != 1 || indexedList.Items[0].Name != "cluster-object" {
		t.Fatalf("expected indexed cluster-object, got %#v", indexedList.Items)
	}
}

func TestFakeInformersUsesConfiguredSharedIndexInformer(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	fakeInformer := controllertest.NewFakeInformer(controllertest.Synced)
	informers := &FakeInformers{
		InformersByGVK: map[schema.GroupVersionKind]toolscache.SharedIndexInformer{
			corev1.SchemeGroupVersion.WithKind("Pod"): sharedIndexInformerWrapper{SharedIndexInformer: fakeInformer},
		},
	}
	fakeInformer.Add(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "ns"}})

	var got corev1.Pod
	if err := informers.Get(ctx, client.ObjectKey{Namespace: "ns", Name: "pod"}, &got); err != nil {
		t.Fatalf("getting object from configured informer: %v", err)
	}
	if got.Name != "pod" {
		t.Fatalf("expected pod, got %q", got.Name)
	}
}

type sharedIndexInformerWrapper struct {
	toolscache.SharedIndexInformer
}
