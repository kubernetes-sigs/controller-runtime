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
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	// Register the Ginkgo flags passed by hack/test-all.sh.
	_ "github.com/onsi/ginkgo/v2"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	toolscache "k8s.io/client-go/tools/cache"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestFakeInformersIndexField(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	informers := &FakeInformers{Client: fake.NewClientBuilder().WithObjects(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "one"}, Spec: corev1.PodSpec{NodeName: "node-a"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-b", Namespace: "two"}, Spec: corev1.PodSpec{NodeName: "node-a"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-c", Namespace: "one"}, Spec: corev1.PodSpec{NodeName: "node-b"}},
	).Build()}
	var before corev1.PodList
	if err := informers.List(ctx, &before, client.MatchingFields{"spec.nodeName": "node-a"}); err == nil {
		t.Fatal("expected query without a registered index to fail")
	}

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
	mapper := apimeta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion})
	mapper.Add(corev1.SchemeGroupVersion.WithKind("Namespace"), apimeta.RESTScopeRoot)
	mapper.Add(corev1.SchemeGroupVersion.WithKind("Pod"), apimeta.RESTScopeNamespace)
	informers := &FakeInformers{RESTMapper: mapper, Client: fake.NewClientBuilder().WithObjects(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "cluster-object"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "one"}},
	).Build()}
	if err := informers.Get(ctx, client.ObjectKey{Namespace: "two", Name: "pod"}, &corev1.Pod{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected namespace mismatch to return NotFound, got %v", err)
	}

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

func TestFakeInformersGetRESTMapper(t *testing.T) {
	t.Parallel()
	gvk := corev1.SchemeGroupVersion.WithKind("Namespace")
	clusterScoped := apimeta.NewDefaultRESTMapper([]schema.GroupVersion{gvk.GroupVersion()})
	clusterScoped.Add(gvk, apimeta.RESTScopeRoot)
	namespaced := apimeta.NewDefaultRESTMapper([]schema.GroupVersion{gvk.GroupVersion()})
	namespaced.Add(gvk, apimeta.RESTScopeNamespace)
	empty := apimeta.NewDefaultRESTMapper(nil)

	for _, tt := range []struct {
		name           string
		backingMapper  apimeta.RESTMapper
		explicitMapper apimeta.RESTMapper
		wantNotFound   bool
		wantNoMatch    bool
	}{
		{name: "backing cluster scope", backingMapper: clusterScoped},
		{name: "backing namespace scope", backingMapper: namespaced, wantNotFound: true},
		{name: "explicit mapper takes precedence", backingMapper: namespaced, explicitMapper: clusterScoped},
		{name: "default empty mapper preserves key", wantNotFound: true},
		{name: "explicit missing mapping returns error", backingMapper: clusterScoped, explicitMapper: empty, wantNoMatch: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			informers := &FakeInformers{
				RESTMapper: tt.explicitMapper,
				Client: fake.NewClientBuilder().WithRESTMapper(tt.backingMapper).WithObjects(
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "cluster-object"}},
				).Build(),
			}
			var got corev1.Namespace
			err := informers.Get(t.Context(), client.ObjectKey{Namespace: "ignored", Name: "cluster-object"}, &got)
			switch {
			case tt.wantNotFound:
				if !apierrors.IsNotFound(err) {
					t.Fatalf("expected NotFound, got %v", err)
				}
			case tt.wantNoMatch:
				if !apimeta.IsNoMatchError(err) {
					t.Fatalf("expected missing mapping error, got %v", err)
				}
			default:
				if err != nil || got.Name != "cluster-object" {
					t.Fatalf("expected cluster-object, got %q: %v", got.Name, err)
				}
			}
		})
	}
}

func TestFakeInformersUsesConfiguredSharedIndexInformer(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	configured := toolscache.NewSharedIndexInformer(&toolscache.ListWatch{}, &corev1.Pod{}, 0, nil)
	informers := &FakeInformers{
		Client: fake.NewClientBuilder().WithObjects(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "ns"}}).Build(),
		InformersByGVK: map[schema.GroupVersionKind]toolscache.SharedIndexInformer{
			corev1.SchemeGroupVersion.WithKind("Pod"): configured,
		},
	}
	gotInformer, err := informers.GetInformer(ctx, &corev1.Pod{})
	if err != nil || gotInformer != configured {
		t.Fatalf("configured informer was not returned: %v", err)
	}
	var got corev1.Pod
	if err := informers.Get(ctx, client.ObjectKey{Namespace: "ns", Name: "pod"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "pod" {
		t.Fatalf("expected pod, got %q", got.Name)
	}
	if err := informers.IndexField(ctx, &corev1.Pod{}, "metadata.name", func(obj client.Object) []string { return []string{obj.GetName()} }); err != nil {
		t.Fatal(err)
	}
	var list corev1.PodList
	if err := informers.List(ctx, &list, client.MatchingFields{"metadata.name": "pod"}); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected one pod, got %d", len(list.Items))
	}
}

func TestFakeInformersBackendMutations(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	backing := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{}).Build()
	informers := &FakeInformers{Client: backing}
	if err := informers.IndexField(ctx, &corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string { return []string{obj.(*corev1.Pod).Spec.NodeName} }); err != nil {
		t.Fatal(err)
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "ns"}, Spec: corev1.PodSpec{NodeName: "old"}}
	if err := backing.Create(ctx, pod); err != nil {
		t.Fatal(err)
	}
	check := func(node string, count int) {
		t.Helper()
		var list corev1.PodList
		if err := informers.List(ctx, &list, client.InNamespace("ns"), client.MatchingFields{"spec.nodeName": node}); err != nil {
			t.Fatal(err)
		}
		if len(list.Items) != count {
			t.Fatalf("node %q: expected %d pods, got %d", node, count, len(list.Items))
		}
	}
	check("old", 1)
	var got corev1.Pod
	if err := informers.Get(ctx, client.ObjectKeyFromObject(pod), &got); err != nil {
		t.Fatal(err)
	}
	got.Spec.NodeName = "new"
	check("old", 1) // Mutating a read must not change the backend.
	if err := backing.Update(ctx, &got); err != nil {
		t.Fatal(err)
	}
	check("old", 0)
	check("new", 1)
	if err := backing.Delete(ctx, &got); err != nil {
		t.Fatal(err)
	}
	check("new", 0)
	if err := informers.Get(ctx, client.ObjectKeyFromObject(pod), &corev1.Pod{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected NotFound after delete, got %v", err)
	}
}

func TestFakeInformersDefaultBackendConcurrentUse(t *testing.T) {
	t.Parallel()
	informers := &FakeInformers{}
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Go(func() {
			field := fmt.Sprintf("field-%d", i)
			if err := informers.IndexField(t.Context(), &corev1.Pod{}, field, func(client.Object) []string { return nil }); err != nil {
				t.Error(err)
				return
			}
			if err := informers.List(t.Context(), &corev1.PodList{}, client.MatchingFields{field: "value"}); err != nil {
				t.Error(err)
			}
			if err := informers.Get(t.Context(), client.ObjectKey{Name: "missing"}, &corev1.Pod{}); !apierrors.IsNotFound(err) {
				t.Errorf("expected NotFound, got %v", err)
			}
		})
	}
	wg.Wait()
}

func TestFakeInformersCustomScheme(t *testing.T) {
	t.Parallel()
	customScheme := runtime.NewScheme()
	gvk := schema.GroupVersionKind{Group: "testing.example.com", Version: "v1", Kind: "Widget"}
	customScheme.AddKnownTypeWithName(gvk, &corev1.ConfigMap{})
	customScheme.AddKnownTypeWithName(gvk.GroupVersion().WithKind("WidgetList"), &corev1.ConfigMapList{})
	backing := fake.NewClientBuilder().WithScheme(customScheme).WithObjects(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "widget"}}).Build()
	informers := &FakeInformers{Client: backing}
	if _, err := informers.GetInformerForKind(t.Context(), gvk); err != nil {
		t.Fatal(err)
	}
	if err := informers.IndexField(t.Context(), &corev1.ConfigMap{}, "name", func(obj client.Object) []string { return []string{obj.GetName()} }); err != nil {
		t.Fatal(err)
	}
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvk.GroupVersion().WithKind("WidgetList"))
	if err := informers.List(t.Context(), list, client.MatchingFields{"name": "widget"}); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected widget, got %v", list.Items)
	}
}

func TestFakeInformersReadErrors(t *testing.T) {
	t.Parallel()
	expected := errors.New("test error")
	for _, configuredError := range []bool{false, true} {
		t.Run(fmt.Sprint(configuredError), func(t *testing.T) {
			informers := &FakeInformers{Client: fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
				Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
					return expected
				},
				List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
					return expected
				},
			}).Build()}
			if configuredError {
				informers.Error = expected
			}
			if err := informers.Get(t.Context(), client.ObjectKey{}, &corev1.Pod{}); !errors.Is(err, expected) {
				t.Fatalf("expected read error, got %v", err)
			}
			if err := informers.List(t.Context(), &corev1.PodList{}); !errors.Is(err, expected) {
				t.Fatalf("expected list error, got %v", err)
			}
			if configuredError {
				if err := informers.IndexField(t.Context(), &corev1.Pod{}, "name", func(client.Object) []string { return nil }); !errors.Is(err, expected) {
					t.Fatalf("expected index error, got %v", err)
				}
			}
		})
	}
}
