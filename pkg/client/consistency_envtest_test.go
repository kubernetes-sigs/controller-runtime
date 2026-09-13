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
	"fmt"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ConsistentClient", func() {
	var counter atomic.Uint64

	type writeResult struct {
		name            string
		deleted         bool
		resourceVersion string
	}

	type objectRepresentation struct {
		name        string
		newObject   func() client.Object
		newList     func() client.ObjectList
		applyConfig func(namespace, name string, annotations map[string]string) (runtime.ApplyConfiguration, func() string)
	}

	configMapGVK := corev1.SchemeGroupVersion.WithKind("ConfigMap")

	representations := []objectRepresentation{{
		name:      "typed",
		newObject: func() client.Object { return &corev1.ConfigMap{} },
		newList:   func() client.ObjectList { return &corev1.ConfigMapList{} },
		applyConfig: func(namespace, name string, annotations map[string]string) (runtime.ApplyConfiguration, func() string) {
			ac := corev1ac.ConfigMap(name, namespace).WithAnnotations(annotations)
			return ac, func() string { return ptr.Deref(ac.ResourceVersion, "") }
		},
	}, {
		name: "unstructured",
		newObject: func() client.Object {
			u := &unstructured.Unstructured{}
			u.SetGroupVersionKind(configMapGVK)
			return u
		},
		newList: func() client.ObjectList {
			l := &unstructured.UnstructuredList{}
			l.SetGroupVersionKind(configMapGVK.GroupVersion().WithKind(configMapGVK.Kind + "List"))
			return l
		},
		applyConfig: func(namespace, name string, annotations map[string]string) (runtime.ApplyConfiguration, func() string) {
			u := &unstructured.Unstructured{}
			u.SetGroupVersionKind(configMapGVK)
			u.SetNamespace(namespace)
			u.SetName(name)
			u.SetAnnotations(annotations)
			return client.ApplyConfigurationFromUnstructured(u), u.GetResourceVersion
		},
	}}

	for _, repr := range representations {
		Context(repr.name, func() {
			var (
				cl     client.Client
				ctx    context.Context
				cancel context.CancelFunc
			)

			BeforeEach(func(specCtx SpecContext) {
				// NB: Don't derive from the BeforeEach's context, Ginkgo cancels it when the
				// node returns and it thus would not outlive it, stopping the cache's watches.
				ctx, cancel = context.WithCancel(context.WithoutCancel(specCtx))

				c, err := cache.New(cfg, cache.Options{Scheme: kscheme.Scheme})
				Expect(err).NotTo(HaveOccurred())

				// Set up informers for types used through the consistent client.
				_, err = c.GetInformer(ctx, repr.newObject())
				Expect(err).NotTo(HaveOccurred())
				_, err = c.GetInformer(ctx, &corev1.Namespace{})
				Expect(err).NotTo(HaveOccurred())

				go func() {
					defer GinkgoRecover()
					Expect(c.Start(ctx)).To(Succeed())
				}()
				Expect(c.WaitForCacheSync(ctx)).To(BeTrue())

				cl, err = client.New(cfg, client.Options{
					Scheme: kscheme.Scheme,
					Cache: &client.CacheOptions{
						Reader:                          c,
						EnableReadYourWritesConsistency: new(true),
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				cancel()
			})

			newConfigMap := func(ns string) client.Object {
				cm := repr.newObject()
				cm.SetNamespace(ns)
				cm.SetName(fmt.Sprintf("consistency-test-%d", counter.Add(1)))
				cm.SetAnnotations(map[string]string{"key": "value"})
				return cm
			}

			DescribeTable("write then read",
				func(ctx context.Context, write func(ctx context.Context, cl client.Client, cm client.Object) (writeResult, error)) {
					ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("consistency-wtr-%d", counter.Add(1))}}
					Expect(cl.Create(ctx, ns)).To(Succeed())
					DeferCleanup(func(ctx context.Context) {
						Expect(client.IgnoreNotFound(cl.Delete(ctx, ns))).To(Succeed())
					})

					cm := newConfigMap(ns.Name)
					Expect(cl.Create(ctx, cm)).To(Succeed())
					DeferCleanup(func(ctx context.Context) {
						Expect(client.IgnoreNotFound(cl.Delete(ctx, cm))).To(Succeed())
					})

					result, err := write(ctx, cl, cm)
					Expect(err).NotTo(HaveOccurred())

					done := make(chan struct{})

					go func() {
						defer GinkgoRecover()
						defer func() { done <- struct{}{} }()

						if result.deleted {
							err := cl.Get(ctx, client.ObjectKeyFromObject(cm), repr.newObject())
							Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected NotFound after delete, got: %v", err)
						} else {
							got := repr.newObject()
							Expect(cl.Get(ctx, client.ObjectKeyFromObject(cm), got)).To(Succeed())
							Expect(got.GetName()).To(Equal(result.name))
							Expect(got.GetResourceVersion()).To(Equal(result.resourceVersion))
						}
					}()

					go func() {
						defer GinkgoRecover()
						defer func() { done <- struct{}{} }()

						list := repr.newList()
						Expect(cl.List(ctx, list, client.InNamespace(ns.Name))).To(Succeed())
						items, err := apimeta.ExtractList(list)
						Expect(err).NotTo(HaveOccurred())

						if result.deleted {
							Expect(items).To(BeEmpty(), "list should be empty after delete")
						} else {
							Expect(items).To(HaveLen(1), "list should contain exactly one ConfigMap")
							item := items[0].(client.Object)
							Expect(item.GetName()).To(Equal(result.name))
							Expect(item.GetResourceVersion()).To(Equal(result.resourceVersion))
						}
					}()

					<-done
					<-done
				},

				Entry("create", func(ctx context.Context, cl client.Client, cm client.Object) (writeResult, error) {
					return writeResult{
						name:            cm.GetName(),
						resourceVersion: cm.GetResourceVersion(),
					}, nil // already created in the setup
				}),

				Entry("update", func(ctx context.Context, cl client.Client, cm client.Object) (writeResult, error) {
					got := repr.newObject()
					if err := cl.Get(ctx, client.ObjectKeyFromObject(cm), got); err != nil {
						return writeResult{}, err
					}
					got.SetAnnotations(map[string]string{"key": "updated"})
					if err := cl.Update(ctx, got); err != nil {
						return writeResult{}, err
					}
					return writeResult{
						name:            cm.GetName(),
						resourceVersion: got.GetResourceVersion(),
					}, nil
				}),

				Entry("patch", func(ctx context.Context, cl client.Client, cm client.Object) (writeResult, error) {
					got := repr.newObject()
					if err := cl.Get(ctx, client.ObjectKeyFromObject(cm), got); err != nil {
						return writeResult{}, err
					}
					patch := client.MergeFrom(got.DeepCopyObject().(client.Object))
					got.SetAnnotations(map[string]string{"key": "value", "patched": "yes"})
					if err := cl.Patch(ctx, got, patch); err != nil {
						return writeResult{}, err
					}
					return writeResult{
						name:            cm.GetName(),
						resourceVersion: got.GetResourceVersion(),
					}, nil
				}),

				Entry("apply", func(ctx context.Context, cl client.Client, cm client.Object) (writeResult, error) {
					ac, resourceVersion := repr.applyConfig(cm.GetNamespace(), cm.GetName(), map[string]string{"key": "applied"})
					if err := cl.Apply(ctx, ac, client.FieldOwner("consistency-test"), client.ForceOwnership); err != nil {
						return writeResult{}, err
					}
					return writeResult{
						name:            cm.GetName(),
						resourceVersion: resourceVersion(),
					}, nil
				}),

				Entry("delete", func(ctx context.Context, cl client.Client, cm client.Object) (writeResult, error) {
					if err := cl.Delete(ctx, cm); err != nil {
						return writeResult{}, err
					}
					return writeResult{
						name:    cm.GetName(),
						deleted: true,
					}, nil
				}),
			)

			Describe("Delete object with finalizer then Get", func() {
				It("should observe the updated object with deletion timestamp after delete", func() {
					cm := newConfigMap("default")
					cm.SetFinalizers([]string{"test.io/hold"})
					Expect(cl.Create(ctx, cm)).To(Succeed())
					DeferCleanup(func(ctx context.Context) {
						got := repr.newObject()
						if err := cl.Get(ctx, client.ObjectKeyFromObject(cm), got); err == nil {
							got.SetFinalizers(nil)
							Expect(cl.Update(ctx, got)).To(Succeed())
						}
					})

					got := repr.newObject()
					Expect(cl.Get(ctx, client.ObjectKeyFromObject(cm), got)).To(Succeed())

					Expect(cl.Delete(ctx, cm)).To(Succeed())

					afterDelete := repr.newObject()
					Expect(cl.Get(ctx, client.ObjectKeyFromObject(cm), afterDelete)).To(Succeed())
					Expect(afterDelete.GetDeletionTimestamp()).NotTo(BeNil(), "should have a deletion timestamp")
				})
			})
		})
	}

	Describe("SubResource Create", func() {
		var (
			cl     client.Client
			ctx    context.Context
			cancel context.CancelFunc
		)

		BeforeEach(func(specCtx SpecContext) {
			// NB: Don't derive from the BeforeEach's context, Ginkgo cancels it when the
			// node returns and it thus would not outlive it, stopping the cache's watches.
			ctx, cancel = context.WithCancel(context.WithoutCancel(specCtx))

			c, err := cache.New(cfg, cache.Options{Scheme: kscheme.Scheme})
			Expect(err).NotTo(HaveOccurred())

			_, err = c.GetInformer(ctx, &corev1.Pod{})
			Expect(err).NotTo(HaveOccurred())
			_, err = c.GetInformer(ctx, &corev1.Namespace{})
			Expect(err).NotTo(HaveOccurred())

			go func() {
				defer GinkgoRecover()
				Expect(c.Start(ctx)).To(Succeed())
			}()
			Expect(c.WaitForCacheSync(ctx)).To(BeTrue())

			cl, err = client.New(cfg, client.Options{
				Scheme: kscheme.Scheme,
				Cache: &client.CacheOptions{
					Reader:                          c,
					EnableReadYourWritesConsistency: new(true),
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			cancel()
		})

		It("evicts a pod without erroring", func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("consistency-evict-%d", counter.Add(1))}}
			Expect(cl.Create(ctx, ns)).To(Succeed())
			DeferCleanup(func(ctx context.Context) {
				Expect(client.IgnoreNotFound(cl.Delete(ctx, ns))).To(Succeed())
			})

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns.Name, Name: fmt.Sprintf("consistency-evict-%d", counter.Add(1))},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "c", Image: "example.com/does-not-matter"}},
				},
			}
			Expect(cl.Create(ctx, pod)).To(Succeed())

			// This is the scenario from https://github.com/kubernetes-sigs/controller-runtime/issues/3590:
			// the eviction response is decoded into a policy/v1.Eviction, not the Pod, so the client
			// must not try to read a resource version to wait for off the Pod after this write.
			eviction := &policyv1.Eviction{ObjectMeta: metav1.ObjectMeta{Namespace: pod.Namespace, Name: pod.Name}}
			Expect(cl.SubResource("eviction").Create(ctx, pod, eviction)).To(Succeed())
		})
	})
})
