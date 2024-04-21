package cleanup_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/afero"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/cleanup"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/env"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/store"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/testhelpers"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/versions"
)

var (
	testLog logr.Logger
	ctx     context.Context
)

func TestCleanup(t *testing.T) {
	testLog = testhelpers.GetLogger()
	ctx = logr.NewContext(context.Background(), testLog)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Cleanup Suite")
}

var _ = Describe("Cleanup", func() {
	var (
		defaultEnvOpts []env.Option
		s              *store.Store
	)

	BeforeEach(func() {
		s = testhelpers.NewMockStore()
	})

	JustBeforeEach(func() {
		defaultEnvOpts = []env.Option{
			env.WithClient(nil), // ensures we fail if we try to connect
			env.WithStore(s),
			env.WithFS(afero.NewIOFS(s.Root)),
		}
	})

	Context("when cleanup is run", func() {
		version := versions.Spec{
			Selector: versions.Concrete{
				Major: 1,
				Minor: 16,
				Patch: 1,
			},
		}

		var (
			matching, nonMatching []store.Item
		)

		BeforeEach(func() {
			// ensure there are some versions matching what we're about to delete
			var err error
			matching, err = s.List(ctx, store.Filter{Version: version, Platform: versions.Platform{OS: "linux", Arch: "amd64"}})
			Expect(err).NotTo(HaveOccurred())
			Expect(matching).NotTo(BeEmpty(), "found no matching versions before cleanup")

			// ensure there are some versions _not_ matching what we're about to delete
			nonMatching, err = s.List(ctx, store.Filter{Version: versions.Spec{Selector: versions.PatchSelector{Major: 1, Minor: 17, Patch: versions.AnyPoint}}, Platform: versions.Platform{OS: "linux", Arch: "amd64"}})
			Expect(err).NotTo(HaveOccurred())
			Expect(nonMatching).NotTo(BeEmpty(), "found no non-matching versions before cleanup")
		})

		JustBeforeEach(func() {
			Expect(cleanup.Cleanup(
				ctx,
				version,
				cleanup.WithPlatform("linux", "amd64"),
				cleanup.WithEnvOptions(defaultEnvOpts...),
			)).To(Succeed())
		})

		It("should remove matching versions", func() {
			items, err := s.List(ctx, store.Filter{Version: version, Platform: versions.Platform{OS: "linux", Arch: "amd64"}})
			Expect(err).NotTo(HaveOccurred())
			Expect(items).To(BeEmpty(), "found matching versions after cleanup")
		})

		It("should not remove non-matching versions", func() {
			items, err := s.List(ctx, store.Filter{Version: versions.AnyVersion, Platform: versions.Platform{OS: "*", Arch: "*"}})
			Expect(err).NotTo(HaveOccurred())
			Expect(items).To(ContainElements(nonMatching), "non-matching items were affected")
		})
	})
})
