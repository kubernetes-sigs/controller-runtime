package main

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
)

func TestRun(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "MirrorPod reconciler Integration Suite", []Reporter{printer.NewlineReporter{}})
}

var _ = Describe("clusterconnector.ClusterConnector", func() {
	var stop chan struct{}
	var referenceClusterCfg *rest.Config
	var mirrorClusterCfg *rest.Config
	var referenceClusterTestEnv *envtest.Environment
	var mirrorClusterTestEnv *envtest.Environment
	var referenceClusterClient client.Client
	var mirrorClusterClient client.Client

	Describe("multi-cluster-controller", func() {
		BeforeEach(func() {
			stop = make(chan struct{})
			referenceClusterTestEnv = &envtest.Environment{}
			mirrorClusterTestEnv = &envtest.Environment{}

			var err error
			referenceClusterCfg, err = referenceClusterTestEnv.Start()
			Expect(err).NotTo(HaveOccurred())

			mirrorClusterCfg, err = mirrorClusterTestEnv.Start()
			Expect(err).NotTo(HaveOccurred())

			referenceClusterClient, err = client.New(referenceClusterCfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())

			mirrorClusterClient, err = client.New(mirrorClusterCfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())

			go func() {
				run(referenceClusterCfg, mirrorClusterCfg, stop)
			}()
		})

		AfterEach(func() {
			close(stop)
			Expect(referenceClusterTestEnv.Stop()).To(Succeed())
			Expect(mirrorClusterTestEnv.Stop()).To(Succeed())
		})

		It("Should reconcile pods", func() {
			ctx := context.Background()
			referencePod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "satan",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "fancy-one",
						Image: "nginx",
					}},
				},
			}
			Expect(referenceClusterClient.Create(ctx, referencePod)).NotTo(HaveOccurred())
			name := types.NamespacedName{Namespace: referencePod.Namespace, Name: referencePod.Name}

			By("Setting a finalizer", func() {
				Eventually(func() error {
					updatedPod := &corev1.Pod{}
					if err := referenceClusterClient.Get(ctx, name, updatedPod); err != nil {
						return err
					}
					if n := len(updatedPod.Finalizers); n != 1 {
						return fmt.Errorf("expected exactly one finalizer, got %d", n)
					}
					return nil
				}).Should(Succeed())

			})

			By("Creating a pod in the mirror cluster", func() {
				Eventually(func() error {
					return mirrorClusterClient.Get(ctx, name, &corev1.Pod{})
				}).Should(Succeed())

			})

			By("Recreating a manually deleted pod in the mirror cluster", func() {
				Expect(mirrorClusterClient.Delete(ctx,
					&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: name.Namespace, Name: name.Name}}),
				).NotTo(HaveOccurred())

				Eventually(func() error {
					return mirrorClusterClient.Get(ctx, name, &corev1.Pod{})
				}).Should(Succeed())

			})

			By("Cleaning up after the reference pod got deleted", func() {
				Expect(referenceClusterClient.Delete(ctx, referencePod)).NotTo(HaveOccurred())

				Eventually(func() bool {
					return apierrors.IsNotFound(mirrorClusterClient.Get(ctx, name, &corev1.Pod{}))
				}).Should(BeTrue())

				Eventually(func() bool {
					return apierrors.IsNotFound(referenceClusterClient.Get(ctx, name, &corev1.Pod{}))
				}).Should(BeTrue())
			})
		})

	})

})
