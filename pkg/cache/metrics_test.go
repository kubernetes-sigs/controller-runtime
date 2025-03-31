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

package cache

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	toolscache "k8s.io/client-go/tools/cache"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

var _ = Describe("Cache Metrics", func() {

	Describe("GetCachedResourceCount", func() {
		var (
			ctx           context.Context
			podGVK        schema.GroupVersionKind
			informerCache *fakeInformerCache
			pod           *unstructured.Unstructured
		)

		BeforeEach(func() {
			ctx = context.Background()
			podGVK = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}
			s := scheme.Scheme
			informerCache = &fakeInformerCache{
				informersByGVK: map[schema.GroupVersionKind]toolscache.SharedIndexInformer{
					podGVK: &fakeSharedIndexInformer{
						objects: []runtime.Object{
							&unstructured.Unstructured{
								Object: map[string]interface{}{
									"apiVersion": "v1",
									"kind":       "Pod",
									"metadata": map[string]interface{}{
										"name":      "pod-1",
										"namespace": "default",
									},
								},
							},
							&unstructured.Unstructured{
								Object: map[string]interface{}{
									"apiVersion": "v1",
									"kind":       "Pod",
									"metadata": map[string]interface{}{
										"name":      "pod-2",
										"namespace": "default",
									},
								},
							},
						},
					},
				},
				scheme: s,
			}

			pod = &unstructured.Unstructured{}
			pod.SetGroupVersionKind(podGVK)
		})

		It("should return the correct resource count", func() {
			count, err := GetCachedResourceCount(ctx, informerCache, pod)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(2))
		})
	})

	Describe("DumpCacheResourceMetrics", func() {
		var (
			ctx           context.Context
			podGVK        schema.GroupVersionKind
			deploymentGVK schema.GroupVersionKind
			cacheImpl     *testInformerCache
		)

		BeforeEach(func() {
			ctx = context.Background()
			podGVK = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}
			deploymentGVK = schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}

			// Prepare test data
			informersByGVK := map[schema.GroupVersionKind]toolscache.SharedIndexInformer{
				podGVK: &fakeSharedIndexInformer{
					objects: []runtime.Object{
						&unstructured.Unstructured{Object: map[string]interface{}{
							"apiVersion": "v1", "kind": "Pod",
							"metadata": map[string]interface{}{
								"name": "pod-1", "namespace": "default",
							},
						}},
						&unstructured.Unstructured{Object: map[string]interface{}{
							"apiVersion": "v1", "kind": "Pod",
							"metadata": map[string]interface{}{
								"name": "pod-2", "namespace": "default",
							},
						}},
					},
				},
				deploymentGVK: &fakeSharedIndexInformer{
					objects: []runtime.Object{
						&unstructured.Unstructured{Object: map[string]interface{}{
							"apiVersion": "apps/v1", "kind": "Deployment",
							"metadata": map[string]interface{}{
								"name": "deployment-1", "namespace": "default",
							},
						}},
					},
				},
			}

			// Create an object that conforms to the informerCache type
			cacheImpl = &testInformerCache{
				informers: informersByGVK,
			}
		})

		It("should collect metrics without error", func() {
			err := DumpCacheResourceMetrics(ctx, cacheImpl)
			Expect(err).NotTo(HaveOccurred())
			// Due to test framework limitations, we can only verify the function didn't throw an error
		})
	})
})

// A simplified version of the real informerCache, sufficient for type checking
type testInformerCache struct {
	informers map[schema.GroupVersionKind]toolscache.SharedIndexInformer
}

// Allow DumpCacheResourceMetrics to access internal informers
func (c *testInformerCache) Informers() interface{} {
	return &testInformersMap{
		informers: c.informers,
	}
}

// Implement the required methods of the Cache interface
func (c *testInformerCache) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return nil
}

func (c *testInformerCache) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return nil
}

func (c *testInformerCache) GetInformer(ctx context.Context, obj client.Object, opts ...InformerGetOption) (Informer, error) {
	return nil, nil
}

func (c *testInformerCache) GetInformerForKind(ctx context.Context, gvk schema.GroupVersionKind, opts ...InformerGetOption) (Informer, error) {
	return nil, nil
}

func (c *testInformerCache) Start(ctx context.Context) error {
	return nil
}

func (c *testInformerCache) WaitForCacheSync(ctx context.Context) bool {
	return true
}

func (c *testInformerCache) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	return nil
}

func (c *testInformerCache) RemoveInformer(ctx context.Context, obj client.Object) error {
	return nil
}

// Implement the VisitInformers method interface to allow DumpCacheResourceMetrics to access informers
type testInformersMap struct {
	informers map[schema.GroupVersionKind]toolscache.SharedIndexInformer
}

func (m *testInformersMap) VisitInformers(visitor func(gvk schema.GroupVersionKind, informer toolscache.SharedIndexInformer)) {
	for gvk, informer := range m.informers {
		visitor(gvk, informer)
	}
}

// Fake implementation for testing
type fakeInformerCache struct {
	scheme         *runtime.Scheme
	informersByGVK map[schema.GroupVersionKind]toolscache.SharedIndexInformer
}

func (c *fakeInformerCache) GetInformer(ctx context.Context, obj client.Object, _ ...InformerGetOption) (Informer, error) {
	gvk, err := apiutil.GVKForObject(obj, c.scheme)
	if err != nil {
		return nil, err
	}

	if informer, ok := c.informersByGVK[gvk]; ok {
		return &fakeInformer{SharedIndexInformer: informer}, nil
	}

	return nil, nil
}

func (c *fakeInformerCache) GetInformerForKind(ctx context.Context, gvk schema.GroupVersionKind, _ ...InformerGetOption) (Informer, error) {
	if informer, ok := c.informersByGVK[gvk]; ok {
		return &fakeInformer{SharedIndexInformer: informer}, nil
	}
	return nil, nil
}

func (c *fakeInformerCache) Start(ctx context.Context) error {
	return nil
}

func (c *fakeInformerCache) WaitForCacheSync(ctx context.Context) bool {
	return true
}

func (c *fakeInformerCache) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	return nil
}

func (c *fakeInformerCache) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return nil
}

func (c *fakeInformerCache) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return nil
}

func (c *fakeInformerCache) RemoveInformer(ctx context.Context, obj client.Object) error {
	return nil
}

func (c *fakeInformerCache) NeedLeaderElection() bool {
	return false
}

// Informer implementation for testing
type fakeInformer struct {
	toolscache.SharedIndexInformer
}

func (f *fakeInformer) AddEventHandler(handler toolscache.ResourceEventHandler) (toolscache.ResourceEventHandlerRegistration, error) {
	return nil, nil
}

func (f *fakeInformer) AddEventHandlerWithResyncPeriod(handler toolscache.ResourceEventHandler, resyncPeriod time.Duration) (toolscache.ResourceEventHandlerRegistration, error) {
	return nil, nil
}

func (f *fakeInformer) AddEventHandlerWithOptions(handler toolscache.ResourceEventHandler, options toolscache.HandlerOptions) (toolscache.ResourceEventHandlerRegistration, error) {
	return nil, nil
}

func (f *fakeInformer) RemoveEventHandler(registration toolscache.ResourceEventHandlerRegistration) error {
	return nil
}

func (f *fakeInformer) HasSynced() bool {
	return true
}

func (f *fakeInformer) AddIndexers(indexers toolscache.Indexers) error {
	return nil
}

// SharedIndexInformer implementation for testing
type fakeSharedIndexInformer struct {
	objects []runtime.Object
}

func (f *fakeSharedIndexInformer) AddEventHandler(handler toolscache.ResourceEventHandler) (toolscache.ResourceEventHandlerRegistration, error) {
	return nil, nil
}

func (f *fakeSharedIndexInformer) AddEventHandlerWithResyncPeriod(handler toolscache.ResourceEventHandler, resyncPeriod time.Duration) (toolscache.ResourceEventHandlerRegistration, error) {
	return nil, nil
}

func (f *fakeSharedIndexInformer) AddEventHandlerWithOptions(handler toolscache.ResourceEventHandler, options toolscache.HandlerOptions) (toolscache.ResourceEventHandlerRegistration, error) {
	return nil, nil
}

func (f *fakeSharedIndexInformer) RemoveEventHandler(registration toolscache.ResourceEventHandlerRegistration) error {
	return nil
}

func (f *fakeSharedIndexInformer) GetStore() toolscache.Store {
	return &fakeStore{objects: f.objects}
}

func (f *fakeSharedIndexInformer) GetController() toolscache.Controller {
	return nil
}

func (f *fakeSharedIndexInformer) Run(stopCh <-chan struct{}) {
}

func (f *fakeSharedIndexInformer) RunWithContext(context.Context) {
}

func (f *fakeSharedIndexInformer) HasSynced() bool {
	return true
}

func (f *fakeSharedIndexInformer) LastSyncResourceVersion() string {
	return ""
}

func (f *fakeSharedIndexInformer) SetWatchErrorHandler(toolscache.WatchErrorHandler) error {
	return nil
}

func (f *fakeSharedIndexInformer) SetWatchErrorHandlerWithContext(toolscache.WatchErrorHandlerWithContext) error {
	return nil
}

func (f *fakeSharedIndexInformer) SetTransform(toolscache.TransformFunc) error {
	return nil
}

func (f *fakeSharedIndexInformer) GetIndexer() toolscache.Indexer {
	return &fakeIndexer{objects: f.objects}
}

func (f *fakeSharedIndexInformer) AddIndexers(toolscache.Indexers) error {
	return nil
}

func (f *fakeSharedIndexInformer) IsStopped() bool {
	return false
}

// Store implementation for testing
type fakeStore struct {
	objects []runtime.Object
}

func (f *fakeStore) Add(obj interface{}) error {
	f.objects = append(f.objects, obj.(runtime.Object))
	return nil
}

func (f *fakeStore) Update(obj interface{}) error {
	return nil
}

func (f *fakeStore) Delete(obj interface{}) error {
	return nil
}

func (f *fakeStore) List() []interface{} {
	result := make([]interface{}, len(f.objects))
	for i, obj := range f.objects {
		result[i] = obj
	}
	return result
}

func (f *fakeStore) ListKeys() []string {
	return nil
}

func (f *fakeStore) Get(obj interface{}) (item interface{}, exists bool, err error) {
	return nil, false, nil
}

func (f *fakeStore) GetByKey(key string) (item interface{}, exists bool, err error) {
	return nil, false, nil
}

func (f *fakeStore) Replace(list []interface{}, resourceVersion string) error {
	return nil
}

func (f *fakeStore) Resync() error {
	return nil
}

// Indexer implementation for testing
type fakeIndexer struct {
	objects []runtime.Object
}

func (f *fakeIndexer) Add(obj interface{}) error {
	f.objects = append(f.objects, obj.(runtime.Object))
	return nil
}

func (f *fakeIndexer) Update(obj interface{}) error {
	return nil
}

func (f *fakeIndexer) Delete(obj interface{}) error {
	return nil
}

func (f *fakeIndexer) List() []interface{} {
	result := make([]interface{}, len(f.objects))
	for i, obj := range f.objects {
		result[i] = obj
	}
	return result
}

func (f *fakeIndexer) ListKeys() []string {
	return nil
}

func (f *fakeIndexer) Get(obj interface{}) (item interface{}, exists bool, err error) {
	return nil, false, nil
}

func (f *fakeIndexer) GetByKey(key string) (item interface{}, exists bool, err error) {
	return nil, false, nil
}

func (f *fakeIndexer) Replace(list []interface{}, resourceVersion string) error {
	return nil
}

func (f *fakeIndexer) Resync() error {
	return nil
}

func (f *fakeIndexer) Index(indexName string, obj interface{}) ([]interface{}, error) {
	return nil, nil
}

func (f *fakeIndexer) IndexKeys(indexName, indexedValue string) ([]string, error) {
	return nil, nil
}

func (f *fakeIndexer) ListIndexFuncValues(indexName string) []string {
	return nil
}

func (f *fakeIndexer) ByIndex(indexName, indexedValue string) ([]interface{}, error) {
	return nil, nil
}

func (f *fakeIndexer) GetIndexers() toolscache.Indexers {
	return nil
}

func (f *fakeIndexer) AddIndexers(newIndexers toolscache.Indexers) error {
	return nil
}

// Implement the Informers interface method
func (c *fakeInformerCache) Informers() interface{} {
	return &testInformersMap{
		informers: c.informersByGVK,
	}
}
