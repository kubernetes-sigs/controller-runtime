/*
Copyright The Kubernetes Authors.

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

package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	appsv1applyconfigurations "k8s.io/client-go/applyconfigurations/apps/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/utils/ptr"

	"k8s.io/apimachinery/pkg/api/meta/testrestmapper"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// watchDelay is how long the cache lags behind the fake client.
const watchDelay = 10 * time.Second

// newConsistentFakeClient returns a consistent client that is backed by a fakeclient
// and uses the real cache.
func newConsistentFakeClient(
	t *testing.T,
	locker client.KeyLock,
	opts cache.Options,
	warmupFor client.Object,
	initObjects ...client.Object,
) client.Client {
	t.Helper()

	upstream := fake.NewClientBuilder().
		WithGlobalResourceVersionCounter().
		WithObjects(initObjects...).
		Build()

	opts.Scheme = scheme.Scheme
	if opts.Mapper == nil {
		opts.Mapper = testrestmapper.TestOnlyStaticRESTMapper(opts.Scheme)
	}
	// NB: Don't use t.Context() directly, it is cancelled before the cleanup
	// functions run and the cache must outlive them.
	ctx, cancel := context.WithCancel(context.WithoutCancel(t.Context()))

	probe := &namespaceProber{}
	opts.HTTPClient = &http.Client{Transport: probe}
	opts.NewInformer = func(realLW toolscache.ListerWatcher, obj runtime.Object, resync time.Duration, indexers toolscache.Indexers) toolscache.SharedIndexInformer {
		namespace, err := probe.namespaceFor(ctx, realLW)
		if err != nil {
			t.Errorf("failed to determine the namespace of the listwatcher for %T: %v", obj, err)
		}
		lw := &fakeListWatcher{ctx: ctx, client: upstream, scheme: opts.Scheme, obj: obj, namespace: namespace}
		return toolscache.NewSharedIndexInformer(lw, obj, resync, indexers)
	}
	c, err := cache.New(&rest.Config{}, opts)
	if err != nil {
		t.Fatalf("failed to construct cache: %v", err)
	}

	// Prewarm the cache to make sure events get deliveted over the delayed
	// watch and not the initial list and so deletes can work.
	if _, err := c.GetInformer(t.Context(), warmupFor); err != nil {
		t.Fatalf("failed to get informer for %T: %v", warmupFor, err)
	}

	consistencyCache, ok := c.(client.ConsistencyCache)
	if !ok {
		t.Fatalf("cache of type %T does not implement %T", c, client.ConsistencyCache(nil))
	}

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		if err := c.Start(ctx); err != nil {
			t.Errorf("cache failed: %v", err)
		}
	}()
	t.Cleanup(func() {
		cancel()
		<-stopped
	})

	return client.NewConsistentClient(
		&fakeConsistentClientUpstream{WithWatch: upstream, reader: c},
		consistencyCache,
		func() client.KeyLock {
			return locker
		},
	)
}

type fakeConsistentClientUpstream struct {
	client.WithWatch
	reader client.Reader
}

func (u *fakeConsistentClientUpstream) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return u.reader.Get(ctx, key, obj, opts...)
}

func (u *fakeConsistentClientUpstream) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return u.reader.List(ctx, list, opts...)
}

// DeleteWithResult returns what an apiserver returns for a delete: the object if it
// is still in storage because it has finalizers, and a response without a resource
// version if it is gone.
func (u *fakeConsistentClientUpstream) DeleteWithResult(ctx context.Context, obj client.Object, opts ...client.DeleteOption) (*unstructured.Unstructured, error) {
	if err := u.WithWatch.Delete(ctx, obj, opts...); err != nil {
		return nil, err
	}

	gvk, err := u.GroupVersionKindFor(obj)
	if err != nil {
		return nil, err
	}
	result := &unstructured.Unstructured{}
	result.SetGroupVersionKind(gvk)
	if err := u.WithWatch.Get(ctx, client.ObjectKeyFromObject(obj), result); err != nil {
		if apierrors.IsNotFound(err) {
			return &unstructured.Unstructured{}, nil
		}
		return nil, err
	}

	return result, nil
}

// namespaceProber is a http.RoundTripper that exists for the sole purpose
// of extracting the namespace from a listwatcher.
type namespaceProber struct {
	lock sync.Mutex
	path string
}

func (p *namespaceProber) RoundTrip(r *http.Request) (*http.Response, error) {
	p.path = r.URL.Path
	return nil, errors.New("request issued by the namespace probe")
}

func (p *namespaceProber) namespaceFor(ctx context.Context, lw toolscache.ListerWatcher) (string, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.path = ""
	_, err := toolscache.ToListerWatcherWithContext(lw).ListWithContext(ctx, metav1.ListOptions{})
	if err == nil {
		return "", fmt.Errorf("expected listing to fail for prober but didn't")
	}

	// Example path for a namespaced request: /apis/<group>/<version>/namespaces/<namespace>/<resource>
	segments := strings.Split(strings.Trim(p.path, "/"), "/")
	for idx, segment := range segments {
		if segment == "namespaces" && idx+1 < len(segments)-1 {
			return segments[idx+1], nil
		}
	}

	return "", nil
}

// fakeListWatcher lists and watches a single kind from the fake client, in the
// representation the informer it belongs to expects. It is restricted to namespace
// if that is set.
type fakeListWatcher struct {
	ctx       context.Context
	client    client.WithWatch
	scheme    *runtime.Scheme
	obj       runtime.Object
	namespace string

	lock sync.Mutex
	// pending is the watch that was opened when the last list was taken.
	pending watch.Interface
}

func (l *fakeListWatcher) List(_ metav1.ListOptions) (runtime.Object, error) {
	list, err := l.newList()
	if err != nil {
		return nil, err
	}

	var opts []client.ListOption
	if l.namespace != "" {
		opts = append(opts, client.InNamespace(l.namespace))
	}

	// Watch before listing. An object that ends up in both is harmless, one that
	// ends up in neither would never reach the informer.
	w, err := l.client.Watch(l.ctx, list, opts...)
	if err != nil {
		return nil, err
	}
	l.lock.Lock()
	l.pending = newDelayedWatch(w, l.convert)
	l.lock.Unlock()

	if err := l.client.List(l.ctx, list, opts...); err != nil {
		return nil, err
	}

	return list, nil
}

// newList returns the list type that matches the informers object type. The fake
// client converts into whatever it is given, the informer is not that forgiving.
func (l *fakeListWatcher) newList() (client.ObjectList, error) {
	gvk, err := apiutil.GVKForObject(l.obj, l.scheme)
	if err != nil {
		return nil, err
	}
	listGVK := gvk.GroupVersion().WithKind(gvk.Kind + "List")

	switch l.obj.(type) {
	case runtime.Unstructured:
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(listGVK)
		return list, nil
	case *metav1.PartialObjectMetadata:
		list := &metav1.PartialObjectMetadataList{}
		list.SetGroupVersionKind(listGVK)
		return list, nil
	default:
		obj, err := l.scheme.New(listGVK)
		if err != nil {
			return nil, err
		}
		return obj.(client.ObjectList), nil
	}
}

func (l *fakeListWatcher) convert(obj runtime.Object) (runtime.Object, error) {
	gvk, err := apiutil.GVKForObject(l.obj, l.scheme)
	if err != nil {
		return nil, err
	}

	switch l.obj.(type) {
	case runtime.Unstructured:
		content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return nil, err
		}
		converted := &unstructured.Unstructured{Object: content}
		converted.SetGroupVersionKind(gvk)
		return converted, nil
	case *metav1.PartialObjectMetadata:
		content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return nil, err
		}
		converted := &metav1.PartialObjectMetadata{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(content, converted); err != nil {
			return nil, err
		}
		converted.SetGroupVersionKind(gvk)
		return converted, nil
	default:
		return obj, nil
	}
}

func (l *fakeListWatcher) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	if opts.SendInitialEvents != nil {
		// Streaming lists are not supported, this makes the reflector fall back
		// to listing and then watching.
		return nil, errors.New("streaming lists are not supported")
	}

	l.lock.Lock()
	defer l.lock.Unlock()

	w := l.pending
	l.pending = nil

	return w, nil
}

func newDelayedWatch(upstream watch.Interface, convert func(runtime.Object) (runtime.Object, error)) watch.Interface {
	w := &delayedWatch{
		upstream: upstream,
		convert:  convert,
		out:      make(chan watch.Event),
		stop:     make(chan struct{}),
	}
	go w.forward()

	return w
}

// delayedWatch delivers the events of another watch with a delay.
type delayedWatch struct {
	upstream watch.Interface
	convert  func(runtime.Object) (runtime.Object, error)
	out      chan watch.Event
	stop     chan struct{}
	stopOnce sync.Once
}

func (w *delayedWatch) forward() {
	defer close(w.out)

	for event := range w.upstream.ResultChan() {
		converted, err := w.convert(event.Object)
		if err != nil {
			panic(fmt.Sprintf("failed to convert %T: %v", event.Object, err))
		}
		event.Object = converted

		time.Sleep(watchDelay)
		select {
		case w.out <- event:
		case <-w.stop:
			return
		}
	}
}

func (w *delayedWatch) ResultChan() <-chan watch.Event { return w.out }

func (w *delayedWatch) Stop() {
	w.stopOnce.Do(func() {
		close(w.stop)
		w.upstream.Stop()
	})
}

type clusterScopedRESTMapper struct {
	meta.RESTMapper
}

func (c clusterScopedRESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	mapping, err := c.RESTMapper.RESTMapping(gk, versions...)
	if err != nil {
		return nil, err
	}
	mapping.Scope = meta.RESTScopeRoot
	return mapping, nil
}

func (c clusterScopedRESTMapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	mappings, err := c.RESTMapper.RESTMappings(gk, versions...)
	if err != nil {
		return nil, err
	}
	for _, mapping := range mappings {
		mapping.Scope = meta.RESTScopeRoot
	}
	return mappings, nil
}

type keyLockerWithLockCallback struct {
	client.KeyLocker
	lockCallback func()
}

func (k *keyLockerWithLockCallback) Lock(ctx context.Context) error {
	err := k.KeyLocker.Lock(ctx)
	k.lockCallback()
	return err
}

// TestConsistentFakeClient uses a callback on the Lock acquisition of a write operation
// to start a read operation, then validates the read operation observes the write.
// It uses a fake informer with a hardcoded 10 second delay on the watch in synctest to
// avoid actually having to wait 10 seconds.
//
// It tests the cross product of:
// * Cluster-scoped and namespaced objects
// * Default and multi-namespace cache
// * Typed and unstructured representations
// * All write operations
// * Get and List
func TestConsistentFakeClient(t *testing.T) {
	t.Parallel()

	scopes := []struct {
		name      string
		namespace string
	}{
		{
			name:      "namespaced object",
			namespace: "default",
		},
		{
			name: "cluster-scoped object",
		},
	}

	for _, scope := range scopes {
		t.Run(scope.name, func(t *testing.T) {
			t.Parallel()
			testConsistentFakeClient(t, scope.namespace)
		})
	}
}

func testConsistentFakeClient(t *testing.T, namespace string) {
	typedDeployment := func() *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: namespace, UID: "test-uid"},
		}
	}

	representations := []struct {
		name       string
		deployment func() client.Object
	}{
		{
			name:       "typed",
			deployment: func() client.Object { return typedDeployment() },
		},
		{
			name: "unstructured",
			deployment: func() client.Object {
				u := mustToUnstructured(typedDeployment())
				u.SetGroupVersionKind(appsv1.SchemeGroupVersion.WithKind("Deployment"))
				return u
			},
		},
		{
			name: "partial object metadata",
			deployment: func() client.Object {
				partial := &metav1.PartialObjectMetadata{ObjectMeta: typedDeployment().ObjectMeta}
				partial.SetGroupVersionKind(appsv1.SchemeGroupVersion.WithKind("Deployment"))
				return partial
			},
		},
	}

	for _, representation := range representations {
		t.Run(representation.name, func(t *testing.T) {
			t.Parallel()
			testConsistentFakeClientForRepresentation(t, namespace, representation.deployment)
		})
	}
}

func testConsistentFakeClientForRepresentation(t *testing.T, namespace string, deployment func() client.Object) {
	mapper := testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme)
	if namespace == "" {
		mapper = clusterScopedRESTMapper{mapper}
	}

	_, isUnstructured := deployment().(*unstructured.Unstructured)
	_, isPartialMetadata := deployment().(*metav1.PartialObjectMetadata)

	deploymentWithFinalizer := func() client.Object {
		d := deployment()
		d.SetFinalizers([]string{"test.io/finalizer"})
		return d
	}

	deploymentList := func() client.ObjectList {
		if isPartialMetadata {
			list := &metav1.PartialObjectMetadataList{}
			list.SetGroupVersionKind(appsv1.SchemeGroupVersion.WithKind("DeploymentList"))
			return list
		}
		if !isUnstructured {
			return &appsv1.DeploymentList{}
		}
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(appsv1.SchemeGroupVersion.WithKind("DeploymentList"))
		return list
	}

	setStatusReplicas := func(g *WithT, obj client.Object, replicas int32) {
		switch obj := obj.(type) {
		case *unstructured.Unstructured:
			g.Expect(unstructured.SetNestedField(obj.Object, int64(replicas), "status", "replicas")).To(Succeed())
		case *appsv1.Deployment:
			obj.Status.Replicas = replicas
		default:
			panic(fmt.Sprintf("unhandled representation %T", obj))
		}
	}

	applyConfiguration := func(ac *appsv1applyconfigurations.DeploymentApplyConfiguration) (runtime.ApplyConfiguration, func() string) {
		if !isUnstructured {
			return ac, func() string { return ptr.Deref(ac.ResourceVersion, "") }
		}
		u := mustToUnstructured(ac)
		return client.ApplyConfigurationFromUnstructured(u), u.GetResourceVersion
	}

	resourceVersion := func(g *WithT, rv string) int64 {
		parsed, err := strconv.ParseInt(rv, 10, 64)
		g.Expect(err).NotTo(HaveOccurred(), "failed to parse resource version %q", rv)
		return parsed
	}

	create := func(ctx context.Context, c client.Client, g *WithT) int64 {
		d := deployment()
		g.Expect(c.Create(ctx, d)).To(Succeed())
		return resourceVersion(g, d.GetResourceVersion())
	}
	update := func(ctx context.Context, c client.Client, g *WithT) int64 {
		d := deployment()
		g.Expect(c.Get(ctx, client.ObjectKeyFromObject(d), d)).To(Succeed())
		d.SetLabels(map[string]string{"updated": "true"})
		g.Expect(c.Update(ctx, d)).To(Succeed())
		return resourceVersion(g, d.GetResourceVersion())
	}
	patch := func(ctx context.Context, c client.Client, g *WithT) int64 {
		d := deployment()
		patch := client.MergeFrom(d.DeepCopyObject().(client.Object))
		d.SetLabels(map[string]string{"patched": "true"})
		g.Expect(c.Patch(ctx, d, patch)).To(Succeed())
		return resourceVersion(g, d.GetResourceVersion())
	}
	apply := func(ctx context.Context, c client.Client, g *WithT) int64 {
		ac, rv := applyConfiguration(appsv1applyconfigurations.Deployment(deployment().GetName(), namespace).
			WithLabels(map[string]string{"applied": "true"}))
		g.Expect(c.Apply(ctx, ac, client.FieldOwner("test"))).To(Succeed())
		return resourceVersion(g, rv())
	}
	deleteObject := func(ctx context.Context, c client.Client, g *WithT) int64 {
		g.Expect(c.Delete(ctx, deployment())).To(Succeed())
		return 0
	}
	updateStatus := func(ctx context.Context, c client.Client, g *WithT) int64 {
		d := deployment()
		g.Expect(c.Get(ctx, client.ObjectKeyFromObject(d), d)).To(Succeed())
		setStatusReplicas(g, d, 5)
		g.Expect(c.Status().Update(ctx, d)).To(Succeed())
		return resourceVersion(g, d.GetResourceVersion())
	}
	patchStatus := func(ctx context.Context, c client.Client, g *WithT) int64 {
		d := deployment()
		patch := client.MergeFrom(d.DeepCopyObject().(client.Object))
		setStatusReplicas(g, d, 6)
		g.Expect(c.Status().Patch(ctx, d, patch)).To(Succeed())
		return resourceVersion(g, d.GetResourceVersion())
	}
	applyStatus := func(ctx context.Context, c client.Client, g *WithT) int64 {
		ac, rv := applyConfiguration(appsv1applyconfigurations.Deployment("test", namespace).
			WithStatus(appsv1applyconfigurations.DeploymentStatus().WithReplicas(7)))
		g.Expect(c.Status().Apply(ctx, ac, client.FieldOwner("test"))).To(Succeed())
		return resourceVersion(g, rv())
	}
	updateScale := func(ctx context.Context, c client.Client, g *WithT) int64 {
		d := deployment()
		scale := &autoscalingv1.Scale{Spec: autoscalingv1.ScaleSpec{Replicas: 8}}
		g.Expect(c.SubResource("scale").Update(ctx, d, client.WithSubResourceBody(scale))).To(Succeed())
		return resourceVersion(g, d.GetResourceVersion())
	}

	get := func(ctx context.Context, c client.Client, g *WithT, writtenRV <-chan int64) {
		result := deployment()
		g.Expect(c.Get(ctx, client.ObjectKeyFromObject(result), result)).To(Succeed())
		g.Expect(resourceVersion(g, result.GetResourceVersion())).To(BeNumerically(">=", <-writtenRV))
	}
	list := func(ctx context.Context, c client.Client, g *WithT, writtenRV <-chan int64) {
		result := deploymentList()
		g.Expect(c.List(ctx, result)).To(Succeed())
		items, err := meta.ExtractList(result)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(items).To(HaveLen(1))

		item, err := meta.Accessor(items[0])
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(resourceVersion(g, item.GetResourceVersion())).To(BeNumerically(">=", <-writtenRV))
	}
	getTerminating := func(ctx context.Context, c client.Client, g *WithT, _ <-chan int64) {
		result := deployment()
		g.Expect(c.Get(ctx, client.ObjectKeyFromObject(result), result)).To(Succeed())
		g.Expect(result.GetDeletionTimestamp()).ToNot(BeNil(), "expected the deletionTimestamp to be set")
		g.Expect(result.GetFinalizers()).To(ConsistOf("test.io/finalizer"))
	}
	listTerminating := func(ctx context.Context, c client.Client, g *WithT, _ <-chan int64) {
		result := deploymentList()
		g.Expect(c.List(ctx, result)).To(Succeed())
		items, err := meta.ExtractList(result)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(items).To(HaveLen(1))

		item, err := meta.Accessor(items[0])
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(item.GetDeletionTimestamp()).ToNot(BeNil(), "expected the deletionTimestamp to be set")
		g.Expect(item.GetFinalizers()).To(ConsistOf("test.io/finalizer"))
	}

	testCases := []struct {
		name                    string
		supportsPartialMetadata bool
		initObjects             func() []client.Object
		write                   func(ctx context.Context, client client.Client, g *WithT) int64
		read                    func(ctx context.Context, client client.Client, g *WithT, writtenRV <-chan int64)
	}{
		{
			name:  "Get after Create",
			write: create,
			read:  get,
		},
		{
			name:  "List after Create",
			write: create,
			read:  list,
		},
		{
			name:        "Get after Update",
			initObjects: func() []client.Object { return []client.Object{deployment()} },
			write:       update,
			read:        get,
		},
		{
			name:        "List after Update",
			initObjects: func() []client.Object { return []client.Object{deployment()} },
			write:       update,
			read:        list,
		},
		{
			name:                    "Get after Patch",
			supportsPartialMetadata: true,
			initObjects:             func() []client.Object { return []client.Object{deployment()} },
			write:                   patch,
			read:                    get,
		},
		{
			name:                    "List after Patch",
			supportsPartialMetadata: true,
			initObjects:             func() []client.Object { return []client.Object{deployment()} },
			write:                   patch,
			read:                    list,
		},
		{
			name:  "Get after Apply",
			write: apply,
			read:  get,
		},
		{
			name:  "List after Apply",
			write: apply,
			read:  list,
		},
		{
			name:                    "Get after Delete",
			supportsPartialMetadata: true,
			initObjects:             func() []client.Object { return []client.Object{deployment()} },
			write:                   deleteObject,
			read: func(ctx context.Context, c client.Client, g *WithT, _ <-chan int64) {
				d := deployment()
				err := c.Get(ctx, client.ObjectKeyFromObject(d), d)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected a NotFound error, got %v", err)
			},
		},
		{
			name:                    "List after Delete",
			supportsPartialMetadata: true,
			initObjects:             func() []client.Object { return []client.Object{deployment()} },
			write:                   deleteObject,
			read: func(ctx context.Context, c client.Client, g *WithT, _ <-chan int64) {
				result := deploymentList()
				g.Expect(c.List(ctx, result)).To(Succeed())
				items, err := meta.ExtractList(result)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(items).To(BeEmpty())
			},
		},
		{
			name:                    "Get after Delete of an object with finalizers",
			supportsPartialMetadata: true,
			initObjects:             func() []client.Object { return []client.Object{deploymentWithFinalizer()} },
			write:                   deleteObject,
			read:                    getTerminating,
		},
		{
			name:                    "List after Delete of an object with finalizers",
			supportsPartialMetadata: true,
			initObjects:             func() []client.Object { return []client.Object{deploymentWithFinalizer()} },
			write:                   deleteObject,
			read:                    listTerminating,
		},
		{
			name:        "Get after status Update",
			initObjects: func() []client.Object { return []client.Object{deployment()} },
			write:       updateStatus,
			read:        get,
		},
		{
			name:        "List after status Update",
			initObjects: func() []client.Object { return []client.Object{deployment()} },
			write:       updateStatus,
			read:        list,
		},
		{
			name:        "Get after status Patch",
			initObjects: func() []client.Object { return []client.Object{deployment()} },
			write:       patchStatus,
			read:        get,
		},
		{
			name:        "List after status Patch",
			initObjects: func() []client.Object { return []client.Object{deployment()} },
			write:       patchStatus,
			read:        list,
		},
		{
			name:        "Get after status Apply",
			initObjects: func() []client.Object { return []client.Object{deployment()} },
			write:       applyStatus,
			read:        get,
		},
		{
			name:        "List after status Apply",
			initObjects: func() []client.Object { return []client.Object{deployment()} },
			write:       applyStatus,
			read:        list,
		},
		{
			name:        "Get after scale Update",
			initObjects: func() []client.Object { return []client.Object{deployment()} },
			write:       updateScale,
			read:        get,
		},
		{
			name:        "List after scale Update",
			initObjects: func() []client.Object { return []client.Object{deployment()} },
			write:       updateScale,
			read:        list,
		},
	}
	cacheConfigs := []struct {
		name string
		opts cache.Options
	}{
		{
			name: "default",
		},
		{
			name: "multi-namespace cache",
			opts: cache.Options{DefaultNamespaces: map[string]cache.Config{
				namespace: {},
				"ns-2":    {},
			}},
		},
	}

	for _, cacheConfig := range cacheConfigs {
		for _, tc := range testCases {
			t.Run(cacheConfig.name+": "+tc.name, func(t *testing.T) {
				t.Parallel()
				if isPartialMetadata && !tc.supportsPartialMetadata {
					t.Skip("not supported by partial metadata object")
				}
				synctest.Test(t, func(t *testing.T) {
					g := NewWithT(t)
					locker := keyLockerWithLockCallback{}
					opts := cacheConfig.opts
					opts.Mapper = mapper

					var initObjects []client.Object
					if tc.initObjects != nil {
						initObjects = tc.initObjects()
					}
					c := newConsistentFakeClient(t, &locker, opts, deployment(), initObjects...)
					synctest.Wait() // wait for cache start to finish

					// Must happen in a goroutine otherwise we deadlock, as we are waiting for the write to release the lock while
					// blocking it from finishing the acquisition.
					writtenRV := make(chan int64, 1)
					callBackFinished := make(chan struct{})
					locker.lockCallback = sync.OnceFunc(func() {
						go func() {
							defer close(callBackFinished)

							tc.read(t.Context(), c, g, writtenRV)
						}()
					})
					writtenRV <- tc.write(t.Context(), c, g)

					<-callBackFinished
				})
			})
		}
	}
}

func mustToUnstructured(obj any) *unstructured.Unstructured {
	serialized, err := json.Marshal(obj)
	if err != nil {
		panic(fmt.Sprintf("failed to serialize %T: %v", obj, err))
	}

	content := map[string]any{}
	if err := json.Unmarshal(serialized, &content); err != nil {
		panic(fmt.Sprintf("failed to deserialize %T into an unstructured: %v", obj, err))
	}

	return &unstructured.Unstructured{Object: content}
}
