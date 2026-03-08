package client_test

import (
	"context"
	"fmt"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	kscheme "k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ConsistentClient", func() {
	var (
		cl      client.Client
		ctx     context.Context
		cancel  context.CancelFunc
		counter uint64
	)

	BeforeEach(func(specCtx context.Context) {
		ctx, cancel = context.WithCancel(specCtx)

		c, err := cache.New(cfg, cache.Options{Scheme: kscheme.Scheme})
		Expect(err).NotTo(HaveOccurred())

		// Set up informers for types used through the consistent client.
		_, err = c.GetInformer(ctx, &corev1.Namespace{})
		Expect(err).NotTo(HaveOccurred())
		_, err = c.GetInformer(ctx, &corev1.ConfigMap{})
		Expect(err).NotTo(HaveOccurred())

		go func() {
			defer GinkgoRecover()
			Expect(c.Start(ctx)).To(Succeed())
		}()
		Expect(c.WaitForCacheSync(ctx)).To(BeTrue())

		cl, err = client.New(cfg, client.Options{
			Scheme: kscheme.Scheme,
			Cache: &client.CacheOptions{
				Reader:                             c,
				ReadYourOwnWriteConsistencyEnabled: true,
			},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cancel()
	})

	newConfigMap := func(ns string) *corev1.ConfigMap {
		n := atomic.AddUint64(&counter, 1)
		return &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("consistency-test-%d", n),
				Namespace: ns,
			},
			Data: map[string]string{"key": "value"},
		}
	}

	type writeResult struct {
		name    string
		deleted bool
		data    map[string]string
	}

	DescribeTable("write then read",
		func(ctx context.Context, write func(ctx context.Context, cl client.Client, cm *corev1.ConfigMap) (writeResult, error)) {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("consistency-wtr-%d", atomic.AddUint64(&counter, 1))}}
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
					err := cl.Get(ctx, client.ObjectKeyFromObject(cm), &corev1.ConfigMap{})
					Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected NotFound after delete, got: %v", err)
				} else {
					got := &corev1.ConfigMap{}
					Expect(cl.Get(ctx, client.ObjectKeyFromObject(cm), got)).To(Succeed())
					Expect(got.Name).To(Equal(result.name))
					Expect(got.Data).To(Equal(result.data))
				}
			}()

			go func() {
				defer GinkgoRecover()
				defer func() { done <- struct{}{} }()

				list := &corev1.ConfigMapList{}
				Expect(cl.List(ctx, list, client.InNamespace(ns.Name))).To(Succeed())

				if result.deleted {
					Expect(list.Items).To(BeEmpty(), "list should be empty after delete")
				} else {
					Expect(list.Items).To(HaveLen(1), "list should contain exactly one ConfigMap")
					Expect(list.Items[0].Name).To(Equal(result.name))
					Expect(list.Items[0].Data).To(Equal(result.data))
				}
			}()

			<-done
			<-done
		},

		Entry("create", func(ctx context.Context, cl client.Client, cm *corev1.ConfigMap) (writeResult, error) {
			return writeResult{
				name: cm.Name,
				data: cm.Data,
			}, nil // already created in the setup
		}),

		Entry("update", func(ctx context.Context, cl client.Client, cm *corev1.ConfigMap) (writeResult, error) {
			got := &corev1.ConfigMap{}
			if err := cl.Get(ctx, client.ObjectKeyFromObject(cm), got); err != nil {
				return writeResult{}, err
			}
			got.Data["key"] = "updated"
			if err := cl.Update(ctx, got); err != nil {
				return writeResult{}, err
			}
			return writeResult{
				name: cm.Name,
				data: map[string]string{"key": "updated"},
			}, nil
		}),

		Entry("patch", func(ctx context.Context, cl client.Client, cm *corev1.ConfigMap) (writeResult, error) {
			got := &corev1.ConfigMap{}
			if err := cl.Get(ctx, client.ObjectKeyFromObject(cm), got); err != nil {
				return writeResult{}, err
			}
			patch := client.MergeFrom(got.DeepCopy())
			got.Data["patched"] = "yes"
			if err := cl.Patch(ctx, got, patch); err != nil {
				return writeResult{}, err
			}
			return writeResult{
				name: cm.Name,
				data: map[string]string{"key": "value", "patched": "yes"},
			}, nil
		}),

		Entry("apply", func(ctx context.Context, cl client.Client, cm *corev1.ConfigMap) (writeResult, error) {
			ac := corev1ac.ConfigMap(cm.Name, cm.Namespace).
				WithData(map[string]string{"key": "applied"})
			if err := cl.Apply(ctx, ac, client.FieldOwner("consistency-test"), client.ForceOwnership); err != nil {
				return writeResult{}, err
			}
			return writeResult{
				name: cm.Name,
				data: map[string]string{"key": "applied"},
			}, nil
		}),

		Entry("delete", func(ctx context.Context, cl client.Client, cm *corev1.ConfigMap) (writeResult, error) {
			if err := cl.Delete(ctx, cm); err != nil {
				return writeResult{}, err
			}
			return writeResult{
				name:    cm.Name,
				deleted: true,
			}, nil
		}),
	)

	Describe("Multiple sequential writes then Get", func() {
		It("should observe the final state after multiple updates", func() {
			cm := newConfigMap("default")
			Expect(cl.Create(ctx, cm)).To(Succeed())
			DeferCleanup(func(ctx context.Context) {
				Expect(client.IgnoreNotFound(cl.Delete(ctx, cm))).To(Succeed())
			})

			for i := range 5 {
				got := &corev1.ConfigMap{}
				Expect(cl.Get(ctx, client.ObjectKeyFromObject(cm), got)).To(Succeed())
				got.Data["key"] = fmt.Sprintf("iteration-%d", i)
				Expect(cl.Update(ctx, got)).To(Succeed())
			}

			final := &corev1.ConfigMap{}
			Expect(cl.Get(ctx, client.ObjectKeyFromObject(cm), final)).To(Succeed())
			Expect(final.Data["key"]).To(Equal("iteration-4"))
		})
	})

	Describe("Create with namespace-scoped object", func() {
		It("should work across different namespaces", func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("consistency-ns-%d", atomic.AddUint64(&counter, 1))}}
			Expect(cl.Create(ctx, ns)).To(Succeed())
			DeferCleanup(func(ctx context.Context) {
				Expect(client.IgnoreNotFound(cl.Delete(ctx, ns))).To(Succeed())
			})

			cm := newConfigMap(ns.Name)
			Expect(cl.Create(ctx, cm)).To(Succeed())
			DeferCleanup(func(ctx context.Context) {
				Expect(client.IgnoreNotFound(cl.Delete(ctx, cm))).To(Succeed())
			})

			got := &corev1.ConfigMap{}
			Expect(cl.Get(ctx, client.ObjectKeyFromObject(cm), got)).To(Succeed())
			Expect(got.Name).To(Equal(cm.Name))
			Expect(got.Namespace).To(Equal(ns.Name))
		})
	})

	Describe("Delete object with finalizer then Get", func() {
		It("should observe the updated object with deletion timestamp after delete", func() {
			cm := newConfigMap("default")
			cm.Finalizers = []string{"test.io/hold"}
			Expect(cl.Create(ctx, cm)).To(Succeed())
			DeferCleanup(func(ctx context.Context) {
				got := &corev1.ConfigMap{}
				if err := cl.Get(ctx, client.ObjectKeyFromObject(cm), got); err == nil {
					got.Finalizers = nil
					Expect(cl.Update(ctx, got)).To(Succeed())
				}
			})

			got := &corev1.ConfigMap{}
			Expect(cl.Get(ctx, client.ObjectKeyFromObject(cm), got)).To(Succeed())

			Expect(cl.Delete(ctx, cm)).To(Succeed())

			afterDelete := &corev1.ConfigMap{}
			Expect(cl.Get(ctx, client.ObjectKeyFromObject(cm), afterDelete)).To(Succeed())
			Expect(afterDelete.DeletionTimestamp).NotTo(BeNil(), "should have a deletion timestamp")
		})
	})
})
