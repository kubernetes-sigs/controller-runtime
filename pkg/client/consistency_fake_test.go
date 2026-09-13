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
	"strconv"
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
	appsv1applyconfigurations "k8s.io/client-go/applyconfigurations/apps/v1"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/cache/cacheapi"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/client/internal/writebarrier"
)

// TestConsistentFakeClient uses a callback on the start of a write operation
// to start a read operation, then validates the read operation observes the write.
// It uses a fake cache with a fixed ten seconds delay in synctest, to avoid having to
// wait ten seconds of wallclock time.
//
// It tests the cross product of all write operations and get and list.
func TestConsistentFakeClient(t *testing.T) {
	t.Parallel()

	const namespace = "default"

	deployment := func() *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: namespace, UID: "test-uid"},
		}
	}

	deploymentWithFinalizer := func() *appsv1.Deployment {
		d := deployment()
		d.SetFinalizers([]string{"test.io/finalizer"})
		return d
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
		ac := appsv1applyconfigurations.Deployment(deployment().GetName(), namespace).
			WithLabels(map[string]string{"applied": "true"})
		g.Expect(c.Apply(ctx, ac, client.FieldOwner("test"))).To(Succeed())
		return resourceVersion(g, ptr.Deref(ac.ResourceVersion, ""))
	}
	deleteObject := func(ctx context.Context, c client.Client, g *WithT) int64 {
		d := deployment()
		g.Expect(c.Delete(ctx, d)).To(Succeed())
		if err := c.Get(ctx, client.ObjectKeyFromObject(d), d); err != nil {
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected a NotFound error, got %v", err)
			return 0
		}
		return resourceVersion(g, d.GetResourceVersion())
	}
	deleteObjectWithoutUID := func(ctx context.Context, c client.Client, g *WithT) int64 {
		d := deployment()
		d.SetUID("")
		g.Expect(c.Delete(ctx, d)).To(Succeed())
		return 0
	}
	deleteObjectWithUIDPrecondition := func(ctx context.Context, c client.Client, g *WithT) int64 {
		d := deployment()
		uid := d.GetUID()
		d.SetUID("")
		g.Expect(c.Delete(ctx, d, client.Preconditions{UID: &uid})).To(Succeed())
		return 0
	}
	updateStatus := func(ctx context.Context, c client.Client, g *WithT) int64 {
		d := deployment()
		g.Expect(c.Get(ctx, client.ObjectKeyFromObject(d), d)).To(Succeed())
		d.Status.Replicas = 5
		g.Expect(c.Status().Update(ctx, d)).To(Succeed())
		return resourceVersion(g, d.GetResourceVersion())
	}
	patchStatus := func(ctx context.Context, c client.Client, g *WithT) int64 {
		d := deployment()
		patch := client.MergeFrom(d.DeepCopyObject().(client.Object))
		d.Status.Replicas = 6
		g.Expect(c.Status().Patch(ctx, d, patch)).To(Succeed())
		return resourceVersion(g, d.GetResourceVersion())
	}
	applyStatus := func(ctx context.Context, c client.Client, g *WithT) int64 {
		ac := appsv1applyconfigurations.Deployment("test", namespace).
			WithStatus(appsv1applyconfigurations.DeploymentStatus().WithReplicas(7))
		g.Expect(c.Status().Apply(ctx, ac, client.FieldOwner("test"))).To(Succeed())
		return resourceVersion(g, ptr.Deref(ac.ResourceVersion, ""))
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
		result := &appsv1.DeploymentList{}
		g.Expect(c.List(ctx, result)).To(Succeed())
		g.Expect(result.Items).To(HaveLen(1))
		g.Expect(resourceVersion(g, result.Items[0].GetResourceVersion())).To(BeNumerically(">=", <-writtenRV))
	}
	getDeleted := func(ctx context.Context, c client.Client, g *WithT, _ <-chan int64) {
		d := deployment()
		err := c.Get(ctx, client.ObjectKeyFromObject(d), d)
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected a NotFound error, got %v", err)
	}
	listDeleted := func(ctx context.Context, c client.Client, g *WithT, _ <-chan int64) {
		result := &appsv1.DeploymentList{}
		g.Expect(c.List(ctx, result)).To(Succeed())
		g.Expect(result.Items).To(BeEmpty())
	}
	getTerminating := func(ctx context.Context, c client.Client, g *WithT, writtenRV <-chan int64) {
		result := deployment()
		g.Expect(c.Get(ctx, client.ObjectKeyFromObject(result), result)).To(Succeed())
		g.Expect(result.GetDeletionTimestamp()).ToNot(BeNil(), "expected the deletionTimestamp to be set")
		g.Expect(result.GetFinalizers()).To(ConsistOf("test.io/finalizer"))
		g.Expect(resourceVersion(g, result.GetResourceVersion())).To(BeNumerically(">=", <-writtenRV))
	}
	listTerminating := func(ctx context.Context, c client.Client, g *WithT, writtenRV <-chan int64) {
		result := &appsv1.DeploymentList{}
		g.Expect(c.List(ctx, result)).To(Succeed())
		g.Expect(result.Items).To(HaveLen(1))
		g.Expect(result.Items[0].GetDeletionTimestamp()).ToNot(BeNil(), "expected the deletionTimestamp to be set")
		g.Expect(result.Items[0].GetFinalizers()).To(ConsistOf("test.io/finalizer"))
		g.Expect(resourceVersion(g, result.Items[0].GetResourceVersion())).To(BeNumerically(">=", <-writtenRV))
	}

	testCases := []struct {
		name            string
		maybeInitObject func() *appsv1.Deployment
		write           func(ctx context.Context, client client.Client, g *WithT) int64
		read            func(ctx context.Context, client client.Client, g *WithT, writtenRV <-chan int64)
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
			name:            "Get after Update",
			maybeInitObject: deployment,
			write:           update,
			read:            get,
		},
		{
			name:            "List after Update",
			maybeInitObject: deployment,
			write:           update,
			read:            list,
		},
		{
			name:            "Get after Patch",
			maybeInitObject: deployment,
			write:           patch,
			read:            get,
		},
		{
			name:            "List after Patch",
			maybeInitObject: deployment,
			write:           patch,
			read:            list,
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
			name:            "Get after Delete",
			maybeInitObject: deployment,
			write:           deleteObject,
			read:            getDeleted,
		},
		{
			name:            "List after Delete",
			maybeInitObject: deployment,
			write:           deleteObject,
			read:            listDeleted,
		},
		{
			name:            "Get after Delete of an object without uid",
			maybeInitObject: deployment,
			write:           deleteObjectWithoutUID,
			read:            getDeleted,
		},
		{
			name:            "List after Delete of an object without uid",
			maybeInitObject: deployment,
			write:           deleteObjectWithoutUID,
			read:            listDeleted,
		},
		{
			name:            "Get after Delete of an object whose uid is only in the preconditions",
			maybeInitObject: deployment,
			write:           deleteObjectWithUIDPrecondition,
			read:            getDeleted,
		},
		{
			name:            "List after Delete of an object whose uid is only in the preconditions",
			maybeInitObject: deployment,
			write:           deleteObjectWithUIDPrecondition,
			read:            listDeleted,
		},
		{
			name:            "Get after Delete of an object with finalizers",
			maybeInitObject: deploymentWithFinalizer,
			write:           deleteObject,
			read:            getTerminating,
		},
		{
			name:            "List after Delete of an object with finalizers",
			maybeInitObject: deploymentWithFinalizer,
			write:           deleteObject,
			read:            listTerminating,
		},
		{
			name:            "Get after status Update",
			maybeInitObject: deployment,
			write:           updateStatus,
			read:            get,
		},
		{
			name:            "List after status Update",
			maybeInitObject: deployment,
			write:           updateStatus,
			read:            list,
		},
		{
			name:            "Get after status Patch",
			maybeInitObject: deployment,
			write:           patchStatus,
			read:            get,
		},
		{
			name:            "List after status Patch",
			maybeInitObject: deployment,
			write:           patchStatus,
			read:            list,
		},
		{
			name:            "Get after status Apply",
			maybeInitObject: deployment,
			write:           applyStatus,
			read:            get,
		},
		{
			name:            "List after status Apply",
			maybeInitObject: deployment,
			write:           applyStatus,
			read:            list,
		},
		{
			name:            "Get after scale Update",
			maybeInitObject: deployment,
			write:           updateScale,
			read:            get,
		},
		{
			name:            "List after scale Update",
			maybeInitObject: deployment,
			write:           updateScale,
			read:            list,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				g := NewWithT(t)
				barrier := keyWriteBarrierWithBeginCallback{WriteBarrier: writebarrier.NewWriteBarrier()}

				var initObjects []client.Object
				if tc.maybeInitObject != nil {
					initObjects = []client.Object{tc.maybeInitObject()}
				}
				c := newConsistentFakeClient(t, &barrier, initObjects...)
				synctest.Wait() // wait for cache start to finish

				writtenRV := make(chan int64, 1)
				callBackFinished := make(chan struct{})
				barrier.beginCallback = sync.OnceFunc(func() {
					// Must happen in a goroutine otherwise we deadlock, as we are waiting for the write to finish while
					// blocking it from starting.
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

func TestConsistentFakeClientDisableReadYourWritesConsistency(t *testing.T) {
	t.Parallel()

	const namespace = "default"

	deployment := func() *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: namespace, UID: "test-uid"},
		}
	}

	get := func(ctx context.Context, c client.Client, g *WithT) {
		d := deployment()
		g.Expect(c.Get(ctx, client.ObjectKeyFromObject(d), d)).To(Succeed())
	}
	list := func(ctx context.Context, c client.Client, g *WithT) {
		g.Expect(c.List(ctx, &appsv1.DeploymentList{})).To(Succeed())
	}

	testCases := []struct {
		name            string
		maybeInitObject func() *appsv1.Deployment
		write           func(ctx context.Context, client client.Client, g *WithT)
		read            func(ctx context.Context, client client.Client, g *WithT)
	}{
		{
			name: "Get",
			write: func(ctx context.Context, c client.Client, g *WithT) {
				g.Expect(c.Create(ctx, deployment())).To(Succeed())
			},
			read: func(ctx context.Context, c client.Client, g *WithT) {
				d := deployment()
				g.Expect(c.Get(ctx, client.ObjectKeyFromObject(d), d, client.DisableReadYourWritesConsistency)).To(Succeed())
			},
		},
		{
			name: "List",
			write: func(ctx context.Context, c client.Client, g *WithT) {
				g.Expect(c.Create(ctx, deployment())).To(Succeed())
			},
			read: func(ctx context.Context, c client.Client, g *WithT) {
				g.Expect(c.List(ctx, &appsv1.DeploymentList{}, client.DisableReadYourWritesConsistency)).To(Succeed())
			},
		},
		{
			name: "Create",
			write: func(ctx context.Context, c client.Client, g *WithT) {
				g.Expect(c.Create(ctx, deployment(), client.DisableReadYourWritesConsistency)).To(Succeed())
			},
			read: get,
		},
		{
			name:            "Update",
			maybeInitObject: deployment,
			write: func(ctx context.Context, c client.Client, g *WithT) {
				d := deployment()
				d.SetLabels(map[string]string{"updated": "true"})
				g.Expect(c.Update(ctx, d, client.DisableReadYourWritesConsistency)).To(Succeed())
			},
			read: get,
		},
		{
			name:            "Patch",
			maybeInitObject: deployment,
			write: func(ctx context.Context, c client.Client, g *WithT) {
				d := deployment()
				patch := client.MergeFrom(d.DeepCopyObject().(client.Object))
				d.SetLabels(map[string]string{"patched": "true"})
				g.Expect(c.Patch(ctx, d, patch, client.DisableReadYourWritesConsistency)).To(Succeed())
			},
			read: get,
		},
		{
			name: "Apply",
			write: func(ctx context.Context, c client.Client, g *WithT) {
				ac := appsv1applyconfigurations.Deployment(deployment().GetName(), namespace).
					WithLabels(map[string]string{"applied": "true"})
				g.Expect(c.Apply(ctx, ac, client.FieldOwner("test"), client.DisableReadYourWritesConsistency)).To(Succeed())
			},
			read: get,
		},
		{
			name:            "Delete",
			maybeInitObject: deployment,
			write: func(ctx context.Context, c client.Client, g *WithT) {
				g.Expect(c.Delete(ctx, deployment(), client.DisableReadYourWritesConsistency)).To(Succeed())
			},
			read: list,
		},
		{
			name:            "Status Update",
			maybeInitObject: deployment,
			write: func(ctx context.Context, c client.Client, g *WithT) {
				d := deployment()
				d.Status.Replicas = 5
				g.Expect(c.Status().Update(ctx, d, client.DisableReadYourWritesConsistency)).To(Succeed())
			},
			read: get,
		},
		{
			name:            "Status Patch",
			maybeInitObject: deployment,
			write: func(ctx context.Context, c client.Client, g *WithT) {
				d := deployment()
				patch := client.MergeFrom(d.DeepCopyObject().(client.Object))
				d.Status.Replicas = 6
				g.Expect(c.Status().Patch(ctx, d, patch, client.DisableReadYourWritesConsistency)).To(Succeed())
			},
			read: get,
		},
		{
			name:            "Status Apply",
			maybeInitObject: deployment,
			write: func(ctx context.Context, c client.Client, g *WithT) {
				ac := appsv1applyconfigurations.Deployment(deployment().GetName(), namespace).
					WithStatus(appsv1applyconfigurations.DeploymentStatus().WithReplicas(7))
				g.Expect(c.Status().Apply(ctx, ac, client.FieldOwner("test"), client.DisableReadYourWritesConsistency)).To(Succeed())
			},
			read: get,
		},
		{
			name:            "Scale Update",
			maybeInitObject: deployment,
			write: func(ctx context.Context, c client.Client, g *WithT) {
				scale := &autoscalingv1.Scale{Spec: autoscalingv1.ScaleSpec{Replicas: 8}}
				g.Expect(c.SubResource("scale").Update(ctx, deployment(), client.WithSubResourceBody(scale), client.DisableReadYourWritesConsistency)).To(Succeed())
			},
			read: get,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				g := NewWithT(t)

				var initObjects []client.Object
				if tc.maybeInitObject != nil {
					initObjects = []client.Object{tc.maybeInitObject()}
				}
				c := newConsistentFakeClient(t, writebarrier.NewWriteBarrier(), initObjects...)
				synctest.Wait() // wait for cache start to finish

				tc.write(t.Context(), c, g)

				start := time.Now()
				tc.read(t.Context(), c, g)
				g.Expect(time.Since(start)).To(BeZero())

				// Let the pending cache event be delivered, the bubble can not
				// end while a goroutine in it is still blocked.
				time.Sleep(watchDelay)
			})
		})
	}
}

// watchDelay is how long the fake cache lags behind the fake client.
const watchDelay = 10 * time.Second

// newConsistentFakeClient returns a consistent client backed by a fakeclient
// and a fake cache that delivers events with a 10 second delay.
func newConsistentFakeClient(
	t *testing.T,
	barrier writebarrier.WriteBarrier,
	initObjects ...client.Object,
) client.Client {
	t.Helper()

	fc := newFakeCache()
	upstream := fake.NewClientBuilder().
		WithGlobalResourceVersionCounter().
		WithObjects(initObjects...).
		WithInterceptorFuncs(fc.interceptorFuncs()).
		Build()

	return client.NewConsistentClient(
		&fakeConsistentClientUpstream{Client: upstream},
		fc,
		func() writebarrier.WriteBarrier { return barrier },
	)
}

type fakeConsistentClientUpstream struct {
	client.Client
}

func (u *fakeConsistentClientUpstream) DeleteWithResult(ctx context.Context, obj client.Object, opts ...client.DeleteOption) (*unstructured.Unstructured, error) {
	if err := u.Delete(ctx, obj, opts...); err != nil {
		return nil, err
	}

	gvk, err := u.GroupVersionKindFor(obj)
	if err != nil {
		return nil, err
	}
	result := &unstructured.Unstructured{}
	result.SetGroupVersionKind(gvk)
	if err := u.Get(ctx, client.ObjectKeyFromObject(obj), result); err != nil {
		if apierrors.IsNotFound(err) {
			return &unstructured.Unstructured{}, nil
		}
		return nil, err
	}

	return result, nil
}

// fakeCache delivers events to eventhandlers from a client using interceptorFuncs
// with a fixed deploy.
type fakeCache struct {
	cacheapi.Informers

	informer *fakeInformer
}

func newFakeCache() *fakeCache {
	return &fakeCache{
		informer: &fakeInformer{},
	}
}

func (c *fakeCache) GetInformer(ctx context.Context, obj cacheapi.Object, opts ...cacheapi.InformerGetOption) (cacheapi.Informer, error) {
	return c.informer, nil
}

func (c *fakeCache) interceptorFuncs() interceptor.Funcs {
	notify := func(event func(h toolscache.ResourceEventHandler)) {
		go func() {
			time.Sleep(watchDelay)
			c.informer.dispatch(event)
		}()
	}

	notifyApply := func(ac runtime.ApplyConfiguration) {
		updated := &appsv1.Deployment{}
		data, err := json.Marshal(ac)
		if err != nil {
			panic(err)
		}
		if err := json.Unmarshal(data, updated); err != nil {
			panic(err)
		}
		notify(func(h toolscache.ResourceEventHandler) { h.OnUpdate(nil, updated) })
	}

	return interceptor.Funcs{
		Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if err := cl.Create(ctx, obj, opts...); err != nil {
				return err
			}
			notify(func(h toolscache.ResourceEventHandler) { h.OnAdd(obj, false) })
			return nil
		},
		Update: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if err := cl.Update(ctx, obj, opts...); err != nil {
				return err
			}
			notify(func(h toolscache.ResourceEventHandler) { h.OnUpdate(nil, obj) })
			return nil
		},
		Patch: func(ctx context.Context, cl client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			if err := cl.Patch(ctx, obj, patch, opts...); err != nil {
				return err
			}
			notify(func(h toolscache.ResourceEventHandler) { h.OnUpdate(nil, obj) })
			return nil
		},
		Apply: func(ctx context.Context, cl client.WithWatch, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
			if err := cl.Apply(ctx, obj, opts...); err != nil {
				return err
			}
			notifyApply(obj)
			return nil
		},
		Delete: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			// The passed object may not have a UID, so fetch the object first to get that
			beforeDelete := &appsv1.Deployment{}
			if err := cl.Get(ctx, client.ObjectKeyFromObject(obj), beforeDelete); err != nil {
				return err
			}

			if err := cl.Delete(ctx, obj, opts...); err != nil {
				return err
			}

			// the fake client doesn't update the passed obj on delete, so fetch it
			// and if that returns something, use it for the event.
			result := &appsv1.Deployment{}
			if err := cl.Get(ctx, client.ObjectKeyFromObject(obj), result); err != nil {
				if !apierrors.IsNotFound(err) {
					panic(err)
				}
				notify(func(h toolscache.ResourceEventHandler) { h.OnDelete(beforeDelete) })
				return nil
			}
			notify(func(h toolscache.ResourceEventHandler) { h.OnUpdate(nil, result) })
			return nil
		},
		SubResourceUpdate: func(ctx context.Context, cl client.Client, subResourceName string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			if err := cl.SubResource(subResourceName).Update(ctx, obj, opts...); err != nil {
				return err
			}
			notify(func(h toolscache.ResourceEventHandler) { h.OnUpdate(nil, obj) })
			return nil
		},
		SubResourcePatch: func(ctx context.Context, cl client.Client, subResourceName string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
			if err := cl.SubResource(subResourceName).Patch(ctx, obj, patch, opts...); err != nil {
				return err
			}
			notify(func(h toolscache.ResourceEventHandler) { h.OnUpdate(nil, obj) })
			return nil
		},
		SubResourceApply: func(ctx context.Context, cl client.Client, subResourceName string, obj runtime.ApplyConfiguration, opts ...client.SubResourceApplyOption) error {
			if err := cl.SubResource(subResourceName).Apply(ctx, obj, opts...); err != nil {
				return err
			}
			notifyApply(obj)
			return nil
		},
	}
}

type fakeInformer struct {
	cacheapi.Informer

	mu       sync.Mutex
	handlers []toolscache.ResourceEventHandler
}

func (i *fakeInformer) AddEventHandler(handler toolscache.ResourceEventHandler) (toolscache.ResourceEventHandlerRegistration, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.handlers = append(i.handlers, handler)
	return fakeRegistration{}, nil
}

func (i *fakeInformer) dispatch(event func(h toolscache.ResourceEventHandler)) {
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, h := range i.handlers {
		event(h)
	}
}

type fakeRegistration struct {
	toolscache.ResourceEventHandlerRegistration
}

func (fakeRegistration) HasSyncedChecker() toolscache.DoneChecker { return fakeDoneChecker{} }

type fakeDoneChecker struct {
	toolscache.DoneChecker
}

func (fakeDoneChecker) Done() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

type keyWriteBarrierWithBeginCallback struct {
	writebarrier.WriteBarrier
	beginCallback func()
}

func (k *keyWriteBarrierWithBeginCallback) Begin() func() {
	release := k.WriteBarrier.Begin()
	k.beginCallback()
	return release
}
