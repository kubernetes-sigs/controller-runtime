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
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// watchDelay is how long the cache lags behind the fake client.
const watchDelay = 10 * time.Second

// newConsistentFakeClient returns a consistent client that is backed by a fakeclient
// and uses the real cache.
func newConsistentFakeClient(t *testing.T, opts cache.Options) client.Client {
	t.Helper()

	opts.SyncPeriod = ptr.To(time.Duration(0))

	upstream := fake.NewClientBuilder().
		WithGlobalResourceVersionCounter().
		Build()

	var listOpts []client.ListOption
	for namespace := range opts.DefaultNamespaces {
		listOpts = append(listOpts, client.InNamespace(namespace))
	}
	if opts.DefaultLabelSelector != nil {
		listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: opts.DefaultLabelSelector})
	}

	opts.NewInformer = func(_ toolscache.ListerWatcher, obj runtime.Object, resync time.Duration, indexers toolscache.Indexers) toolscache.SharedIndexInformer {
		lw := &fakeListWatcher{client: upstream, scheme: opts.Scheme, obj: obj, listOpts: listOpts}
		return toolscache.NewSharedIndexInformer(lw, obj, resync, indexers)
	}

	c, err := cache.New(&rest.Config{}, opts)
	if err != nil {
		t.Fatalf("failed to construct cache: %v", err)
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

	return client.NewConsistentClient(&fakeConsistentClientUpstream{WithWatch: upstream, reader: c}, consistencyCache, nil)
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
	client   client.WithWatch
	scheme   *runtime.Scheme
	obj      runtime.Object
	listOpts []client.ListOption

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
	w, err := l.client.Watch(context.Background(), list, l.listOpts...)
	if err != nil {
		return nil, err
	}
	l.lock.Lock()
	l.pending = newDelayedWatch(w, l.convert)
	l.lock.Unlock()

	if err := l.client.List(context.Background(), list, l.listOpts...); err != nil {
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
