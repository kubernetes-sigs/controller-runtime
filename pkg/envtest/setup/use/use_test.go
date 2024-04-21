package use_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/afero"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/env"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/remote"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/testhelpers"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/use"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/versions"
)

var (
	testLog logr.Logger
	ctx     context.Context
)

func TestEnv(t *testing.T) {
	testLog = testhelpers.GetLogger()
	ctx = logr.NewContext(context.Background(), testLog)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Use Suite")
}

var _ = Describe("Use", func() {
	var (
		defaultEnvOpts []env.Option
		version        = versions.Spec{
			Selector: versions.Concrete{Major: 1, Minor: 16, Patch: 0},
		}
	)
	JustBeforeEach(func() {
		addr, shutdown, err := testhelpers.NewServer()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(shutdown)

		s := testhelpers.NewMockStore()

		defaultEnvOpts = []env.Option{
			env.WithClient(&remote.GCSClient{ //nolint:staticcheck
				Log:      testLog.WithName("test-remote-client"),
				Bucket:   "kubebuilder-tools-test",
				Server:   addr,
				Insecure: true,
			}),
			env.WithStore(s),
			env.WithFS(afero.NewIOFS(s.Root)),
		}
	})

	Context("when useEnv is set", func() {
		It("should fall back to normal behavior when the env is not set", func() {
			result, err := use.Use(
				ctx,
				version,
				use.WithAssetsFromEnv(true),
				use.WithPlatform("*", "*"),
				use.WithEnvOptions(defaultEnvOpts...),
			)

			Expect(err).NotTo(HaveOccurred())

			Expect(result.Version).To(Equal(version))
			Expect(result.Path).To(HaveSuffix("/1.16.0-linux-amd64"), "should fall back to a local version")
		})

		It("should fall back to normal behavior if binaries are missing", func() {
			result, err := use.Use(
				ctx,
				version,
				use.WithAssetsFromEnv(true),
				use.WithAssetsAt(".test-binaries/missing-binaries"),
				use.WithPlatform("*", "*"),
				use.WithEnvOptions(defaultEnvOpts...),
			)

			Expect(err).NotTo(HaveOccurred())

			Expect(result.Version).To(Equal(version), "should fall back to a local version")
			Expect(result.Path).To(HaveSuffix("/1.16.0-linux-amd64"))
		})

		It("should use the value of the env if it contains the right binaries", func() {
			result, err := use.Use(
				ctx,
				version,
				use.WithAssetsFromEnv(true),
				use.WithAssetsAt("a/good/version"),
				use.WithPlatform("*", "*"),
				use.WithEnvOptions(defaultEnvOpts...),
			)

			Expect(err).NotTo(HaveOccurred())

			Expect(result.Version).To(Equal(versions.AnyVersion))
			Expect(result.Path).To(HaveSuffix("/good/version"))
		})

		It("should not try to check the version of the binaries", func() {
			result, err := use.Use(
				ctx,
				version,
				use.WithAssetsFromEnv(true),
				use.WithAssetsAt("wrong/version"),
				use.WithPlatform("*", "*"),
				use.WithEnvOptions(defaultEnvOpts...),
			)

			Expect(err).NotTo(HaveOccurred())

			Expect(result.Version).To(Equal(versions.AnyVersion))
			Expect(result.Path).To(Equal("wrong/version"))
		})

		It("should not need to contact the network", func() {
			result, err := use.Use(
				ctx,
				version,
				use.WithAssetsFromEnv(true),
				use.WithAssetsAt("a/good/version"),
				use.WithPlatform("*", "*"),
				use.WithEnvOptions(append(defaultEnvOpts, env.WithClient(nil))...),
			)

			Expect(err).NotTo(HaveOccurred())

			Expect(result.Version).To(Equal(versions.AnyVersion))
			Expect(result.Path).To(HaveSuffix("/good/version"))
		})
	})

	Context("when downloads are disabled", func() {
		It("should error if no matches are found locally", func() {
			_, err := use.Use(
				ctx,
				versions.Spec{Selector: versions.Concrete{Major: 9001}},
				use.NoDownload(true),
				use.WithPlatform("*", "*"),
				// ensures tests panic if we try to connect to the network
				use.WithEnvOptions(append(defaultEnvOpts, env.WithClient(nil))...),
			)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(use.ErrNoMatchingVersion))
		})

		It("should settle for the latest local match if latest is requested", func() {
			result, err := use.Use(
				ctx,
				versions.Spec{
					CheckLatest: true,
					Selector: versions.PatchSelector{
						Major: 1,
						Minor: 16,
						Patch: versions.AnyPoint,
					},
				},
				use.WithPlatform("*", "*"),
				use.NoDownload(true),
				use.WithEnvOptions(append(defaultEnvOpts, env.WithClient(nil))...),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Version).To(Equal(versions.Spec{Selector: versions.Concrete{Major: 1, Minor: 16, Patch: 2}}))
		})
	})

	Context("if latest is requested", func() {
		It("should contact the network to see if there's anything newer", func() {
			result, err := use.Use(
				ctx,
				versions.Spec{
					CheckLatest: true,
					Selector: versions.PatchSelector{
						Major: 1,
						Minor: 16,
						Patch: versions.AnyPoint,
					},
				},
				use.WithPlatform("*", "*"),
				use.WithEnvOptions(defaultEnvOpts...),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Version).To(Equal(versions.Spec{Selector: versions.Concrete{Major: 1, Minor: 16, Patch: 4}}))
		})

		It("should still use the latest local if the network doesn't have anything newer", func() {
			result, err := use.Use(
				ctx,
				versions.Spec{
					CheckLatest: true,
					Selector: versions.PatchSelector{
						Major: 1,
						Minor: 14,
						Patch: versions.AnyPoint,
					},
				},
				use.WithPlatform("linux", "amd64"),
				use.WithEnvOptions(defaultEnvOpts...),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Version).To(Equal(versions.Spec{Selector: versions.Concrete{Major: 1, Minor: 14, Patch: 26}}))
		})
	})

	It("should check for a local match first", func() {
		result, err := use.Use(
			ctx,
			versions.Spec{
				Selector: versions.TildeSelector{
					Concrete: versions.Concrete{Major: 1, Minor: 16, Patch: 0},
				},
			},
			use.WithPlatform("linux", "amd64"),
			use.WithEnvOptions(append(defaultEnvOpts, env.WithClient(nil))...),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Version).To(Equal(versions.Spec{Selector: versions.Concrete{Major: 1, Minor: 16, Patch: 1}}))
	})

	It("should fall back to the network if no local matches are found", func() {
		result, err := use.Use(
			ctx,
			versions.Spec{
				Selector: versions.TildeSelector{
					Concrete: versions.Concrete{Major: 1, Minor: 19, Patch: 0},
				},
			},
			use.WithPlatform("linux", "amd64"),
			use.WithEnvOptions(defaultEnvOpts...),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Version).To(Equal(versions.Spec{Selector: versions.Concrete{Major: 1, Minor: 19, Patch: 2}}))
	})

	It("should error out if no matches can be found anywhere", func() {
		_, err := use.Use(
			ctx,
			versions.Spec{
				Selector: versions.Concrete{Major: 1, Minor: 13, Patch: 0},
			},
			use.WithPlatform("*", "*"),
			use.WithEnvOptions(defaultEnvOpts...),
		)

		Expect(err).To(MatchError(use.ErrNoMatchingVersion))
	})

	It("should skip local version matches with non-matching platform", func() {
		_, err := use.Use(
			ctx,
			versions.Spec{
				Selector: versions.Concrete{Minor: 1, Major: 16, Patch: 2},
			},
			use.WithPlatform("linux", "amd64"),
			use.NoDownload(true),
			use.WithEnvOptions(defaultEnvOpts...),
		)

		Expect(err).To(MatchError(use.ErrNoMatchingVersion))
	})

	It("should skip remote version matches with non-matching platform", func() {
		_, err := use.Use(
			ctx,
			versions.Spec{
				Selector: versions.Concrete{Minor: 1, Major: 11, Patch: 1},
			},
			use.WithPlatform("linux", "amd64"),
			use.NoDownload(true),
			use.WithEnvOptions(defaultEnvOpts...),
		)

		Expect(err).To(MatchError(use.ErrNoMatchingVersion))
	})

	Context("with an invalid checksum", func() {
		var client remote.Client
		BeforeEach(func() {
			name := "kubebuilder-tools-86.75.309-linux-amd64.tar.gz"
			contents, err := testhelpers.ContentsFor(name)
			Expect(err).NotTo(HaveOccurred())

			server, stop, err := testhelpers.NewServer(testhelpers.Item{
				Meta: testhelpers.BucketObject{
					Name: name,
					Hash: "not the right one!",
				},
				Contents: contents,
			})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(stop)

			client = &remote.GCSClient{ //nolint:staticcheck
				Bucket:   "kubebuilder-tools-test",
				Server:   server,
				Insecure: true,
				Log:      testLog.WithName("test-remote-client"),
			}
		})

		When("validating the checksum", func() {
			It("should fail with an appropriate error", func() {
				_, err := use.Use(
					ctx,
					versions.Spec{
						Selector: versions.Concrete{
							Major: 86,
							Minor: 75,
							Patch: 309,
						},
					},
					use.VerifySum(true),
					use.WithEnvOptions(append(defaultEnvOpts, env.WithClient(client))...),
				)

				Expect(err).To(MatchError(remote.ErrChecksumMismatch))
			})
		})

		When("not validating checksum", func() {
			It("should return the version without error", func() {
				result, err := use.Use(
					ctx,
					versions.Spec{
						Selector: versions.Concrete{
							Major: 86,
							Minor: 75,
							Patch: 309,
						},
					},
					use.WithEnvOptions(append(defaultEnvOpts, env.WithClient(client))...),
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Version).To(Equal(versions.Spec{Selector: versions.Concrete{Major: 86, Minor: 75, Patch: 309}}))
			})
		})
	})
})
