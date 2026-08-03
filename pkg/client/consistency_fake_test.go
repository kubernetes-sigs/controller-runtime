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
	"errors"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	appsv1applyconfigurations "k8s.io/client-go/applyconfigurations/apps/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"

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
func newConsistentFakeClient(t *testing.T, locker client.KeyLock, initObjects ...client.Object) client.Client {
	t.Helper()

	upstream := fake.NewClientBuilder().
		WithGlobalResourceVersionCounter().
		WithObjects(initObjects...).
		Build()

	scheme := scheme.Scheme
	opts := cache.Options{
		Scheme: scheme,
		Mapper: testrestmapper.TestOnlyStaticRESTMapper(scheme),
	}
	opts.NewInformer = func(_ toolscache.ListerWatcher, obj runtime.Object, resync time.Duration, indexers toolscache.Indexers) toolscache.SharedIndexInformer {
		lw := &fakeListWatcher{client: upstream, scheme: scheme, obj: obj}
		return toolscache.NewSharedIndexInformer(lw, obj, resync, indexers)
	}

	c, err := cache.New(&rest.Config{}, opts)
	if err != nil {
		t.Fatalf("failed to construct cache: %v", err)
	}

	// Create a cache for initObjects GVK, otherwise deletes will fail as they require the informer
	// to be present.
	for _, initObj := range initObjects {
		gvk, err := apiutil.GVKForObject(initObj, scheme)
		if err != nil {
			t.Fatalf("failed to get GVK for %T: %v", initObj, err)
		}
		if _, err := c.GetInformerForKind(t.Context(), gvk); err != nil {
			t.Fatalf("failed to get informer for %v: %v", gvk, err)
		}
	}

	consistencyCache, ok := c.(client.ConsistencyCache)
	if !ok {
		t.Fatalf("cache of type %T does not implement %T", c, client.ConsistencyCache(nil))
	}

	ctx, cancel := context.WithCancel(context.Background())
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

// fakeListWatcher lists and watches a single kind from the fake client, in the
// representation the informer it belongs to expects.
type fakeListWatcher struct {
	client client.WithWatch
	scheme *runtime.Scheme
	obj    runtime.Object

	lock sync.Mutex
	// pending is the watch that was opened when the last list was taken.
	pending watch.Interface
}

func (l *fakeListWatcher) List(_ metav1.ListOptions) (runtime.Object, error) {
	list, err := l.newList()
	if err != nil {
		return nil, err
	}

	// Watch before listing. An object that ends up in both is harmless, one that
	// ends up in neither would never reach the informer.
	w, err := l.client.Watch(context.Background(), list)
	if err != nil {
		return nil, err
	}
	l.lock.Lock()
	l.pending = newDelayedWatch(w, l.convert)
	l.lock.Unlock()

	if err := l.client.List(context.Background(), list); err != nil {
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
func TestConsistentFakeClient(t *testing.T) {
	t.Parallel()

	deployment := func() *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "test", UID: "test-uid"},
			Spec:       appsv1.DeploymentSpec{Replicas: new(int32(1))},
		}
	}

	create := func(ctx context.Context, c client.Client, g *WithT) {
		g.Expect(c.Create(ctx, deployment())).To(Succeed())
	}
	update := func(ctx context.Context, c client.Client, g *WithT) {
		d := &appsv1.Deployment{}
		g.Expect(c.Get(ctx, client.ObjectKeyFromObject(deployment()), d)).To(Succeed())
		d.Spec.Replicas = new(int32(2))
		g.Expect(c.Update(ctx, d)).To(Succeed())
	}
	patch := func(ctx context.Context, c client.Client, g *WithT) {
		d := deployment()
		patch := client.MergeFrom(d.DeepCopy())
		d.Spec.Replicas = new(int32(3))
		g.Expect(c.Patch(ctx, d, patch)).To(Succeed())
	}
	apply := func(ctx context.Context, c client.Client, g *WithT) {
		ac := appsv1applyconfigurations.Deployment(deployment().Name, deployment().Namespace).
			WithSpec(appsv1applyconfigurations.DeploymentSpec().WithReplicas(4))
		g.Expect(c.Apply(ctx, ac, client.FieldOwner("test"))).To(Succeed())
	}
	deleteObject := func(ctx context.Context, c client.Client, g *WithT) {
		g.Expect(c.Delete(ctx, deployment())).To(Succeed())
	}
	updateStatus := func(ctx context.Context, c client.Client, g *WithT) {
		d := &appsv1.Deployment{}
		g.Expect(c.Get(ctx, client.ObjectKeyFromObject(deployment()), d)).To(Succeed())
		d.Status.Replicas = 5
		g.Expect(c.Status().Update(ctx, d)).To(Succeed())
	}
	patchStatus := func(ctx context.Context, c client.Client, g *WithT) {
		d := deployment()
		patch := client.MergeFrom(d.DeepCopy())
		d.Status.Replicas = 6
		g.Expect(c.Status().Patch(ctx, d, patch)).To(Succeed())
	}
	applyStatus := func(ctx context.Context, c client.Client, g *WithT) {
		ac := appsv1applyconfigurations.Deployment(deployment().Name, deployment().Namespace).
			WithStatus(appsv1applyconfigurations.DeploymentStatus().WithReplicas(7))
		g.Expect(c.Status().Apply(ctx, ac, client.FieldOwner("test"))).To(Succeed())
	}
	updateScale := func(ctx context.Context, c client.Client, g *WithT) {
		scale := &autoscalingv1.Scale{Spec: autoscalingv1.ScaleSpec{Replicas: 8}}
		g.Expect(c.SubResource("scale").Update(ctx, deployment(), client.WithSubResourceBody(scale))).To(Succeed())
	}

	get := func(assert func(*appsv1.Deployment, *WithT)) func(context.Context, client.Client, *WithT) {
		return func(ctx context.Context, c client.Client, g *WithT) {
			result := &appsv1.Deployment{}
			g.Expect(c.Get(ctx, client.ObjectKeyFromObject(deployment()), result)).To(Succeed())
			assert(result, g)
		}
	}
	list := func(assert func(*appsv1.Deployment, *WithT)) func(context.Context, client.Client, *WithT) {
		return func(ctx context.Context, c client.Client, g *WithT) {
			result := &appsv1.DeploymentList{}
			g.Expect(c.List(ctx, result)).To(Succeed())
			g.Expect(result.Items).To(HaveLen(1))
			assert(&result.Items[0], g)
		}
	}
	specReplicas := func(expected int32) func(*appsv1.Deployment, *WithT) {
		return func(d *appsv1.Deployment, g *WithT) {
			g.Expect(d.Spec.Replicas).To(HaveValue(Equal(expected)))
		}
	}
	statusReplicas := func(expected int32) func(*appsv1.Deployment, *WithT) {
		return func(d *appsv1.Deployment, g *WithT) {
			g.Expect(d.Status.Replicas).To(Equal(expected))
		}
	}

	testCases := []struct {
		name        string
		initObjects []client.Object
		write       func(ctx context.Context, client client.Client, g *WithT)
		read        func(ctx context.Context, client client.Client, g *WithT)
	}{
		{
			name:  "Get after Create",
			write: create,
			read:  get(specReplicas(1)),
		},
		{
			name:  "List after Create",
			write: create,
			read:  list(specReplicas(1)),
		},
		{
			name:        "Get after Update",
			initObjects: []client.Object{deployment()},
			write:       update,
			read:        get(specReplicas(2)),
		},
		{
			name:        "List after Update",
			initObjects: []client.Object{deployment()},
			write:       update,
			read:        list(specReplicas(2)),
		},
		{
			name:        "Get after Patch",
			initObjects: []client.Object{deployment()},
			write:       patch,
			read:        get(specReplicas(3)),
		},
		{
			name:        "List after Patch",
			initObjects: []client.Object{deployment()},
			write:       patch,
			read:        list(specReplicas(3)),
		},
		{
			name:  "Get after Apply",
			write: apply,
			read:  get(specReplicas(4)),
		},
		{
			name:  "List after Apply",
			write: apply,
			read:  list(specReplicas(4)),
		},
		{
			name:        "Get after Delete",
			initObjects: []client.Object{deployment()},
			write:       deleteObject,
			read: func(ctx context.Context, c client.Client, g *WithT) {
				err := c.Get(ctx, client.ObjectKeyFromObject(deployment()), &appsv1.Deployment{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected a NotFound error, got %v", err)
			},
		},
		{
			name:        "List after Delete",
			initObjects: []client.Object{deployment()},
			write:       deleteObject,
			read: func(ctx context.Context, c client.Client, g *WithT) {
				result := &appsv1.DeploymentList{}
				g.Expect(c.List(ctx, result)).To(Succeed())
				g.Expect(result.Items).To(BeEmpty())
			},
		},
		{
			name:        "Get after status Update",
			initObjects: []client.Object{deployment()},
			write:       updateStatus,
			read:        get(statusReplicas(5)),
		},
		{
			name:        "List after status Update",
			initObjects: []client.Object{deployment()},
			write:       updateStatus,
			read:        list(statusReplicas(5)),
		},
		{
			name:        "Get after status Patch",
			initObjects: []client.Object{deployment()},
			write:       patchStatus,
			read:        get(statusReplicas(6)),
		},
		{
			name:        "List after status Patch",
			initObjects: []client.Object{deployment()},
			write:       patchStatus,
			read:        list(statusReplicas(6)),
		},
		{
			name:        "Get after status Apply",
			initObjects: []client.Object{deployment()},
			write:       applyStatus,
			read:        get(statusReplicas(7)),
		},
		{
			name:        "List after status Apply",
			initObjects: []client.Object{deployment()},
			write:       applyStatus,
			read:        list(statusReplicas(7)),
		},
		{
			name:        "Get after scale Update",
			initObjects: []client.Object{deployment()},
			write:       updateScale,
			read:        get(specReplicas(8)),
		},
		{
			name:        "List after scale Update",
			initObjects: []client.Object{deployment()},
			write:       updateScale,
			read:        list(specReplicas(8)),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				g := NewWithT(t)
				locker := keyLockerWithLockCallback{}
				c := newConsistentFakeClient(t, &locker, tc.initObjects...)
				synctest.Wait() // wait for cache start to finish

				// Must happen in a goroutine otherwise we deadlock, as we are waiting for the write to release the lock while
				// blocking it from finishing the acquisition.
				callBackFinished := make(chan struct{})
				locker.lockCallback = sync.OnceFunc(func() {
					go func() {
						defer close(callBackFinished)

						tc.read(t.Context(), c, g)
					}()
				})
				tc.write(t.Context(), c, g)

				<-callBackFinished
			})
		})
	}
}
