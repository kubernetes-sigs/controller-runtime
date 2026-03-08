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

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		c, err := cache.New(cfg, cache.Options{Scheme: kscheme.Scheme})
		Expect(err).NotTo(HaveOccurred())

		// Set up a Namespace informer as tests will delete namespaces through the consistent client.
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

	Describe("Create then Get", func() {
		It("should immediately observe the created object", func() {
			cm := newConfigMap("default")
			Expect(cl.Create(ctx, cm)).To(Succeed())
			DeferCleanup(func(ctx context.Context) {
				Expect(client.IgnoreNotFound(cl.Delete(ctx, cm))).To(Succeed())
			})

			got := &corev1.ConfigMap{}
			Expect(cl.Get(ctx, client.ObjectKeyFromObject(cm), got)).To(Succeed())
			Expect(got.Name).To(Equal(cm.Name))
			Expect(got.Data).To(Equal(cm.Data))
		})
	})

	Describe("Create then List", func() {
		It("should immediately observe the created object in a list", func() {
			cm := newConfigMap("default")
			Expect(cl.Create(ctx, cm)).To(Succeed())
			DeferCleanup(func(ctx context.Context) {
				Expect(client.IgnoreNotFound(cl.Delete(ctx, cm))).To(Succeed())
			})

			list := &corev1.ConfigMapList{}
			Expect(cl.List(ctx, list, client.InNamespace("default"))).To(Succeed())

			var found bool
			for _, item := range list.Items {
				if item.Name == cm.Name {
					found = true
					Expect(item.Data).To(Equal(cm.Data))
					break
				}
			}
			Expect(found).To(BeTrue(), "created ConfigMap should appear in list")
		})
	})

	Describe("Update then Get", func() {
		It("should immediately observe the updated object", func() {
			cm := newConfigMap("default")
			Expect(cl.Create(ctx, cm)).To(Succeed())
			DeferCleanup(func(ctx context.Context) {
				Expect(client.IgnoreNotFound(cl.Delete(ctx, cm))).To(Succeed())
			})

			got := &corev1.ConfigMap{}
			Expect(cl.Get(ctx, client.ObjectKeyFromObject(cm), got)).To(Succeed())

			got.Data["key"] = "updated"
			Expect(cl.Update(ctx, got)).To(Succeed())

			updated := &corev1.ConfigMap{}
			Expect(cl.Get(ctx, client.ObjectKeyFromObject(cm), updated)).To(Succeed())
			Expect(updated.Data["key"]).To(Equal("updated"))
		})
	})

	Describe("Update then List", func() {
		It("should immediately observe the updated object in a list", func() {
			cm := newConfigMap("default")
			Expect(cl.Create(ctx, cm)).To(Succeed())
			DeferCleanup(func(ctx context.Context) {
				Expect(client.IgnoreNotFound(cl.Delete(ctx, cm))).To(Succeed())
			})

			got := &corev1.ConfigMap{}
			Expect(cl.Get(ctx, client.ObjectKeyFromObject(cm), got)).To(Succeed())

			got.Data["key"] = "updated-for-list"
			Expect(cl.Update(ctx, got)).To(Succeed())

			list := &corev1.ConfigMapList{}
			Expect(cl.List(ctx, list, client.InNamespace("default"))).To(Succeed())

			var found bool
			for _, item := range list.Items {
				if item.Name == cm.Name {
					found = true
					Expect(item.Data["key"]).To(Equal("updated-for-list"))
					break
				}
			}
			Expect(found).To(BeTrue(), "updated ConfigMap should appear in list")
		})
	})

	Describe("Patch then Get", func() {
		It("should immediately observe the patched object", func() {
			cm := newConfigMap("default")
			Expect(cl.Create(ctx, cm)).To(Succeed())
			DeferCleanup(func(ctx context.Context) {
				Expect(client.IgnoreNotFound(cl.Delete(ctx, cm))).To(Succeed())
			})

			got := &corev1.ConfigMap{}
			Expect(cl.Get(ctx, client.ObjectKeyFromObject(cm), got)).To(Succeed())

			patch := client.MergeFrom(got.DeepCopy())
			got.Data["patched"] = "yes"
			Expect(cl.Patch(ctx, got, patch)).To(Succeed())

			patched := &corev1.ConfigMap{}
			Expect(cl.Get(ctx, client.ObjectKeyFromObject(cm), patched)).To(Succeed())
			Expect(patched.Data["patched"]).To(Equal("yes"))
		})
	})

	Describe("Delete then Get", func() {
		It("should immediately observe the object as deleted", func() {
			cm := newConfigMap("default")
			Expect(cl.Create(ctx, cm)).To(Succeed())

			got := &corev1.ConfigMap{}
			Expect(cl.Get(ctx, client.ObjectKeyFromObject(cm), got)).To(Succeed())

			Expect(cl.Delete(ctx, cm)).To(Succeed())

			err := cl.Get(ctx, client.ObjectKeyFromObject(cm), &corev1.ConfigMap{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected NotFound after delete, got: %v", err)
		})
	})

	Describe("Delete then List", func() {
		It("should immediately not include the deleted object in a list", func() {
			cm := newConfigMap("default")
			Expect(cl.Create(ctx, cm)).To(Succeed())

			got := &corev1.ConfigMap{}
			Expect(cl.Get(ctx, client.ObjectKeyFromObject(cm), got)).To(Succeed())

			Expect(cl.Delete(ctx, cm)).To(Succeed())

			list := &corev1.ConfigMapList{}
			Expect(cl.List(ctx, list, client.InNamespace("default"))).To(Succeed())

			for _, item := range list.Items {
				Expect(item.Name).NotTo(Equal(cm.Name), "deleted ConfigMap should not appear in list")
			}
		})
	})

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
