package controlplane_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	kauthn "k8s.io/api/authorization/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	. "sigs.k8s.io/controller-runtime/pkg/internal/testing/controlplane"
)

var _ = Describe("Control Plane", func() {
	It("should start and stop successfully with a default etcd & apiserver", func() {
		plane := &ControlPlane{}
		Expect(plane.Start()).To(Succeed())
		Expect(plane.Stop()).To(Succeed())
	})
	It("should use the given etcd & apiserver when starting, if present", func() {
		apiServer := &APIServer{}
		etcd := &Etcd{}
		plane := &ControlPlane{
			APIServer: apiServer,
			Etcd:      etcd,
		}
		Expect(plane.Start()).To(Succeed())
		defer func() { Expect(plane.Stop()).To(Succeed()) }()

		Expect(plane.APIServer).To(BeIdenticalTo(apiServer))
		Expect(plane.Etcd).To(BeIdenticalTo(etcd))
	})

	Context("after having started", func() {
		var plane *ControlPlane
		BeforeEach(func() {
			plane = &ControlPlane{}
			Expect(plane.Start()).To(Succeed())
		})
		AfterEach(func() {
			Expect(plane.Stop()).To(Succeed())
		})

		It("should provision a working legacy user and legacy kubectl", func() {
			By("grabbing the legacy kubectl")
			Expect(plane.KubeCtl()).NotTo(BeNil())

			By("grabbing the legacy REST config and testing it")
			cfg, err := plane.RESTClientConfig()
			Expect(err).NotTo(HaveOccurred(), "should be able to grab the legacy REST config")
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred(), "should be able to create a client")

			sar := &kauthn.SelfSubjectAccessReview{
				Spec: kauthn.SelfSubjectAccessReviewSpec{
					ResourceAttributes: &kauthn.ResourceAttributes{
						Verb:     "*",
						Group:    "*",
						Version:  "*",
						Resource: "*",
					},
				},
			}
			Expect(cl.Create(context.Background(), sar)).To(Succeed(), "should be able to make a Self-SAR")
			Expect(sar.Status.Allowed).To(BeTrue(), "admin user should be able to do everything")
		})

		// TODO(directxman12): more explicit tests for AddUser -- it's tested indirectly via the
		// legacy user flow, but we should be explicit

		Describe("adding users", func() {
			PIt("should be able to provision new users that have a corresponding REST config and & kubectl", func() {

			})

			PIt("should produce a default base REST config if none is given to add", func() {

			})
		})
	})
})
