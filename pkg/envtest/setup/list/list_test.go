package list_test

import (
	"cmp"
	"context"
	"regexp"
	"slices"
	"testing"

	"github.com/go-logr/logr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/env"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/list"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/remote"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/testhelpers"
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
	RunSpecs(t, "List Suite")
}

var _ = Describe("List", func() {
	var (
		envOpts []env.Option
	)

	JustBeforeEach(func() {
		addr, shutdown, err := testhelpers.NewServer()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(shutdown)

		envOpts = append(
			envOpts,
			env.WithClient(&remote.GCSClient{ //nolint:staticcheck
				Log:      testLog.WithName("test-remote-client"),
				Bucket:   "kubebuilder-tools-test",
				Server:   addr,
				Insecure: true,
			}),
			env.WithStore(testhelpers.NewMockStore()),
		)
	})

	Context("when downloads are disabled", func() {
		JustBeforeEach(func() {
			envOpts = append(envOpts, env.WithClient(nil)) // ensure tests fail if we try to contact remote
		})

		It("should include local contents sorted by version", func() {
			result, err := list.List(
				ctx,
				versions.AnyVersion,
				list.NoDownload(true),
				list.WithPlatform("*", "*"),
				list.WithEnvOptions(envOpts...),
			)
			Expect(err).NotTo(HaveOccurred())

			// build this list based on test data, to avoid having to change
			// in two places if we add some more test cases
			expected := make([]list.Result, 0)
			for _, v := range testhelpers.LocalVersions {
				for _, p := range v.Platforms {
					expected = append(expected, list.Result{
						Version:  v.Version,
						Platform: p.Platform,
						Status:   list.Installed,
					})
				}
			}
			// this sorting ensures the List method fulfils the contract of
			// returning the most relevant items first
			slices.SortFunc(expected, func(a, b list.Result) int {
				return cmp.Or(
					// we want the results sorted in descending order by version
					cmp.Compare(b.Version.Major, a.Version.Major),
					cmp.Compare(b.Version.Minor, a.Version.Minor),
					cmp.Compare(b.Version.Patch, a.Version.Patch),
					// ..and then in ascending order by platform
					cmp.Compare(a.Platform.OS, b.Platform.OS),
					cmp.Compare(a.Platform.Arch, b.Platform.Arch),
				)
			})

			Expect(result).To(HaveExactElements(expected))
		})

		It("should skip non-matching local contents", func() {
			spec := versions.Spec{
				Selector: versions.PatchSelector{Major: 1, Minor: 16, Patch: versions.AnyPoint},
			}
			result, err := list.List(
				ctx,
				spec,
				list.NoDownload(true),
				list.WithPlatform("linux", "*"),
				list.WithEnvOptions(envOpts...),
			)
			Expect(err).NotTo(HaveOccurred())

			expected := make([]list.Result, 0)
			for _, v := range testhelpers.LocalVersions {
				if !spec.Matches(v.Version) {
					continue
				}
				for _, p := range v.Platforms {
					if p.OS != "linux" {
						continue
					}

					expected = append(expected, list.Result{
						Version:  v.Version,
						Platform: p.Platform,
						Status:   list.Installed,
					})
				}
			}
			// this sorting ensures the List method fulfils the contract of
			// returning the most relevant items first
			slices.SortFunc(expected, func(a, b list.Result) int {
				return cmp.Or(
					// we want the results sorted in descending order by version
					cmp.Compare(b.Version.Major, a.Version.Major),
					cmp.Compare(b.Version.Minor, a.Version.Minor),
					cmp.Compare(b.Version.Patch, a.Version.Patch),
					// ..and then in ascending order by platform
					cmp.Compare(a.Platform.OS, b.Platform.OS),
					cmp.Compare(a.Platform.Arch, b.Platform.Arch),
				)
			})

			Expect(result).To(HaveExactElements(expected))
		})
	})

	Context("when downloads are enabled", func() {
		It("should sort local & remote by version", func() {
			result, err := list.List(
				ctx,
				versions.AnyVersion,
				list.WithPlatform("*", "*"),
				list.WithEnvOptions(envOpts...),
			)
			Expect(err).NotTo(HaveOccurred())

			// build this list based on test data, to avoid having to change
			// in two places if we add some more test cases
			expected := make([]list.Result, 0)
			for _, v := range testhelpers.LocalVersions {
				for _, p := range v.Platforms {
					expected = append(expected, list.Result{
						Version:  v.Version,
						Platform: p.Platform,
						Status:   list.Installed,
					})
				}
			}
			rx := regexp.MustCompile(`^kubebuilder-tools-(\d+\.\d+\.\d+)-(\w+)-(\w+).tar.gz$`)
			for _, v := range testhelpers.RemoteNames {
				if m := rx.FindStringSubmatch(v); m != nil {
					s, err := versions.FromExpr(m[1])
					Expect(err).NotTo(HaveOccurred())

					expected = append(expected, list.Result{
						Version: *s.AsConcrete(),
						Platform: versions.Platform{
							OS:   m[2],
							Arch: m[3],
						},
						Status: list.Available,
					})
				}
			}
			// this sorting ensures the List method fulfils the contract of
			// returning the most relevant items first
			slices.SortFunc(expected, func(a, b list.Result) int {
				return cmp.Or(
					// we want installed versions first, available after;
					// compare in reverse order since "installed > "available"
					cmp.Compare(b.Status, a.Status),
					// then, sort in descending order by version
					cmp.Compare(b.Version.Major, a.Version.Major),
					cmp.Compare(b.Version.Minor, a.Version.Minor),
					cmp.Compare(b.Version.Patch, a.Version.Patch),
					// ..and then in ascending order by platform
					cmp.Compare(a.Platform.OS, b.Platform.OS),
					cmp.Compare(a.Platform.Arch, b.Platform.Arch),
				)
			})

			Expect(result).To(HaveExactElements(expected))
		})

		It("should skip non-matching remote contents", func() {
			result, err := list.List(
				ctx,
				versions.Spec{
					Selector: versions.PatchSelector{Major: 1, Minor: 16, Patch: versions.AnyPoint},
				},
				list.WithPlatform("*", "*"),
				list.WithEnvOptions(envOpts...),
			)
			Expect(err).NotTo(HaveOccurred())

			// build this list based on test data, to avoid having to change
			// in two places if we add some more test cases
			expected := make([]list.Result, 0)
			for _, v := range testhelpers.LocalVersions {
				// ignore versions not matching the filter in the options
				if v.Version.Major != 1 || v.Version.Minor != 16 {
					continue
				}
				for _, p := range v.Platforms {
					expected = append(expected, list.Result{
						Version:  v.Version,
						Platform: p.Platform,
						Status:   list.Installed,
					})
				}
			}
			rx := regexp.MustCompile(`^kubebuilder-tools-(\d+\.\d+\.\d+)-(\w+)-(\w+).tar.gz$`)
			for _, v := range testhelpers.RemoteNames {
				if m := rx.FindStringSubmatch(v); m != nil {
					s, err := versions.FromExpr(m[1])
					Expect(err).NotTo(HaveOccurred())
					v := *s.AsConcrete()
					// ignore versions not matching the filter in the options
					if v.Major != 1 || v.Minor != 16 {
						continue
					}
					expected = append(expected, list.Result{
						Version: v,
						Platform: versions.Platform{
							OS:   m[2],
							Arch: m[3],
						},
						Status: list.Available,
					})
				}
			}
			// this sorting ensures the List method fulfils the contract of
			// returning the most relevant items first
			slices.SortFunc(expected, func(a, b list.Result) int {
				return cmp.Or(
					// we want installed versions first, available after;
					// compare in reverse order since "installed > "available"
					cmp.Compare(b.Status, a.Status),
					// then, sort in descending order by version
					cmp.Compare(b.Version.Major, a.Version.Major),
					cmp.Compare(b.Version.Minor, a.Version.Minor),
					cmp.Compare(b.Version.Patch, a.Version.Patch),
					// ..and then in ascending order by platform
					cmp.Compare(a.Platform.OS, b.Platform.OS),
					cmp.Compare(a.Platform.Arch, b.Platform.Arch),
				)
			})

			Expect(result).To(HaveExactElements(expected))
		})
	})
})
