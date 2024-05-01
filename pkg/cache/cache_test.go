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

package cache_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	kcache "k8s.io/client-go/tools/cache"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllertest"
)

const testNodeOne = "test-node-1"
const testNodeTwo = "test-node-2"
const testNamespaceOne = "test-namespace-1"
const testNamespaceTwo = "test-namespace-2"
const testNamespaceThree = "test-namespace-3"

// TODO(community): Pull these helper functions into testenv.
// Restart policy is included to allow indexing on that field.
func createPodWithLabels(name, namespace string, restartPolicy corev1.RestartPolicy, labels map[string]string) client.Object {
	three := int64(3)
	if labels == nil {
		labels = map[string]string{}
	}
	labels["test-label"] = name
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers:            []corev1.Container{{Name: "nginx", Image: "nginx"}},
			RestartPolicy:         restartPolicy,
			ActiveDeadlineSeconds: &three,
		},
	}
	cl, err := client.New(cfg, client.Options{})
	Expect(err).NotTo(HaveOccurred())
	err = cl.Create(context.Background(), pod)
	Expect(err).NotTo(HaveOccurred())
	return pod
}

func createSvc(name, namespace string, cl client.Client) client.Object {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Port: 1}},
		},
	}
	err := cl.Create(context.Background(), svc)
	Expect(err).NotTo(HaveOccurred())
	return svc
}

func createSA(name, namespace string, cl client.Client) client.Object {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	err := cl.Create(context.Background(), sa)
	Expect(err).NotTo(HaveOccurred())
	return sa
}

func createPod(name, namespace string, restartPolicy corev1.RestartPolicy) client.Object {
	return createPodWithLabels(name, namespace, restartPolicy, nil)
}

func deletePod(pod client.Object) {
	cl, err := client.New(cfg, client.Options{})
	Expect(err).NotTo(HaveOccurred())
	err = cl.Delete(context.Background(), pod)
	Expect(err).NotTo(HaveOccurred())
}

var _ = Describe("Informer Cache", func() {
	CacheTest(cache.New, cache.Options{})
	NonBlockingGetTest(cache.New, cache.Options{})
})

var _ = Describe("Informer Cache with ReaderFailOnMissingInformer", func() {
	CacheTestReaderFailOnMissingInformer(cache.New, cache.Options{ReaderFailOnMissingInformer: true})
})

var _ = Describe("Multi-Namespace Informer Cache", func() {
	CacheTest(cache.New, cache.Options{
		DefaultNamespaces: map[string]cache.Config{
			testNamespaceOne: {},
			testNamespaceTwo: {},
			"default":        {},
		},
	})
	NonBlockingGetTest(cache.New, cache.Options{
		DefaultNamespaces: map[string]cache.Config{
			testNamespaceOne: {},
			testNamespaceTwo: {},
			"default":        {},
		},
	})
})

var _ = Describe("Informer Cache without global DeepCopy", func() {
	CacheTest(cache.New, cache.Options{
		DefaultUnsafeDisableDeepCopy: ptr.To(true),
	})
	NonBlockingGetTest(cache.New, cache.Options{
		DefaultUnsafeDisableDeepCopy: ptr.To(true),
	})
})

var _ = Describe("Cache with transformers", func() {
	var (
		informerCache       cache.Cache
		informerCacheCtx    context.Context
		informerCacheCancel context.CancelFunc
		knownPod1           client.Object
		knownPod2           client.Object
		knownPod3           client.Object
		knownPod4           client.Object
		knownPod5           client.Object
		knownPod6           client.Object
	)

	getTransformValue := func(obj client.Object) string {
		accessor, err := meta.Accessor(obj)
		if err == nil {
			annotations := accessor.GetAnnotations()
			if val, exists := annotations["transformed"]; exists {
				return val
			}
		}
		return ""
	}

	BeforeEach(func() {
		informerCacheCtx, informerCacheCancel = context.WithCancel(context.Background())
		Expect(cfg).NotTo(BeNil())

		By("creating three pods")
		cl, err := client.New(cfg, client.Options{})
		Expect(err).NotTo(HaveOccurred())
		err = ensureNode(testNodeOne, cl)
		Expect(err).NotTo(HaveOccurred())
		err = ensureNamespace(testNamespaceOne, cl)
		Expect(err).NotTo(HaveOccurred())
		err = ensureNamespace(testNamespaceTwo, cl)
		Expect(err).NotTo(HaveOccurred())
		err = ensureNamespace(testNamespaceThree, cl)
		Expect(err).NotTo(HaveOccurred())
		// Includes restart policy since these objects are indexed on this field.
		knownPod1 = createPod("test-pod-1", testNamespaceOne, corev1.RestartPolicyNever)
		knownPod2 = createPod("test-pod-2", testNamespaceTwo, corev1.RestartPolicyAlways)
		knownPod3 = createPodWithLabels("test-pod-3", testNamespaceTwo, corev1.RestartPolicyOnFailure, map[string]string{"common-label": "common"})
		knownPod4 = createPodWithLabels("test-pod-4", testNamespaceThree, corev1.RestartPolicyNever, map[string]string{"common-label": "common"})
		knownPod5 = createPod("test-pod-5", testNamespaceOne, corev1.RestartPolicyNever)
		knownPod6 = createPod("test-pod-6", testNamespaceTwo, corev1.RestartPolicyAlways)

		podGVK := schema.GroupVersionKind{
			Kind:    "Pod",
			Version: "v1",
		}

		knownPod1.GetObjectKind().SetGroupVersionKind(podGVK)
		knownPod2.GetObjectKind().SetGroupVersionKind(podGVK)
		knownPod3.GetObjectKind().SetGroupVersionKind(podGVK)
		knownPod4.GetObjectKind().SetGroupVersionKind(podGVK)
		knownPod5.GetObjectKind().SetGroupVersionKind(podGVK)
		knownPod6.GetObjectKind().SetGroupVersionKind(podGVK)

		By("creating the informer cache")
		informerCache, err = cache.New(cfg, cache.Options{
			DefaultTransform: func(i interface{}) (interface{}, error) {
				obj := i.(runtime.Object)
				Expect(obj).NotTo(BeNil())

				accessor, err := meta.Accessor(obj)
				Expect(err).ToNot(HaveOccurred())
				annotations := accessor.GetAnnotations()

				if _, exists := annotations["transformed"]; exists {
					// Avoid performing transformation multiple times.
					return i, nil
				}

				if annotations == nil {
					annotations = make(map[string]string)
				}
				annotations["transformed"] = "default"
				accessor.SetAnnotations(annotations)
				return i, nil
			},
			ByObject: map[client.Object]cache.ByObject{
				&corev1.Pod{}: {
					Transform: func(i interface{}) (interface{}, error) {
						obj := i.(runtime.Object)
						Expect(obj).NotTo(BeNil())
						accessor, err := meta.Accessor(obj)
						Expect(err).ToNot(HaveOccurred())

						annotations := accessor.GetAnnotations()
						if _, exists := annotations["transformed"]; exists {
							// Avoid performing transformation multiple times.
							return i, nil
						}

						if annotations == nil {
							annotations = make(map[string]string)
						}
						annotations["transformed"] = "explicit"
						accessor.SetAnnotations(annotations)
						return i, nil
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		By("running the cache and waiting for it to sync")
		// pass as an arg so that we don't race between close and re-assign
		go func(ctx context.Context) {
			defer GinkgoRecover()
			Expect(informerCache.Start(ctx)).To(Succeed())
		}(informerCacheCtx)
		Expect(informerCache.WaitForCacheSync(informerCacheCtx)).To(BeTrue())
	})

	AfterEach(func() {
		By("cleaning up created pods")
		deletePod(knownPod1)
		deletePod(knownPod2)
		deletePod(knownPod3)
		deletePod(knownPod4)
		deletePod(knownPod5)
		deletePod(knownPod6)

		informerCacheCancel()
	})

	Context("with structured objects", func() {
		It("should apply transformers to explicitly specified GVKS", func() {
			By("listing pods")
			out := corev1.PodList{}
			Expect(informerCache.List(context.Background(), &out)).To(Succeed())

			By("verifying that the returned pods were transformed")
			for i := 0; i < len(out.Items); i++ {
				Expect(getTransformValue(&out.Items[i])).To(BeIdenticalTo("explicit"))
			}
		})

		It("should apply default transformer to objects when none is specified", func() {
			By("getting the Kubernetes service")
			svc := &corev1.Service{}
			svcKey := client.ObjectKey{Namespace: "default", Name: "kubernetes"}
			Expect(informerCache.Get(context.Background(), svcKey, svc)).To(Succeed())

			By("verifying that the returned service was transformed")
			Expect(getTransformValue(svc)).To(BeIdenticalTo("default"))
		})
	})

	Context("with unstructured objects", func() {
		It("should apply transformers to explicitly specified GVKS", func() {
			By("listing pods")
			out := unstructured.UnstructuredList{}
			out.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "PodList",
			})
			Expect(informerCache.List(context.Background(), &out)).To(Succeed())

			By("verifying that the returned pods were transformed")
			for i := 0; i < len(out.Items); i++ {
				Expect(getTransformValue(&out.Items[i])).To(BeIdenticalTo("explicit"))
			}
		})

		It("should apply default transformer to objects when none is specified", func() {
			By("getting the Kubernetes service")
			svc := &unstructured.Unstructured{}
			svc.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "Service",
			})
			svcKey := client.ObjectKey{Namespace: "default", Name: "kubernetes"}
			Expect(informerCache.Get(context.Background(), svcKey, svc)).To(Succeed())

			By("verifying that the returned service was transformed")
			Expect(getTransformValue(svc)).To(BeIdenticalTo("default"))
		})
	})

	Context("with metadata-only objects", func() {
		It("should apply transformers to explicitly specified GVKS", func() {
			By("listing pods")
			out := metav1.PartialObjectMetadataList{}
			out.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "PodList",
			})
			Expect(informerCache.List(context.Background(), &out)).To(Succeed())

			By("verifying that the returned pods were transformed")
			for i := 0; i < len(out.Items); i++ {
				Expect(getTransformValue(&out.Items[i])).To(BeIdenticalTo("explicit"))
			}
		})
		It("should apply default transformer to objects when none is specified", func() {
			By("getting the Kubernetes service")
			svc := &metav1.PartialObjectMetadata{}
			svc.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "Service",
			})
			svcKey := client.ObjectKey{Namespace: "default", Name: "kubernetes"}
			Expect(informerCache.Get(context.Background(), svcKey, svc)).To(Succeed())

			By("verifying that the returned service was transformed")
			Expect(getTransformValue(svc)).To(BeIdenticalTo("default"))
		})
	})
})

var _ = Describe("Cache with selectors", func() {
	defer GinkgoRecover()
	var (
		informerCache       cache.Cache
		informerCacheCtx    context.Context
		informerCacheCancel context.CancelFunc
	)

	BeforeEach(func() {
		informerCacheCtx, informerCacheCancel = context.WithCancel(context.Background())
		Expect(cfg).NotTo(BeNil())
		cl, err := client.New(cfg, client.Options{})
		Expect(err).NotTo(HaveOccurred())
		err = ensureNamespace(testNamespaceOne, cl)
		Expect(err).NotTo(HaveOccurred())
		err = ensureNamespace(testNamespaceTwo, cl)
		Expect(err).NotTo(HaveOccurred())
		for idx, namespace := range []string{testNamespaceOne, testNamespaceTwo} {
			_ = createSA("test-sa-"+strconv.Itoa(idx), namespace, cl)
			_ = createSvc("test-svc-"+strconv.Itoa(idx), namespace, cl)
		}

		opts := cache.Options{
			DefaultFieldSelector: fields.OneTermEqualSelector("metadata.namespace", testNamespaceTwo),
			ByObject: map[client.Object]cache.ByObject{
				&corev1.ServiceAccount{}: {
					Field: fields.OneTermEqualSelector("metadata.namespace", testNamespaceOne),
				},
			},
		}

		By("creating the informer cache")
		informerCache, err = cache.New(cfg, opts)
		Expect(err).NotTo(HaveOccurred())
		By("running the cache and waiting for it to sync")
		// pass as an arg so that we don't race between close and re-assign
		go func(ctx context.Context) {
			defer GinkgoRecover()
			Expect(informerCache.Start(ctx)).To(Succeed())
		}(informerCacheCtx)
		Expect(informerCache.WaitForCacheSync(informerCacheCtx)).To(BeTrue())
	})

	AfterEach(func() {
		ctx := context.Background()
		cl, err := client.New(cfg, client.Options{})
		Expect(err).NotTo(HaveOccurred())
		for idx, namespace := range []string{testNamespaceOne, testNamespaceTwo} {
			err = cl.Delete(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "test-sa-" + strconv.Itoa(idx)}})
			Expect(err).NotTo(HaveOccurred())
			err = cl.Delete(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "test-svc-" + strconv.Itoa(idx)}})
			Expect(err).NotTo(HaveOccurred())
		}
		informerCacheCancel()
	})

	It("Should list serviceaccounts and find exactly one in namespace "+testNamespaceOne, func() {
		var sas corev1.ServiceAccountList
		err := informerCache.List(informerCacheCtx, &sas)
		Expect(err).NotTo(HaveOccurred())
		Expect(sas.Items).To(HaveLen(1))
		Expect(sas.Items[0].Namespace).To(Equal(testNamespaceOne))
	})

	It("Should list services and find exactly one in namespace "+testNamespaceTwo, func() {
		var svcs corev1.ServiceList
		err := informerCache.List(informerCacheCtx, &svcs)
		Expect(err).NotTo(HaveOccurred())
		Expect(svcs.Items).To(HaveLen(1))
		Expect(svcs.Items[0].Namespace).To(Equal(testNamespaceTwo))
	})
})

func CacheTestReaderFailOnMissingInformer(createCacheFunc func(config *rest.Config, opts cache.Options) (cache.Cache, error), opts cache.Options) {
	Describe("Cache test with ReaderFailOnMissingInformer = true", func() {
		var (
			informerCache       cache.Cache
			informerCacheCtx    context.Context
			informerCacheCancel context.CancelFunc
			errNotCached        *cache.ErrResourceNotCached
		)

		BeforeEach(func() {
			informerCacheCtx, informerCacheCancel = context.WithCancel(context.Background())
			Expect(cfg).NotTo(BeNil())
			By("creating the informer cache")
			var err error
			informerCache, err = createCacheFunc(cfg, opts)
			Expect(err).NotTo(HaveOccurred())
			By("running the cache and waiting for it to sync")
			// pass as an arg so that we don't race between close and re-assign
			go func(ctx context.Context) {
				defer GinkgoRecover()
				Expect(informerCache.Start(ctx)).To(Succeed())
			}(informerCacheCtx)
			Expect(informerCache.WaitForCacheSync(informerCacheCtx)).To(BeTrue())
		})

		AfterEach(func() {
			informerCacheCancel()
		})

		Describe("as a Reader", func() {
			Context("with structured objects", func() {
				It("should not be able to list objects that haven't been watched previously", func() {
					By("listing all services in the cluster")
					listObj := &corev1.ServiceList{}
					Expect(errors.As(informerCache.List(context.Background(), listObj), &errNotCached)).To(BeTrue())
				})

				It("should not be able to get objects that haven't been watched previously", func() {
					By("getting the Kubernetes service")
					svc := &corev1.Service{}
					svcKey := client.ObjectKey{Namespace: "default", Name: "kubernetes"}
					Expect(errors.As(informerCache.Get(context.Background(), svcKey, svc), &errNotCached)).To(BeTrue())
				})

				It("should be able to list objects that are configured to be watched", func() {
					By("indicating that we need to watch services")
					_, err := informerCache.GetInformer(context.Background(), &corev1.Service{})
					Expect(err).ToNot(HaveOccurred())

					By("listing all services in the cluster")
					svcList := &corev1.ServiceList{}
					Expect(informerCache.List(context.Background(), svcList)).To(Succeed())

					By("verifying that the returned service looks reasonable")
					Expect(svcList.Items).To(HaveLen(1))
					Expect(svcList.Items[0].Name).To(Equal("kubernetes"))
					Expect(svcList.Items[0].Namespace).To(Equal("default"))
				})

				It("should be able to get objects that are configured to be watched", func() {
					By("indicating that we need to watch services")
					_, err := informerCache.GetInformer(context.Background(), &corev1.Service{})
					Expect(err).ToNot(HaveOccurred())

					By("getting the Kubernetes service")
					svc := &corev1.Service{}
					svcKey := client.ObjectKey{Namespace: "default", Name: "kubernetes"}
					Expect(informerCache.Get(context.Background(), svcKey, svc)).To(Succeed())

					By("verifying that the returned service looks reasonable")
					Expect(svc.Name).To(Equal("kubernetes"))
					Expect(svc.Namespace).To(Equal("default"))
				})
			})
		})
	})
}

func NonBlockingGetTest(createCacheFunc func(config *rest.Config, opts cache.Options) (cache.Cache, error), opts cache.Options) {
	Describe("non-blocking get test", func() {
		var (
			informerCache       cache.Cache
			informerCacheCtx    context.Context
			informerCacheCancel context.CancelFunc
		)
		BeforeEach(func() {
			informerCacheCtx, informerCacheCancel = context.WithCancel(context.Background())
			Expect(cfg).NotTo(BeNil())

			By("creating expected namespaces")
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			err = ensureNode(testNodeOne, cl)
			Expect(err).NotTo(HaveOccurred())
			err = ensureNamespace(testNamespaceOne, cl)
			Expect(err).NotTo(HaveOccurred())
			err = ensureNamespace(testNamespaceTwo, cl)
			Expect(err).NotTo(HaveOccurred())
			err = ensureNamespace(testNamespaceThree, cl)
			Expect(err).NotTo(HaveOccurred())

			By("creating the informer cache")
			v := reflect.ValueOf(&opts).Elem()
			newInformerField := v.FieldByName("newInformer")
			newFakeInformer := func(_ kcache.ListerWatcher, _ runtime.Object, _ time.Duration, _ kcache.Indexers) kcache.SharedIndexInformer {
				return &controllertest.FakeInformer{Synced: false}
			}
			reflect.NewAt(newInformerField.Type(), newInformerField.Addr().UnsafePointer()).
				Elem().
				Set(reflect.ValueOf(&newFakeInformer))
			informerCache, err = createCacheFunc(cfg, opts)
			Expect(err).NotTo(HaveOccurred())
			By("running the cache and waiting for it to sync")
			// pass as an arg so that we don't race between close and re-assign
			go func(ctx context.Context) {
				defer GinkgoRecover()
				Expect(informerCache.Start(ctx)).To(Succeed())
			}(informerCacheCtx)
			Expect(informerCache.WaitForCacheSync(informerCacheCtx)).To(BeTrue())
		})

		AfterEach(func() {
			By("cleaning up created pods")
			informerCacheCancel()
		})

		Describe("as an Informer", func() {
			It("should be able to get informer for the object without blocking", func() {
				By("getting a shared index informer for a pod")
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "informer-obj",
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "nginx",
								Image: "nginx",
							},
						},
					},
				}

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				sii, err := informerCache.GetInformer(ctx, pod, cache.BlockUntilSynced(false))
				Expect(err).NotTo(HaveOccurred())
				Expect(sii).NotTo(BeNil())
				Expect(sii.HasSynced()).To(BeFalse())
			})
		})
	})
}

func CacheTest(createCacheFunc func(config *rest.Config, opts cache.Options) (cache.Cache, error), opts cache.Options) {
	Describe("Cache test", func() {
		var (
			informerCache       cache.Cache
			informerCacheCtx    context.Context
			informerCacheCancel context.CancelFunc
			knownPod1           client.Object
			knownPod2           client.Object
			knownPod3           client.Object
			knownPod4           client.Object
			knownPod5           client.Object
			knownPod6           client.Object
		)

		BeforeEach(func() {
			informerCacheCtx, informerCacheCancel = context.WithCancel(context.Background())
			Expect(cfg).NotTo(BeNil())

			By("creating three pods")
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			err = ensureNode(testNodeOne, cl)
			Expect(err).NotTo(HaveOccurred())
			err = ensureNode(testNodeTwo, cl)
			Expect(err).NotTo(HaveOccurred())
			err = ensureNamespace(testNamespaceOne, cl)
			Expect(err).NotTo(HaveOccurred())
			err = ensureNamespace(testNamespaceTwo, cl)
			Expect(err).NotTo(HaveOccurred())
			err = ensureNamespace(testNamespaceThree, cl)
			Expect(err).NotTo(HaveOccurred())
			// Includes restart policy since these objects are indexed on this field.
			knownPod1 = createPod("test-pod-1", testNamespaceOne, corev1.RestartPolicyNever)
			knownPod2 = createPod("test-pod-2", testNamespaceTwo, corev1.RestartPolicyAlways)
			knownPod3 = createPodWithLabels("test-pod-3", testNamespaceTwo, corev1.RestartPolicyOnFailure, map[string]string{"common-label": "common"})
			knownPod4 = createPodWithLabels("test-pod-4", testNamespaceThree, corev1.RestartPolicyNever, map[string]string{"common-label": "common"})
			knownPod5 = createPod("test-pod-5", testNamespaceOne, corev1.RestartPolicyNever)
			knownPod6 = createPod("test-pod-6", testNamespaceTwo, corev1.RestartPolicyAlways)

			podGVK := schema.GroupVersionKind{
				Kind:    "Pod",
				Version: "v1",
			}

			knownPod1.GetObjectKind().SetGroupVersionKind(podGVK)
			knownPod2.GetObjectKind().SetGroupVersionKind(podGVK)
			knownPod3.GetObjectKind().SetGroupVersionKind(podGVK)
			knownPod4.GetObjectKind().SetGroupVersionKind(podGVK)
			knownPod5.GetObjectKind().SetGroupVersionKind(podGVK)
			knownPod6.GetObjectKind().SetGroupVersionKind(podGVK)

			By("creating the informer cache")
			informerCache, err = createCacheFunc(cfg, opts)
			Expect(err).NotTo(HaveOccurred())
			By("running the cache and waiting for it to sync")
			// pass as an arg so that we don't race between close and re-assign
			go func(ctx context.Context) {
				defer GinkgoRecover()
				Expect(informerCache.Start(ctx)).To(Succeed())
			}(informerCacheCtx)
			Expect(informerCache.WaitForCacheSync(informerCacheCtx)).To(BeTrue())
		})

		AfterEach(func() {
			By("cleaning up created pods")
			deletePod(knownPod1)
			deletePod(knownPod2)
			deletePod(knownPod3)
			deletePod(knownPod4)
			deletePod(knownPod5)
			deletePod(knownPod6)

			informerCacheCancel()
		})

		Describe("as a Reader", func() {
			Context("with structured objects", func() {
				It("should be able to list objects that haven't been watched previously", func() {
					By("listing all services in the cluster")
					listObj := &corev1.ServiceList{}
					Expect(informerCache.List(context.Background(), listObj)).To(Succeed())

					By("verifying that the returned list contains the Kubernetes service")
					// NB: kubernetes default service is automatically created in testenv.
					Expect(listObj.Items).NotTo(BeEmpty())
					hasKubeService := false
					for i := range listObj.Items {
						svc := &listObj.Items[i]
						if isKubeService(svc) {
							hasKubeService = true
							break
						}
					}
					Expect(hasKubeService).To(BeTrue())
				})

				It("should be able to get objects that haven't been watched previously", func() {
					By("getting the Kubernetes service")
					svc := &corev1.Service{}
					svcKey := client.ObjectKey{Namespace: "default", Name: "kubernetes"}
					Expect(informerCache.Get(context.Background(), svcKey, svc)).To(Succeed())

					By("verifying that the returned service looks reasonable")
					Expect(svc.Name).To(Equal("kubernetes"))
					Expect(svc.Namespace).To(Equal("default"))
				})

				It("should support filtering by labels in a single namespace", func() {
					By("listing pods with a particular label")
					// NB: each pod has a "test-label": <pod-name>
					out := corev1.PodList{}
					Expect(informerCache.List(context.Background(), &out,
						client.InNamespace(testNamespaceTwo),
						client.MatchingLabels(map[string]string{"test-label": "test-pod-2"}))).To(Succeed())

					By("verifying the returned pods have the correct label")
					Expect(out.Items).NotTo(BeEmpty())
					Expect(out.Items).Should(HaveLen(1))
					actual := out.Items[0]
					Expect(actual.Labels["test-label"]).To(Equal("test-pod-2"))
				})

				It("should support filtering by labels from multiple namespaces", func() {
					By("creating another pod with the same label but different namespace")
					anotherPod := createPod("test-pod-2", testNamespaceOne, corev1.RestartPolicyAlways)
					defer deletePod(anotherPod)

					By("listing pods with a particular label")
					// NB: each pod has a "test-label": <pod-name>
					out := corev1.PodList{}
					labels := map[string]string{"test-label": "test-pod-2"}
					Expect(informerCache.List(context.Background(), &out, client.MatchingLabels(labels))).To(Succeed())

					By("verifying multiple pods with the same label in different namespaces are returned")
					Expect(out.Items).NotTo(BeEmpty())
					Expect(out.Items).Should(HaveLen(2))
					for _, actual := range out.Items {
						Expect(actual.Labels["test-label"]).To(Equal("test-pod-2"))
					}
				})

				if !isPodDisableDeepCopy(opts) {
					It("should be able to list objects with GVK populated", func() {
						By("listing pods")
						out := &corev1.PodList{}
						Expect(informerCache.List(context.Background(), out)).To(Succeed())

						By("verifying that the returned pods have GVK populated")
						Expect(out.Items).NotTo(BeEmpty())
						Expect(out.Items).Should(SatisfyAny(HaveLen(5), HaveLen(6)))
						for _, p := range out.Items {
							Expect(p.GroupVersionKind()).To(Equal(corev1.SchemeGroupVersion.WithKind("Pod")))
						}
					})
				}

				It("should be able to list objects by namespace", func() {
					By("listing pods in test-namespace-1")
					listObj := &corev1.PodList{}
					Expect(informerCache.List(context.Background(), listObj,
						client.InNamespace(testNamespaceOne))).To(Succeed())

					By("verifying that the returned pods are in test-namespace-1")
					Expect(listObj.Items).NotTo(BeEmpty())
					Expect(listObj.Items).Should(HaveLen(2))
					for _, item := range listObj.Items {
						Expect(item.Namespace).To(Equal(testNamespaceOne))
					}
				})

				if !isPodDisableDeepCopy(opts) {
					It("should deep copy the object unless told otherwise", func() {
						By("retrieving a specific pod from the cache")
						out := &corev1.Pod{}
						podKey := client.ObjectKey{Name: "test-pod-2", Namespace: testNamespaceTwo}
						Expect(informerCache.Get(context.Background(), podKey, out)).To(Succeed())

						By("verifying the retrieved pod is equal to a known pod")
						Expect(out).To(Equal(knownPod2))

						By("altering a field in the retrieved pod")
						*out.Spec.ActiveDeadlineSeconds = 4

						By("verifying the pods are no longer equal")
						Expect(out).NotTo(Equal(knownPod2))
					})
				} else {
					It("should not deep copy the object if UnsafeDisableDeepCopy is enabled", func() {
						By("getting a specific pod from the cache twice")
						podKey := client.ObjectKey{Name: "test-pod-2", Namespace: testNamespaceTwo}
						out1 := &corev1.Pod{}
						Expect(informerCache.Get(context.Background(), podKey, out1)).To(Succeed())
						out2 := &corev1.Pod{}
						Expect(informerCache.Get(context.Background(), podKey, out2)).To(Succeed())

						By("verifying the pointer fields in pod have the same addresses")
						Expect(out1).To(Equal(out2))
						Expect(reflect.ValueOf(out1.Labels).Pointer()).To(BeIdenticalTo(reflect.ValueOf(out2.Labels).Pointer()))

						By("listing pods from the cache twice")
						outList1 := &corev1.PodList{}
						Expect(informerCache.List(context.Background(), outList1, client.InNamespace(testNamespaceOne))).To(Succeed())
						outList2 := &corev1.PodList{}
						Expect(informerCache.List(context.Background(), outList2, client.InNamespace(testNamespaceOne))).To(Succeed())

						By("verifying the pointer fields in pod have the same addresses")
						Expect(outList1.Items).To(HaveLen(len(outList2.Items)))
						sort.SliceStable(outList1.Items, func(i, j int) bool { return outList1.Items[i].Name <= outList1.Items[j].Name })
						sort.SliceStable(outList2.Items, func(i, j int) bool { return outList2.Items[i].Name <= outList2.Items[j].Name })
						for i := range outList1.Items {
							a := &outList1.Items[i]
							b := &outList2.Items[i]
							Expect(a).To(Equal(b))
							Expect(reflect.ValueOf(a.Labels).Pointer()).To(BeIdenticalTo(reflect.ValueOf(b.Labels).Pointer()))
						}
					})
				}

				It("should return an error if the object is not found", func() {
					By("getting a service that does not exists")
					svc := &corev1.Service{}
					svcKey := client.ObjectKey{Namespace: testNamespaceOne, Name: "unknown"}

					By("verifying that an error is returned")
					err := informerCache.Get(context.Background(), svcKey, svc)
					Expect(err).To(HaveOccurred())
					Expect(apierrors.IsNotFound(err)).To(BeTrue())
				})

				It("should return an error if getting object in unwatched namespace", func() {
					By("getting a service that does not exists")
					svc := &corev1.Service{}
					svcKey := client.ObjectKey{Namespace: "unknown", Name: "unknown"}

					By("verifying that an error is returned")
					err := informerCache.Get(context.Background(), svcKey, svc)
					Expect(err).To(HaveOccurred())
				})

				It("should return an error when context is cancelled", func() {
					By("cancelling the context")
					informerCacheCancel()

					By("listing pods in test-namespace-1 with a cancelled context")
					listObj := &corev1.PodList{}
					err := informerCache.List(informerCacheCtx, listObj, client.InNamespace(testNamespaceOne))

					By("verifying that an error is returned")
					Expect(err).To(HaveOccurred())
					Expect(apierrors.IsTimeout(err)).To(BeTrue())
				})

				It("should set the Limit option and limit number of objects to Limit when List is called", func() {
					opts := &client.ListOptions{Limit: int64(3)}
					By("verifying that only Limit (3) number of objects are retrieved from the cache")
					listObj := &corev1.PodList{}
					Expect(informerCache.List(context.Background(), listObj, opts)).To(Succeed())
					Expect(listObj.Items).Should(HaveLen(3))
				})

				It("should return a limited result set matching the correct label", func() {
					listObj := &corev1.PodList{}
					labelOpt := client.MatchingLabels(map[string]string{"common-label": "common"})
					limitOpt := client.Limit(1)
					By("verifying that only Limit (1) number of objects are retrieved from the cache")
					Expect(informerCache.List(context.Background(), listObj, labelOpt, limitOpt)).To(Succeed())
					Expect(listObj.Items).Should(HaveLen(1))
				})

				It("should return an error if the continue list options is set", func() {
					listObj := &corev1.PodList{}
					continueOpt := client.Continue("token")
					By("verifying that an error is returned")
					err := informerCache.List(context.Background(), listObj, continueOpt)
					Expect(err).To(HaveOccurred())
				})
			})

			Context("with unstructured objects", func() {
				It("should be able to list objects that haven't been watched previously", func() {
					By("listing all services in the cluster")
					listObj := &unstructured.UnstructuredList{}
					listObj.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "ServiceList",
					})
					err := informerCache.List(context.Background(), listObj)
					Expect(err).To(Succeed())

					By("verifying that the returned list contains the Kubernetes service")
					// NB: kubernetes default service is automatically created in testenv.
					Expect(listObj.Items).NotTo(BeEmpty())
					hasKubeService := false
					for i := range listObj.Items {
						svc := &listObj.Items[i]
						if isKubeService(svc) {
							hasKubeService = true
							break
						}
					}
					Expect(hasKubeService).To(BeTrue())
				})
				It("should be able to get objects that haven't been watched previously", func() {
					By("getting the Kubernetes service")
					svc := &unstructured.Unstructured{}
					svc.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Service",
					})
					svcKey := client.ObjectKey{Namespace: "default", Name: "kubernetes"}
					Expect(informerCache.Get(context.Background(), svcKey, svc)).To(Succeed())

					By("verifying that the returned service looks reasonable")
					Expect(svc.GetName()).To(Equal("kubernetes"))
					Expect(svc.GetNamespace()).To(Equal("default"))
				})

				It("should support filtering by labels in a single namespace", func() {
					By("listing pods with a particular label")
					// NB: each pod has a "test-label": <pod-name>
					out := unstructured.UnstructuredList{}
					out.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "PodList",
					})
					err := informerCache.List(context.Background(), &out,
						client.InNamespace(testNamespaceTwo),
						client.MatchingLabels(map[string]string{"test-label": "test-pod-2"}))
					Expect(err).To(Succeed())

					By("verifying the returned pods have the correct label")
					Expect(out.Items).NotTo(BeEmpty())
					Expect(out.Items).Should(HaveLen(1))
					actual := out.Items[0]
					Expect(actual.GetLabels()["test-label"]).To(Equal("test-pod-2"))
				})

				It("should support filtering by labels from multiple namespaces", func() {
					By("creating another pod with the same label but different namespace")
					anotherPod := createPod("test-pod-2", testNamespaceOne, corev1.RestartPolicyAlways)
					defer deletePod(anotherPod)

					By("listing pods with a particular label")
					// NB: each pod has a "test-label": <pod-name>
					out := unstructured.UnstructuredList{}
					out.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "PodList",
					})
					labels := map[string]string{"test-label": "test-pod-2"}
					err := informerCache.List(context.Background(), &out, client.MatchingLabels(labels))
					Expect(err).To(Succeed())

					By("verifying multiple pods with the same label in different namespaces are returned")
					Expect(out.Items).NotTo(BeEmpty())
					Expect(out.Items).Should(HaveLen(2))
					for _, actual := range out.Items {
						Expect(actual.GetLabels()["test-label"]).To(Equal("test-pod-2"))
					}
				})

				It("should be able to list objects by namespace", func() {
					By("listing pods in test-namespace-1")
					listObj := &unstructured.UnstructuredList{}
					listObj.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "PodList",
					})
					err := informerCache.List(context.Background(), listObj, client.InNamespace(testNamespaceOne))
					Expect(err).To(Succeed())

					By("verifying that the returned pods are in test-namespace-1")
					Expect(listObj.Items).NotTo(BeEmpty())
					Expect(listObj.Items).Should(HaveLen(2))
					for _, item := range listObj.Items {
						Expect(item.GetNamespace()).To(Equal(testNamespaceOne))
					}
				})

				cacheRestrictSubTests := []struct {
					nameSuffix string
					cacheOpts  cache.Options
				}{
					{
						nameSuffix: "by using the per-gvk setting",
						cacheOpts: cache.Options{
							ByObject: map[client.Object]cache.ByObject{
								&corev1.Pod{}: {
									Namespaces: map[string]cache.Config{
										testNamespaceOne: {},
									},
								},
							},
						},
					},
					{
						nameSuffix: "by using the global DefaultNamespaces setting",
						cacheOpts: cache.Options{
							DefaultNamespaces: map[string]cache.Config{
								testNamespaceOne: {},
							},
						},
					},
				}

				for _, tc := range cacheRestrictSubTests {
					It("should be able to restrict cache to a namespace "+tc.nameSuffix, func() {
						By("creating a namespaced cache")
						namespacedCache, err := cache.New(cfg, tc.cacheOpts)
						Expect(err).NotTo(HaveOccurred())

						By("running the cache and waiting for it to sync")
						go func() {
							defer GinkgoRecover()
							Expect(namespacedCache.Start(informerCacheCtx)).To(Succeed())
						}()
						Expect(namespacedCache.WaitForCacheSync(informerCacheCtx)).To(BeTrue())

						By("listing pods in all namespaces")
						out := &unstructured.UnstructuredList{}
						out.SetGroupVersionKind(schema.GroupVersionKind{
							Group:   "",
							Version: "v1",
							Kind:    "PodList",
						})
						Expect(namespacedCache.List(context.Background(), out)).To(Succeed())

						By("verifying the returned pod is from the watched namespace")
						Expect(out.Items).NotTo(BeEmpty())
						Expect(out.Items).Should(HaveLen(2))
						for _, item := range out.Items {
							Expect(item.GetNamespace()).To(Equal(testNamespaceOne))
						}
						By("listing all nodes - should still be able to list a cluster-scoped resource")
						nodeList := &unstructured.UnstructuredList{}
						nodeList.SetGroupVersionKind(schema.GroupVersionKind{
							Group:   "",
							Version: "v1",
							Kind:    "NodeList",
						})
						Expect(namespacedCache.List(context.Background(), nodeList)).To(Succeed())

						By("verifying the node list is not empty")
						Expect(nodeList.Items).NotTo(BeEmpty())

						By("getting a node - should still be able to get a cluster-scoped resource")
						node := &unstructured.Unstructured{}
						node.SetGroupVersionKind(schema.GroupVersionKind{
							Group:   "",
							Version: "v1",
							Kind:    "Node",
						})

						By("verifying that getting the node works with an empty namespace")
						key1 := client.ObjectKey{Namespace: "", Name: testNodeOne}
						Expect(namespacedCache.Get(context.Background(), key1, node)).To(Succeed())

						By("verifying that the namespace is ignored when getting a cluster-scoped resource")
						key2 := client.ObjectKey{Namespace: "random", Name: testNodeOne}
						Expect(namespacedCache.Get(context.Background(), key2, node)).To(Succeed())
					})
				}

				if !isPodDisableDeepCopy(opts) {
					It("should deep copy the object unless told otherwise", func() {
						By("retrieving a specific pod from the cache")
						out := &unstructured.Unstructured{}
						out.SetGroupVersionKind(schema.GroupVersionKind{
							Group:   "",
							Version: "v1",
							Kind:    "Pod",
						})
						uKnownPod2 := &unstructured.Unstructured{}
						Expect(kscheme.Scheme.Convert(knownPod2, uKnownPod2, nil)).To(Succeed())

						podKey := client.ObjectKey{Name: "test-pod-2", Namespace: testNamespaceTwo}
						Expect(informerCache.Get(context.Background(), podKey, out)).To(Succeed())

						By("verifying the retrieved pod is equal to a known pod")
						Expect(out).To(Equal(uKnownPod2))

						By("altering a field in the retrieved pod")
						m, _ := out.Object["spec"].(map[string]interface{})
						m["activeDeadlineSeconds"] = 4

						By("verifying the pods are no longer equal")
						Expect(out).NotTo(Equal(knownPod2))
					})
				} else {
					It("should not deep copy the object if UnsafeDisableDeepCopy is enabled", func() {
						By("getting a specific pod from the cache twice")
						podKey := client.ObjectKey{Name: "test-pod-2", Namespace: testNamespaceTwo}
						out1 := &unstructured.Unstructured{}
						out1.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"})
						Expect(informerCache.Get(context.Background(), podKey, out1)).To(Succeed())
						out2 := &unstructured.Unstructured{}
						out2.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"})
						Expect(informerCache.Get(context.Background(), podKey, out2)).To(Succeed())

						By("verifying the pointer fields in pod have the same addresses")
						Expect(out1).To(Equal(out2))
						Expect(reflect.ValueOf(out1.Object).Pointer()).To(BeIdenticalTo(reflect.ValueOf(out2.Object).Pointer()))

						By("listing pods from the cache twice")
						outList1 := &unstructured.UnstructuredList{}
						outList1.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PodList"})
						Expect(informerCache.List(context.Background(), outList1, client.InNamespace(testNamespaceOne))).To(Succeed())
						outList2 := &unstructured.UnstructuredList{}
						outList2.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PodList"})
						Expect(informerCache.List(context.Background(), outList2, client.InNamespace(testNamespaceOne))).To(Succeed())

						By("verifying the pointer fields in pod have the same addresses")
						Expect(outList1.Items).To(HaveLen(len(outList2.Items)))
						sort.SliceStable(outList1.Items, func(i, j int) bool { return outList1.Items[i].GetName() <= outList1.Items[j].GetName() })
						sort.SliceStable(outList2.Items, func(i, j int) bool { return outList2.Items[i].GetName() <= outList2.Items[j].GetName() })
						for i := range outList1.Items {
							a := &outList1.Items[i]
							b := &outList2.Items[i]
							Expect(a).To(Equal(b))
							Expect(reflect.ValueOf(a.Object).Pointer()).To(BeIdenticalTo(reflect.ValueOf(b.Object).Pointer()))
						}
					})
				}

				It("should return an error if the object is not found", func() {
					By("getting a service that does not exists")
					svc := &unstructured.Unstructured{}
					svc.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Service",
					})
					svcKey := client.ObjectKey{Namespace: testNamespaceOne, Name: "unknown"}

					By("verifying that an error is returned")
					err := informerCache.Get(context.Background(), svcKey, svc)
					Expect(err).To(HaveOccurred())
					Expect(apierrors.IsNotFound(err)).To(BeTrue())
				})
				It("should return an error if getting object in unwatched namespace", func() {
					By("getting a service that does not exists")
					svc := &corev1.Service{}
					svcKey := client.ObjectKey{Namespace: "unknown", Name: "unknown"}

					By("verifying that an error is returned")
					err := informerCache.Get(context.Background(), svcKey, svc)
					Expect(err).To(HaveOccurred())
				})
				It("test multinamespaced cache for cluster scoped resources", func() {
					By("creating a multinamespaced cache to watch specific namespaces")
					m, err := cache.New(cfg, cache.Options{
						DefaultNamespaces: map[string]cache.Config{
							"default":        {},
							testNamespaceOne: {},
						},
					})
					Expect(err).NotTo(HaveOccurred())

					By("running the cache and waiting it for sync")
					go func() {
						defer GinkgoRecover()
						Expect(m.Start(informerCacheCtx)).To(Succeed())
					}()
					Expect(m.WaitForCacheSync(informerCacheCtx)).To(BeTrue())

					By("should be able to fetch cluster scoped resource")
					node := &corev1.Node{}

					By("verifying that getting the node works with an empty namespace")
					key1 := client.ObjectKey{Namespace: "", Name: testNodeOne}
					Expect(m.Get(context.Background(), key1, node)).To(Succeed())

					By("verifying if the cluster scoped resources are not duplicated")
					nodeList := &unstructured.UnstructuredList{}
					nodeList.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "NodeList",
					})
					Expect(m.List(context.Background(), nodeList)).To(Succeed())

					By("verifying the node list is not empty")
					Expect(nodeList.Items).NotTo(BeEmpty())
					Expect(len(nodeList.Items)).To(BeEquivalentTo(2))
				})
				It("should return an error if the continue list options is set", func() {
					podList := &unstructured.Unstructured{}
					continueOpt := client.Continue("token")
					By("verifying that an error is returned")
					err := informerCache.List(context.Background(), podList, continueOpt)
					Expect(err).To(HaveOccurred())
				})
			})
			Context("with metadata-only objects", func() {
				It("should be able to list objects that haven't been watched previously", func() {
					By("listing all services in the cluster")
					listObj := &metav1.PartialObjectMetadataList{}
					listObj.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "ServiceList",
					})
					err := informerCache.List(context.Background(), listObj)
					Expect(err).To(Succeed())

					By("verifying that the returned list contains the Kubernetes service")
					// NB: kubernetes default service is automatically created in testenv.
					Expect(listObj.Items).NotTo(BeEmpty())
					hasKubeService := false
					for i := range listObj.Items {
						svc := &listObj.Items[i]
						if isKubeService(svc) {
							hasKubeService = true
							break
						}
					}
					Expect(hasKubeService).To(BeTrue())
				})
				It("should be able to get objects that haven't been watched previously", func() {
					By("getting the Kubernetes service")
					svc := &metav1.PartialObjectMetadata{}
					svc.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Service",
					})
					svcKey := client.ObjectKey{Namespace: "default", Name: "kubernetes"}
					Expect(informerCache.Get(context.Background(), svcKey, svc)).To(Succeed())

					By("verifying that the returned service looks reasonable")
					Expect(svc.GetName()).To(Equal("kubernetes"))
					Expect(svc.GetNamespace()).To(Equal("default"))
				})

				It("should support filtering by labels in a single namespace", func() {
					By("listing pods with a particular label")
					// NB: each pod has a "test-label": <pod-name>
					out := metav1.PartialObjectMetadataList{}
					out.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "PodList",
					})
					err := informerCache.List(context.Background(), &out,
						client.InNamespace(testNamespaceTwo),
						client.MatchingLabels(map[string]string{"test-label": "test-pod-2"}))
					Expect(err).To(Succeed())

					By("verifying the returned pods have the correct label")
					Expect(out.Items).NotTo(BeEmpty())
					Expect(out.Items).Should(HaveLen(1))
					actual := out.Items[0]
					Expect(actual.GetLabels()["test-label"]).To(Equal("test-pod-2"))
				})

				It("should support filtering by labels from multiple namespaces", func() {
					By("creating another pod with the same label but different namespace")
					anotherPod := createPod("test-pod-2", testNamespaceOne, corev1.RestartPolicyAlways)
					defer deletePod(anotherPod)

					By("listing pods with a particular label")
					// NB: each pod has a "test-label": <pod-name>
					out := metav1.PartialObjectMetadataList{}
					out.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "PodList",
					})
					labels := map[string]string{"test-label": "test-pod-2"}
					err := informerCache.List(context.Background(), &out, client.MatchingLabels(labels))
					Expect(err).To(Succeed())

					By("verifying multiple pods with the same label in different namespaces are returned")
					Expect(out.Items).NotTo(BeEmpty())
					Expect(out.Items).Should(HaveLen(2))
					for _, actual := range out.Items {
						Expect(actual.GetLabels()["test-label"]).To(Equal("test-pod-2"))
					}
				})

				It("should be able to list objects by namespace", func() {
					By("listing pods in test-namespace-1")
					listObj := &metav1.PartialObjectMetadataList{}
					listObj.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "PodList",
					})
					err := informerCache.List(context.Background(), listObj, client.InNamespace(testNamespaceOne))
					Expect(err).To(Succeed())

					By("verifying that the returned pods are in test-namespace-1")
					Expect(listObj.Items).NotTo(BeEmpty())
					Expect(listObj.Items).Should(HaveLen(2))
					for _, item := range listObj.Items {
						Expect(item.Namespace).To(Equal(testNamespaceOne))
					}
				})

				It("should be able to restrict cache to a namespace", func() {
					By("creating a namespaced cache")
					namespacedCache, err := cache.New(cfg, cache.Options{DefaultNamespaces: map[string]cache.Config{testNamespaceOne: {}}})
					Expect(err).NotTo(HaveOccurred())

					By("running the cache and waiting for it to sync")
					go func() {
						defer GinkgoRecover()
						Expect(namespacedCache.Start(informerCacheCtx)).To(Succeed())
					}()
					Expect(namespacedCache.WaitForCacheSync(informerCacheCtx)).To(BeTrue())

					By("listing pods in all namespaces")
					out := &metav1.PartialObjectMetadataList{}
					out.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "PodList",
					})
					Expect(namespacedCache.List(context.Background(), out)).To(Succeed())

					By("verifying the returned pod is from the watched namespace")
					Expect(out.Items).NotTo(BeEmpty())
					Expect(out.Items).Should(HaveLen(2))
					for _, item := range out.Items {
						Expect(item.Namespace).To(Equal(testNamespaceOne))
					}
					By("listing all nodes - should still be able to list a cluster-scoped resource")
					nodeList := &metav1.PartialObjectMetadataList{}
					nodeList.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "NodeList",
					})
					Expect(namespacedCache.List(context.Background(), nodeList)).To(Succeed())

					By("verifying the node list is not empty")
					Expect(nodeList.Items).NotTo(BeEmpty())

					By("getting a node - should still be able to get a cluster-scoped resource")
					node := &metav1.PartialObjectMetadata{}
					node.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Node",
					})

					By("verifying that getting the node works with an empty namespace")
					key1 := client.ObjectKey{Namespace: "", Name: testNodeOne}
					Expect(namespacedCache.Get(context.Background(), key1, node)).To(Succeed())

					By("verifying that the namespace is ignored when getting a cluster-scoped resource")
					key2 := client.ObjectKey{Namespace: "random", Name: testNodeOne}
					Expect(namespacedCache.Get(context.Background(), key2, node)).To(Succeed())
				})

				It("should be able to restrict cache to a namespace for namespaced object and to given selectors for non namespaced object", func() {
					By("creating a namespaced cache")
					namespacedCache, err := cache.New(cfg, cache.Options{
						DefaultNamespaces: map[string]cache.Config{testNamespaceOne: {}},
						ByObject: map[client.Object]cache.ByObject{
							&corev1.Node{}: {
								Label: labels.SelectorFromSet(labels.Set{"name": testNodeTwo}),
							},
						},
					})
					Expect(err).NotTo(HaveOccurred())

					By("running the cache and waiting for it to sync")
					go func() {
						defer GinkgoRecover()
						Expect(namespacedCache.Start(informerCacheCtx)).To(Succeed())
					}()
					Expect(namespacedCache.WaitForCacheSync(informerCacheCtx)).To(BeTrue())

					By("listing pods in all namespaces")
					out := &metav1.PartialObjectMetadataList{}
					out.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "PodList",
					})
					Expect(namespacedCache.List(context.Background(), out)).To(Succeed())

					By("verifying the returned pod is from the watched namespace")
					Expect(out.Items).NotTo(BeEmpty())
					Expect(out.Items).Should(HaveLen(2))
					for _, item := range out.Items {
						Expect(item.Namespace).To(Equal(testNamespaceOne))
					}
					By("listing all nodes - should still be able to list a cluster-scoped resource")
					nodeList := &metav1.PartialObjectMetadataList{}
					nodeList.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "NodeList",
					})
					Expect(namespacedCache.List(context.Background(), nodeList)).To(Succeed())

					By("verifying the node list is not empty")
					Expect(nodeList.Items).NotTo(BeEmpty())

					By("getting a node - should still be able to get a cluster-scoped resource")
					node := &metav1.PartialObjectMetadata{}
					node.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Node",
					})

					By("verifying that getting the node works with an empty namespace")
					key1 := client.ObjectKey{Namespace: "", Name: testNodeTwo}
					Expect(namespacedCache.Get(context.Background(), key1, node)).To(Succeed())

					By("verifying that the namespace is ignored when getting a cluster-scoped resource")
					key2 := client.ObjectKey{Namespace: "random", Name: testNodeTwo}
					Expect(namespacedCache.Get(context.Background(), key2, node)).To(Succeed())

					By("verifying that an error is returned for node with not matching label")
					key3 := client.ObjectKey{Namespace: "", Name: testNodeOne}
					err = namespacedCache.Get(context.Background(), key3, node)
					Expect(err).To(HaveOccurred())
					Expect(apierrors.IsNotFound(err)).To(BeTrue())
				})

				if !isPodDisableDeepCopy(opts) {
					It("should deep copy the object unless told otherwise", func() {
						By("retrieving a specific pod from the cache")
						out := &metav1.PartialObjectMetadata{}
						out.SetGroupVersionKind(schema.GroupVersionKind{
							Group:   "",
							Version: "v1",
							Kind:    "Pod",
						})
						uKnownPod2 := &metav1.PartialObjectMetadata{}
						knownPod2.(*corev1.Pod).ObjectMeta.DeepCopyInto(&uKnownPod2.ObjectMeta)
						uKnownPod2.SetGroupVersionKind(schema.GroupVersionKind{
							Group:   "",
							Version: "v1",
							Kind:    "Pod",
						})

						podKey := client.ObjectKey{Name: "test-pod-2", Namespace: testNamespaceTwo}
						Expect(informerCache.Get(context.Background(), podKey, out)).To(Succeed())

						By("verifying the retrieved pod is equal to a known pod")
						Expect(out).To(Equal(uKnownPod2))

						By("altering a field in the retrieved pod")
						out.Labels["foo"] = "bar"

						By("verifying the pods are no longer equal")
						Expect(out).NotTo(Equal(knownPod2))
					})
				} else {
					It("should not deep copy the object if UnsafeDisableDeepCopy is enabled", func() {
						By("getting a specific pod from the cache twice")
						podKey := client.ObjectKey{Name: "test-pod-2", Namespace: testNamespaceTwo}
						out1 := &metav1.PartialObjectMetadata{}
						out1.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"})
						Expect(informerCache.Get(context.Background(), podKey, out1)).To(Succeed())
						out2 := &metav1.PartialObjectMetadata{}
						out2.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"})
						Expect(informerCache.Get(context.Background(), podKey, out2)).To(Succeed())

						By("verifying the pods have the same pointer addresses")
						By("verifying the pointer fields in pod have the same addresses")
						Expect(out1).To(Equal(out2))
						Expect(reflect.ValueOf(out1.Labels).Pointer()).To(BeIdenticalTo(reflect.ValueOf(out2.Labels).Pointer()))

						By("listing pods from the cache twice")
						outList1 := &metav1.PartialObjectMetadataList{}
						outList1.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PodList"})
						Expect(informerCache.List(context.Background(), outList1, client.InNamespace(testNamespaceOne))).To(Succeed())
						outList2 := &metav1.PartialObjectMetadataList{}
						outList2.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PodList"})
						Expect(informerCache.List(context.Background(), outList2, client.InNamespace(testNamespaceOne))).To(Succeed())

						By("verifying the pointer fields in pod have the same addresses")
						Expect(outList1.Items).To(HaveLen(len(outList2.Items)))
						sort.SliceStable(outList1.Items, func(i, j int) bool { return outList1.Items[i].Name <= outList1.Items[j].Name })
						sort.SliceStable(outList2.Items, func(i, j int) bool { return outList2.Items[i].Name <= outList2.Items[j].Name })
						for i := range outList1.Items {
							a := &outList1.Items[i]
							b := &outList2.Items[i]
							Expect(a).To(Equal(b))
							Expect(reflect.ValueOf(a.Labels).Pointer()).To(BeIdenticalTo(reflect.ValueOf(b.Labels).Pointer()))
						}
					})
				}

				It("should return an error if the object is not found", func() {
					By("getting a service that does not exists")
					svc := &metav1.PartialObjectMetadata{}
					svc.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Service",
					})
					svcKey := client.ObjectKey{Namespace: testNamespaceOne, Name: "unknown"}

					By("verifying that an error is returned")
					err := informerCache.Get(context.Background(), svcKey, svc)
					Expect(err).To(HaveOccurred())
					Expect(apierrors.IsNotFound(err)).To(BeTrue())
				})
				It("should return an error if getting object in unwatched namespace", func() {
					By("getting a service that does not exists")
					svc := &corev1.Service{}
					svcKey := client.ObjectKey{Namespace: "unknown", Name: "unknown"}

					By("verifying that an error is returned")
					err := informerCache.Get(context.Background(), svcKey, svc)
					Expect(err).To(HaveOccurred())
				})
			})
			type selectorsTestCase struct {
				options      cache.Options
				expectedPods []string
			}
			DescribeTable(" and cache with selectors", func(tc selectorsTestCase) {
				By("creating the cache")
				informer, err := cache.New(cfg, tc.options)
				Expect(err).NotTo(HaveOccurred())

				By("running the cache and waiting for it to sync")
				go func() {
					defer GinkgoRecover()
					Expect(informer.Start(informerCacheCtx)).To(Succeed())
				}()
				Expect(informer.WaitForCacheSync(informerCacheCtx)).To(BeTrue())

				By("Checking with structured")
				obtainedStructuredPodList := corev1.PodList{}
				Expect(informer.List(context.Background(), &obtainedStructuredPodList)).To(Succeed())
				Expect(obtainedStructuredPodList.Items).Should(WithTransform(func(pods []corev1.Pod) []string {
					obtainedPodNames := []string{}
					for _, pod := range pods {
						obtainedPodNames = append(obtainedPodNames, pod.Name)
					}
					return obtainedPodNames
				}, ConsistOf(tc.expectedPods)))
				for _, pod := range obtainedStructuredPodList.Items {
					Expect(informer.Get(context.Background(), client.ObjectKeyFromObject(&pod), &pod)).To(Succeed()) //nolint:gosec // We don't retain the pointer
				}

				By("Checking with unstructured")
				obtainedUnstructuredPodList := unstructured.UnstructuredList{}
				obtainedUnstructuredPodList.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "PodList",
				})
				err = informer.List(context.Background(), &obtainedUnstructuredPodList)
				Expect(err).To(Succeed())
				Expect(obtainedUnstructuredPodList.Items).Should(WithTransform(func(pods []unstructured.Unstructured) []string {
					obtainedPodNames := []string{}
					for _, pod := range pods {
						obtainedPodNames = append(obtainedPodNames, pod.GetName())
					}
					return obtainedPodNames
				}, ConsistOf(tc.expectedPods)))
				for _, pod := range obtainedUnstructuredPodList.Items {
					Expect(informer.Get(context.Background(), client.ObjectKeyFromObject(&pod), &pod)).To(Succeed()) //nolint:gosec // We don't retain the pointer
				}

				By("Checking with metadata")
				obtainedMetadataPodList := metav1.PartialObjectMetadataList{}
				obtainedMetadataPodList.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "PodList",
				})
				err = informer.List(context.Background(), &obtainedMetadataPodList)
				Expect(err).To(Succeed())
				Expect(obtainedMetadataPodList.Items).Should(WithTransform(func(pods []metav1.PartialObjectMetadata) []string {
					obtainedPodNames := []string{}
					for _, pod := range pods {
						obtainedPodNames = append(obtainedPodNames, pod.Name)
					}
					return obtainedPodNames
				}, ConsistOf(tc.expectedPods)))
				for _, pod := range obtainedMetadataPodList.Items {
					Expect(informer.Get(context.Background(), client.ObjectKeyFromObject(&pod), &pod)).To(Succeed()) //nolint:gosec // We don't retain the pointer
				}
			},
				Entry("when selectors are empty it has to inform about all the pods", selectorsTestCase{
					expectedPods: []string{"test-pod-1", "test-pod-2", "test-pod-3", "test-pod-4", "test-pod-5", "test-pod-6"},
				}),
				Entry("type-level field selector matches one pod", selectorsTestCase{
					options: cache.Options{ByObject: map[client.Object]cache.ByObject{
						&corev1.Pod{}: {Field: fields.SelectorFromSet(map[string]string{
							"metadata.name": "test-pod-2",
						})},
					}},
					expectedPods: []string{"test-pod-2"},
				}),
				Entry("global field selector matches one pod", selectorsTestCase{
					options: cache.Options{
						DefaultFieldSelector: fields.SelectorFromSet(map[string]string{
							"metadata.name": "test-pod-2",
						}),
					},
					expectedPods: []string{"test-pod-2"},
				}),
				Entry("type-level field selectors matches multiple pods", selectorsTestCase{
					options: cache.Options{ByObject: map[client.Object]cache.ByObject{
						&corev1.Pod{}: {Field: fields.SelectorFromSet(map[string]string{
							"metadata.namespace": testNamespaceTwo,
						})},
					}},
					expectedPods: []string{"test-pod-2", "test-pod-3", "test-pod-6"},
				}),
				Entry("global field selectors matches multiple pods", selectorsTestCase{
					options: cache.Options{
						DefaultFieldSelector: fields.SelectorFromSet(map[string]string{
							"metadata.namespace": testNamespaceTwo,
						}),
					},
					expectedPods: []string{"test-pod-2", "test-pod-3", "test-pod-6"},
				}),
				Entry("type-level label selector matches one pod", selectorsTestCase{
					options: cache.Options{ByObject: map[client.Object]cache.ByObject{
						&corev1.Pod{}: {Label: labels.SelectorFromSet(map[string]string{
							"test-label": "test-pod-4",
						})},
					}},
					expectedPods: []string{"test-pod-4"},
				}),
				Entry("namespaces configured, type-level label selector matches everything, overrides global selector", selectorsTestCase{
					options: cache.Options{
						DefaultNamespaces: map[string]cache.Config{testNamespaceOne: {}},
						ByObject: map[client.Object]cache.ByObject{
							&corev1.Pod{}: {Label: labels.Everything()},
						},
						DefaultLabelSelector: labels.SelectorFromSet(map[string]string{"does-not": "match-anything"}),
					},
					expectedPods: []string{"test-pod-1", "test-pod-5"},
				}),
				Entry("namespaces configured, global selector is used", selectorsTestCase{
					options: cache.Options{
						DefaultNamespaces: map[string]cache.Config{testNamespaceTwo: {}},
						ByObject: map[client.Object]cache.ByObject{
							&corev1.Pod{}: {},
						},
						DefaultLabelSelector: labels.SelectorFromSet(map[string]string{"common-label": "common"}),
					},
					expectedPods: []string{"test-pod-3"},
				}),
				Entry("global label selector matches one pod", selectorsTestCase{
					options: cache.Options{
						DefaultLabelSelector: labels.SelectorFromSet(map[string]string{
							"test-label": "test-pod-4",
						}),
					},
					expectedPods: []string{"test-pod-4"},
				}),
				Entry("type-level label selector matches multiple pods", selectorsTestCase{
					options: cache.Options{ByObject: map[client.Object]cache.ByObject{
						&corev1.Pod{}: {Label: labels.SelectorFromSet(map[string]string{
							"common-label": "common",
						})},
					}},
					expectedPods: []string{"test-pod-3", "test-pod-4"},
				}),
				Entry("global label selector matches multiple pods", selectorsTestCase{
					options: cache.Options{
						DefaultLabelSelector: labels.SelectorFromSet(map[string]string{
							"common-label": "common",
						}),
					},
					expectedPods: []string{"test-pod-3", "test-pod-4"},
				}),
				Entry("type-level label and field selector, matches one pod", selectorsTestCase{
					options: cache.Options{ByObject: map[client.Object]cache.ByObject{
						&corev1.Pod{}: {
							Label: labels.SelectorFromSet(map[string]string{"common-label": "common"}),
							Field: fields.SelectorFromSet(map[string]string{"metadata.namespace": testNamespaceTwo}),
						},
					}},
					expectedPods: []string{"test-pod-3"},
				}),
				Entry("global label and field selector, matches one pod", selectorsTestCase{
					options: cache.Options{
						DefaultLabelSelector: labels.SelectorFromSet(map[string]string{"common-label": "common"}),
						DefaultFieldSelector: fields.SelectorFromSet(map[string]string{"metadata.namespace": testNamespaceTwo}),
					},
					expectedPods: []string{"test-pod-3"},
				}),
				Entry("type-level label selector does not match, no results", selectorsTestCase{
					options: cache.Options{ByObject: map[client.Object]cache.ByObject{
						&corev1.Pod{}: {Label: labels.SelectorFromSet(map[string]string{
							"new-label": "new",
						})},
					}},
					expectedPods: []string{},
				}),
				Entry("global label selector does not match, no results", selectorsTestCase{
					options: cache.Options{
						DefaultLabelSelector: labels.SelectorFromSet(map[string]string{
							"new-label": "new",
						}),
					},
					expectedPods: []string{},
				}),
				Entry("type-level field selector does not match, no results", selectorsTestCase{
					options: cache.Options{ByObject: map[client.Object]cache.ByObject{
						&corev1.Pod{}: {Field: fields.SelectorFromSet(map[string]string{
							"metadata.namespace": "new",
						})},
					}},
					expectedPods: []string{},
				}),
				Entry("global field selector does not match, no results", selectorsTestCase{
					options: cache.Options{
						DefaultFieldSelector: fields.SelectorFromSet(map[string]string{
							"metadata.namespace": "new",
						}),
					},
					expectedPods: []string{},
				}),
				Entry("type-level field selector on namespace matches one pod", selectorsTestCase{
					options: cache.Options{ByObject: map[client.Object]cache.ByObject{
						&corev1.Pod{}: {Namespaces: map[string]cache.Config{
							testNamespaceTwo: {
								FieldSelector: fields.SelectorFromSet(map[string]string{
									"metadata.name": "test-pod-2",
								}),
							},
						}},
					}},
					expectedPods: []string{"test-pod-2"},
				}),
				Entry("type-level field selector on namespace doesn't match", selectorsTestCase{
					options: cache.Options{ByObject: map[client.Object]cache.ByObject{
						&corev1.Pod{}: {Namespaces: map[string]cache.Config{
							testNamespaceTwo: {
								FieldSelector: fields.SelectorFromSet(map[string]string{
									"metadata.name": "test-pod-doesn-exist",
								}),
							},
						}},
					}},
					expectedPods: []string{},
				}),
				Entry("global field selector on namespace matches one pod", selectorsTestCase{
					options: cache.Options{
						DefaultNamespaces: map[string]cache.Config{
							testNamespaceTwo: {
								FieldSelector: fields.SelectorFromSet(map[string]string{
									"metadata.name": "test-pod-2",
								}),
							},
						},
					},
					expectedPods: []string{"test-pod-2"},
				}),
				Entry("global field selector on namespace doesn't match", selectorsTestCase{
					options: cache.Options{
						DefaultNamespaces: map[string]cache.Config{
							testNamespaceTwo: {
								FieldSelector: fields.SelectorFromSet(map[string]string{
									"metadata.name": "test-pod-doesn-exist",
								}),
							},
						},
					},
					expectedPods: []string{},
				}),
				Entry("type-level label selector on namespace matches one pod", selectorsTestCase{
					options: cache.Options{ByObject: map[client.Object]cache.ByObject{
						&corev1.Pod{}: {Namespaces: map[string]cache.Config{
							testNamespaceTwo: {
								LabelSelector: labels.SelectorFromSet(map[string]string{
									"test-label": "test-pod-2",
								}),
							},
						}},
					}},
					expectedPods: []string{"test-pod-2"},
				}),
				Entry("type-level label selector on namespace doesn't match", selectorsTestCase{
					options: cache.Options{ByObject: map[client.Object]cache.ByObject{
						&corev1.Pod{}: {Namespaces: map[string]cache.Config{
							testNamespaceTwo: {
								LabelSelector: labels.SelectorFromSet(map[string]string{
									"test-label": "test-pod-doesn-exist",
								}),
							},
						}},
					}},
					expectedPods: []string{},
				}),
				Entry("global label selector on namespace matches one pod", selectorsTestCase{
					options: cache.Options{
						DefaultNamespaces: map[string]cache.Config{
							testNamespaceTwo: {
								LabelSelector: labels.SelectorFromSet(map[string]string{
									"test-label": "test-pod-2",
								}),
							},
						},
					},
					expectedPods: []string{"test-pod-2"},
				}),
				Entry("global label selector on namespace doesn't match", selectorsTestCase{
					options: cache.Options{
						DefaultNamespaces: map[string]cache.Config{
							testNamespaceTwo: {
								LabelSelector: labels.SelectorFromSet(map[string]string{
									"test-label": "test-pod-doesn-exist",
								}),
							},
						},
					},
					expectedPods: []string{},
				}),
				Entry("Only NamespaceAll in DefaultNamespaces returns all pods", selectorsTestCase{
					options: cache.Options{
						DefaultNamespaces: map[string]cache.Config{
							metav1.NamespaceAll: {},
						},
					},
					expectedPods: []string{"test-pod-1", "test-pod-2", "test-pod-3", "test-pod-4", "test-pod-5", "test-pod-6"},
				}),
				Entry("Only NamespaceAll in ByObject.Namespaces returns all pods", selectorsTestCase{
					options: cache.Options{
						ByObject: map[client.Object]cache.ByObject{
							&corev1.Pod{}: {
								Namespaces: map[string]cache.Config{
									metav1.NamespaceAll: {},
								},
							},
						},
					},
					expectedPods: []string{"test-pod-1", "test-pod-2", "test-pod-3", "test-pod-4", "test-pod-5", "test-pod-6"},
				}),
				Entry("NamespaceAll in DefaultNamespaces creates a cache for all Namespaces that are not in DefaultNamespaces", selectorsTestCase{
					options: cache.Options{
						DefaultNamespaces: map[string]cache.Config{
							metav1.NamespaceAll: {},
							testNamespaceOne: {
								// labels.Nothing when serialized matches everything, so we have to construct our own "match nothing" selector
								LabelSelector: labels.SelectorFromSet(labels.Set{"no-present": "not-present"})},
						},
					},
					// All pods that are not in NamespaceOne
					expectedPods: []string{"test-pod-2", "test-pod-3", "test-pod-4", "test-pod-6"},
				}),
				Entry("NamespaceAll in ByObject.Namespaces creates a cache for all Namespaces that are not in ByObject.Namespaces", selectorsTestCase{
					options: cache.Options{
						ByObject: map[client.Object]cache.ByObject{
							&corev1.Pod{}: {
								Namespaces: map[string]cache.Config{
									metav1.NamespaceAll: {},
									testNamespaceOne: {
										// labels.Nothing when serialized matches everything, so we have to construct our own "match nothing" selector
										LabelSelector: labels.SelectorFromSet(labels.Set{"no-present": "not-present"})},
								},
							},
						},
					},
					// All pods that are not in NamespaceOne
					expectedPods: []string{"test-pod-2", "test-pod-3", "test-pod-4", "test-pod-6"},
				}),
			)
		})
		Describe("as an Informer", func() {
			It("should error when starting the cache a second time", func() {
				err := informerCache.Start(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Informer already started"))
			})

			Context("with structured objects", func() {
				It("should be able to get informer for the object", func() {
					By("getting a shared index informer for a pod")
					pod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "informer-obj",
							Namespace: "default",
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx",
								},
							},
						},
					}
					sii, err := informerCache.GetInformer(context.TODO(), pod)
					Expect(err).NotTo(HaveOccurred())
					Expect(sii).NotTo(BeNil())
					Expect(sii.HasSynced()).To(BeTrue())

					By("adding an event handler listening for object creation which sends the object to a channel")
					out := make(chan interface{})
					addFunc := func(obj interface{}) {
						out <- obj
					}
					_, _ = sii.AddEventHandler(kcache.ResourceEventHandlerFuncs{AddFunc: addFunc})

					By("adding an object")
					cl, err := client.New(cfg, client.Options{})
					Expect(err).NotTo(HaveOccurred())
					Expect(cl.Create(context.Background(), pod)).To(Succeed())
					defer deletePod(pod)

					By("verifying the object is received on the channel")
					Eventually(out).Should(Receive(Equal(pod)))
				})
				It("should be able to stop and restart informers", func() {
					By("getting a shared index informer for a pod")
					pod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "informer-obj",
							Namespace: "default",
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx",
								},
							},
						},
					}
					sii, err := informerCache.GetInformer(context.TODO(), pod)
					Expect(err).NotTo(HaveOccurred())
					Expect(sii).NotTo(BeNil())
					Expect(sii.HasSynced()).To(BeTrue())

					By("removing the existing informer")
					Expect(informerCache.RemoveInformer(context.TODO(), pod)).To(Succeed())
					Eventually(sii.IsStopped).WithTimeout(5 * time.Second).Should(BeTrue())

					By("recreating the informer")

					sii2, err := informerCache.GetInformer(context.TODO(), pod)
					Expect(err).NotTo(HaveOccurred())
					Expect(sii2).NotTo(BeNil())
					Expect(sii2.HasSynced()).To(BeTrue())

					By("validating the two informers are in different states")
					Expect(sii.IsStopped()).To(BeTrue())
					Expect(sii2.IsStopped()).To(BeFalse())
				})
				It("should be able to get an informer by group/version/kind", func() {
					By("getting an shared index informer for gvk = core/v1/pod")
					gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}
					sii, err := informerCache.GetInformerForKind(context.TODO(), gvk)
					Expect(err).NotTo(HaveOccurred())
					Expect(sii).NotTo(BeNil())
					Expect(sii.HasSynced()).To(BeTrue())

					By("adding an event handler listening for object creation which sends the object to a channel")
					out := make(chan interface{})
					addFunc := func(obj interface{}) {
						out <- obj
					}
					_, _ = sii.AddEventHandler(kcache.ResourceEventHandlerFuncs{AddFunc: addFunc})

					By("adding an object")
					cl, err := client.New(cfg, client.Options{})
					Expect(err).NotTo(HaveOccurred())
					pod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "informer-gvk",
							Namespace: "default",
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx",
								},
							},
						},
					}
					Expect(cl.Create(context.Background(), pod)).To(Succeed())
					defer deletePod(pod)

					By("verifying the object is received on the channel")
					Eventually(out).Should(Receive(Equal(pod)))
				})
				It("should be able to index an object field then retrieve objects by that field", func() {
					By("creating the cache")
					informer, err := cache.New(cfg, cache.Options{})
					Expect(err).NotTo(HaveOccurred())

					By("indexing the restartPolicy field of the Pod object before starting")
					pod := &corev1.Pod{}
					indexFunc := func(obj client.Object) []string {
						return []string{string(obj.(*corev1.Pod).Spec.RestartPolicy)}
					}
					Expect(informer.IndexField(context.TODO(), pod, "spec.restartPolicy", indexFunc)).To(Succeed())

					By("running the cache and waiting for it to sync")
					go func() {
						defer GinkgoRecover()
						Expect(informer.Start(informerCacheCtx)).To(Succeed())
					}()
					Expect(informer.WaitForCacheSync(informerCacheCtx)).To(BeTrue())

					By("listing Pods with restartPolicyOnFailure")
					listObj := &corev1.PodList{}
					Expect(informer.List(context.Background(), listObj,
						client.MatchingFields{"spec.restartPolicy": "OnFailure"})).To(Succeed())
					By("verifying that the returned pods have correct restart policy")
					Expect(listObj.Items).NotTo(BeEmpty())
					Expect(listObj.Items).Should(HaveLen(1))
					actual := listObj.Items[0]
					Expect(actual.Name).To(Equal("test-pod-3"))
				})

				It("should allow for get informer to be cancelled", func() {
					By("creating a context and cancelling it")
					informerCacheCancel()

					By("getting a shared index informer for a pod with a cancelled context")
					pod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "informer-obj",
							Namespace: "default",
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx",
								},
							},
						},
					}
					sii, err := informerCache.GetInformer(informerCacheCtx, pod)
					Expect(err).To(HaveOccurred())
					Expect(sii).To(BeNil())
					Expect(apierrors.IsTimeout(err)).To(BeTrue())
				})

				It("should allow getting an informer by group/version/kind to be cancelled", func() {
					By("creating a context and cancelling it")
					informerCacheCancel()

					By("getting an shared index informer for gvk = core/v1/pod with a cancelled context")
					gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}
					sii, err := informerCache.GetInformerForKind(informerCacheCtx, gvk)
					Expect(err).To(HaveOccurred())
					Expect(sii).To(BeNil())
					Expect(apierrors.IsTimeout(err)).To(BeTrue())
				})

				It("should be able not to change indexer values after indexing cluster-scope objects", func() {
					By("creating the cache")
					informer, err := cache.New(cfg, cache.Options{})
					Expect(err).NotTo(HaveOccurred())

					By("indexing the Namespace objects with fixed values before starting")
					ns := &corev1.Namespace{}
					indexerValues := []string{"a", "b", "c"}
					fieldName := "fixedValues"
					indexFunc := func(obj client.Object) []string {
						return indexerValues
					}
					Expect(informer.IndexField(context.TODO(), ns, fieldName, indexFunc)).To(Succeed())

					By("running the cache and waiting for it to sync")
					go func() {
						defer GinkgoRecover()
						Expect(informer.Start(informerCacheCtx)).To(Succeed())
					}()
					Expect(informer.WaitForCacheSync(informerCacheCtx)).To(BeTrue())

					By("listing Namespaces with fixed indexer")
					listObj := &corev1.NamespaceList{}
					Expect(informer.List(context.Background(), listObj,
						client.MatchingFields{fieldName: "a"})).To(Succeed())
					Expect(listObj.Items).NotTo(BeZero())

					By("verifying the indexing does not change fixed returned values")
					Expect(indexerValues).Should(HaveLen(3))
					Expect(indexerValues[0]).To(Equal("a"))
					Expect(indexerValues[1]).To(Equal("b"))
					Expect(indexerValues[2]).To(Equal("c"))
				})

				It("should be able to matching fields with multiple indexes", func() {
					By("creating the cache")
					informer, err := cache.New(cfg, cache.Options{})
					Expect(err).NotTo(HaveOccurred())

					pod := &corev1.Pod{}
					By("indexing pods with label before starting")
					fieldName1 := "indexByLabel"
					indexFunc1 := func(obj client.Object) []string {
						return []string{obj.(*corev1.Pod).Labels["common-label"]}
					}
					Expect(informer.IndexField(context.TODO(), pod, fieldName1, indexFunc1)).To(Succeed())
					By("indexing pods with restart policy before starting")
					fieldName2 := "indexByPolicy"
					indexFunc2 := func(obj client.Object) []string {
						return []string{string(obj.(*corev1.Pod).Spec.RestartPolicy)}
					}
					Expect(informer.IndexField(context.TODO(), pod, fieldName2, indexFunc2)).To(Succeed())

					By("running the cache and waiting for it to sync")
					go func() {
						defer GinkgoRecover()
						Expect(informer.Start(informerCacheCtx)).To(Succeed())
					}()
					Expect(informer.WaitForCacheSync(informerCacheCtx)).To(BeTrue())

					By("listing pods with label index")
					listObj := &corev1.PodList{}
					Expect(informer.List(context.Background(), listObj,
						client.MatchingFields{fieldName1: "common"})).To(Succeed())
					Expect(listObj.Items).To(HaveLen(2))

					By("listing pods with restart policy index")
					listObj = &corev1.PodList{}
					Expect(informer.List(context.Background(), listObj,
						client.MatchingFields{fieldName2: string(corev1.RestartPolicyNever)})).To(Succeed())
					Expect(listObj.Items).To(HaveLen(3))

					By("listing pods with both fixed indexers 1 and 2")
					listObj = &corev1.PodList{}
					Expect(informer.List(context.Background(), listObj,
						client.MatchingFields{fieldName1: "common", fieldName2: string(corev1.RestartPolicyNever)})).To(Succeed())
					Expect(listObj.Items).To(HaveLen(1))
				})
			})
			Context("with unstructured objects", func() {
				It("should be able to get informer for the object", func() {
					By("getting a shared index informer for a pod")

					pod := &unstructured.Unstructured{
						Object: map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []map[string]interface{}{
									{
										"name":  "nginx",
										"image": "nginx",
									},
								},
							},
						},
					}
					pod.SetName("informer-obj2")
					pod.SetNamespace("default")
					pod.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Pod",
					})
					sii, err := informerCache.GetInformer(context.TODO(), pod)
					Expect(err).NotTo(HaveOccurred())
					Expect(sii).NotTo(BeNil())
					Expect(sii.HasSynced()).To(BeTrue())

					By("adding an event handler listening for object creation which sends the object to a channel")
					out := make(chan interface{})
					addFunc := func(obj interface{}) {
						out <- obj
					}
					_, _ = sii.AddEventHandler(kcache.ResourceEventHandlerFuncs{AddFunc: addFunc})

					By("adding an object")
					cl, err := client.New(cfg, client.Options{})
					Expect(err).NotTo(HaveOccurred())
					Expect(cl.Create(context.Background(), pod)).To(Succeed())
					defer deletePod(pod)

					By("verifying the object is received on the channel")
					Eventually(out).Should(Receive(Equal(pod)))
				})

				It("should be able to stop and restart informers", func() {
					By("getting a shared index informer for a pod")
					pod := &unstructured.Unstructured{
						Object: map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []map[string]interface{}{
									{
										"name":  "nginx",
										"image": "nginx",
									},
								},
							},
						},
					}
					pod.SetName("informer-obj2")
					pod.SetNamespace("default")
					pod.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Pod",
					})
					sii, err := informerCache.GetInformer(context.TODO(), pod)
					Expect(err).NotTo(HaveOccurred())
					Expect(sii).NotTo(BeNil())
					Expect(sii.HasSynced()).To(BeTrue())

					By("removing the existing informer")
					Expect(informerCache.RemoveInformer(context.TODO(), pod)).To(Succeed())
					Eventually(sii.IsStopped).WithTimeout(5 * time.Second).Should(BeTrue())

					By("recreating the informer")

					sii2, err := informerCache.GetInformer(context.TODO(), pod)
					Expect(err).NotTo(HaveOccurred())
					Expect(sii2).NotTo(BeNil())
					Expect(sii2.HasSynced()).To(BeTrue())

					By("validating the two informers are in different states")
					Expect(sii.IsStopped()).To(BeTrue())
					Expect(sii2.IsStopped()).To(BeFalse())
				})

				It("should be able to index an object field then retrieve objects by that field", func() {
					By("creating the cache")
					informer, err := cache.New(cfg, cache.Options{})
					Expect(err).NotTo(HaveOccurred())

					By("indexing the restartPolicy field of the Pod object before starting")
					pod := &unstructured.Unstructured{}
					pod.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Pod",
					})
					indexFunc := func(obj client.Object) []string {
						s, ok := obj.(*unstructured.Unstructured).Object["spec"]
						if !ok {
							return []string{}
						}
						m, ok := s.(map[string]interface{})
						if !ok {
							return []string{}
						}
						return []string{fmt.Sprintf("%v", m["restartPolicy"])}
					}
					Expect(informer.IndexField(context.TODO(), pod, "spec.restartPolicy", indexFunc)).To(Succeed())

					By("running the cache and waiting for it to sync")
					go func() {
						defer GinkgoRecover()
						Expect(informer.Start(informerCacheCtx)).To(Succeed())
					}()
					Expect(informer.WaitForCacheSync(informerCacheCtx)).To(BeTrue())

					By("listing Pods with restartPolicyOnFailure")
					listObj := &unstructured.UnstructuredList{}
					listObj.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "PodList",
					})
					err = informer.List(context.Background(), listObj,
						client.MatchingFields{"spec.restartPolicy": "OnFailure"})
					Expect(err).To(Succeed())

					By("verifying that the returned pods have correct restart policy")
					Expect(listObj.Items).NotTo(BeEmpty())
					Expect(listObj.Items).Should(HaveLen(1))
					actual := listObj.Items[0]
					Expect(actual.GetName()).To(Equal("test-pod-3"))
				})

				It("should allow for get informer to be cancelled", func() {
					By("cancelling the context")
					informerCacheCancel()

					By("getting a shared index informer for a pod with a cancelled context")
					pod := &unstructured.Unstructured{}
					pod.SetName("informer-obj2")
					pod.SetNamespace("default")
					pod.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Pod",
					})
					sii, err := informerCache.GetInformer(informerCacheCtx, pod)
					Expect(err).To(HaveOccurred())
					Expect(sii).To(BeNil())
					Expect(apierrors.IsTimeout(err)).To(BeTrue())
				})
			})
			Context("with metadata-only objects", func() {
				It("should be able to get informer for the object", func() {
					By("getting a shared index informer for a pod")

					pod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "informer-obj",
							Namespace: "default",
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx",
								},
							},
						},
					}

					podMeta := &metav1.PartialObjectMetadata{}
					pod.ObjectMeta.DeepCopyInto(&podMeta.ObjectMeta)
					podMeta.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Pod",
					})

					sii, err := informerCache.GetInformer(context.TODO(), podMeta)
					Expect(err).NotTo(HaveOccurred())
					Expect(sii).NotTo(BeNil())
					Expect(sii.HasSynced()).To(BeTrue())

					By("adding an event handler listening for object creation which sends the object to a channel")
					out := make(chan interface{})
					addFunc := func(obj interface{}) {
						out <- obj
					}
					_, _ = sii.AddEventHandler(kcache.ResourceEventHandlerFuncs{AddFunc: addFunc})

					By("adding an object")
					cl, err := client.New(cfg, client.Options{})
					Expect(err).NotTo(HaveOccurred())
					Expect(cl.Create(context.Background(), pod)).To(Succeed())
					defer deletePod(pod)
					// re-copy the result in so that we can match on it properly
					pod.ObjectMeta.DeepCopyInto(&podMeta.ObjectMeta)

					By("verifying the object's metadata is received on the channel")
					Eventually(out).Should(Receive(Equal(podMeta)))
				})

				It("should be able to index an object field then retrieve objects by that field", func() {
					By("creating the cache")
					informer, err := cache.New(cfg, cache.Options{})
					Expect(err).NotTo(HaveOccurred())

					By("indexing the restartPolicy field of the Pod object before starting")
					pod := &metav1.PartialObjectMetadata{}
					pod.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Pod",
					})
					indexFunc := func(obj client.Object) []string {
						metadata := obj.(*metav1.PartialObjectMetadata)
						return []string{metadata.Labels["test-label"]}
					}
					Expect(informer.IndexField(context.TODO(), pod, "metadata.labels.test-label", indexFunc)).To(Succeed())

					By("running the cache and waiting for it to sync")
					go func() {
						defer GinkgoRecover()
						Expect(informer.Start(informerCacheCtx)).To(Succeed())
					}()
					Expect(informer.WaitForCacheSync(informerCacheCtx)).To(BeTrue())

					By("listing Pods with restartPolicyOnFailure")
					listObj := &metav1.PartialObjectMetadataList{}
					gvk := schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "PodList",
					}
					listObj.SetGroupVersionKind(gvk)
					err = informer.List(context.Background(), listObj,
						client.MatchingFields{"metadata.labels.test-label": "test-pod-3"})
					Expect(err).To(Succeed())

					By("verifying that the GVK has been preserved for the list object")
					Expect(listObj.GroupVersionKind()).To(Equal(gvk))

					By("verifying that the returned pods have correct restart policy")
					Expect(listObj.Items).NotTo(BeEmpty())
					Expect(listObj.Items).Should(HaveLen(1))
					actual := listObj.Items[0]
					Expect(actual.GetName()).To(Equal("test-pod-3"))

					By("verifying that the GVK has been preserved for the item in the list")
					Expect(actual.GroupVersionKind()).To(Equal(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Pod",
					}))
				})

				It("should allow for get informer to be cancelled", func() {
					By("creating a context and cancelling it")
					ctx, cancel := context.WithCancel(context.Background())
					cancel()

					By("getting a shared index informer for a pod with a cancelled context")
					pod := &metav1.PartialObjectMetadata{}
					pod.SetName("informer-obj2")
					pod.SetNamespace("default")
					pod.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Pod",
					})
					sii, err := informerCache.GetInformer(ctx, pod)
					Expect(err).To(HaveOccurred())
					Expect(sii).To(BeNil())
					Expect(apierrors.IsTimeout(err)).To(BeTrue())
				})
			})
		})
		Describe("use UnsafeDisableDeepCopy list options", func() {
			It("should be able to change object in informer cache", func() {
				By("listing pods")
				out := corev1.PodList{}
				Expect(informerCache.List(context.Background(), &out, client.UnsafeDisableDeepCopy)).To(Succeed())
				for _, item := range out.Items {
					if strings.Compare(item.Name, "test-pod-3") == 0 { // test-pod-3 has labels
						item.Labels["UnsafeDisableDeepCopy"] = "true"
						break
					}
				}

				By("verifying that the returned pods were changed")
				out2 := corev1.PodList{}
				Expect(informerCache.List(context.Background(), &out, client.UnsafeDisableDeepCopy)).To(Succeed())
				for _, item := range out2.Items {
					if strings.Compare(item.Name, "test-pod-3") == 0 {
						Expect(item.Labels["UnsafeDisableDeepCopy"]).To(Equal("true"))
						break
					}
				}
			})
		})
	})
}

var _ = Describe("TransformStripManagedFields", func() {
	It("should strip managed fields from an object", func() {
		obj := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			ManagedFields: []metav1.ManagedFieldsEntry{{
				Manager: "foo",
			}},
		}}
		transformed, err := cache.TransformStripManagedFields()(obj)
		Expect(err).NotTo(HaveOccurred())
		Expect(transformed).To(Equal(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{}}))
	})

	It("should not trip over an unexpected object", func() {
		transformed, err := cache.TransformStripManagedFields()("foo")
		Expect(err).NotTo(HaveOccurred())
		Expect(transformed).To(Equal("foo"))
	})
})

// ensureNamespace installs namespace of a given name if not exists.
func ensureNamespace(namespace string, client client.Client) error {
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
	}
	err := client.Create(context.TODO(), &ns)
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func ensureNode(name string, client client.Client) error {
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"name": name},
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
	}
	err := client.Create(context.TODO(), &node)
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

//nolint:interfacer
func isKubeService(svc metav1.Object) bool {
	// grumble grumble linters grumble grumble
	return svc.GetNamespace() == "default" && svc.GetName() == "kubernetes"
}

func isPodDisableDeepCopy(opts cache.Options) bool {
	if opts.ByObject[&corev1.Pod{}].UnsafeDisableDeepCopy != nil {
		return *opts.ByObject[&corev1.Pod{}].UnsafeDisableDeepCopy
	}
	if opts.DefaultUnsafeDisableDeepCopy != nil {
		return *opts.DefaultUnsafeDisableDeepCopy
	}
	return false
}
