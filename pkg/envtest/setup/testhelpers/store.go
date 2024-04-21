package testhelpers

import (
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/spf13/afero"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/store"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/versions"
)

var (
	// keep this sorted.

	// LocalVersions is a list of versions that the test helpers make available in the local store
	LocalVersions = []versions.Set{
		{Version: versions.Concrete{Major: 1, Minor: 17, Patch: 9}, Platforms: []versions.PlatformItem{
			{Platform: versions.Platform{OS: "linux", Arch: "amd64"}},
		}},
		{Version: versions.Concrete{Major: 1, Minor: 16, Patch: 2}, Platforms: []versions.PlatformItem{
			{Platform: versions.Platform{OS: "linux", Arch: "yourimagination"}},
			{Platform: versions.Platform{OS: "ifonlysingularitywasstillathing", Arch: "amd64"}},
		}},
		{Version: versions.Concrete{Major: 1, Minor: 16, Patch: 1}, Platforms: []versions.PlatformItem{
			{Platform: versions.Platform{OS: "linux", Arch: "amd64"}},
		}},
		{Version: versions.Concrete{Major: 1, Minor: 16, Patch: 0}, Platforms: []versions.PlatformItem{
			{Platform: versions.Platform{OS: "linux", Arch: "amd64"}},
		}},
		{Version: versions.Concrete{Major: 1, Minor: 14, Patch: 26}, Platforms: []versions.PlatformItem{
			{Platform: versions.Platform{OS: "linux", Arch: "amd64"}},
			{Platform: versions.Platform{OS: "hyperwarp", Arch: "pixiedust"}},
		}},
	}
)

func initializeFakeStore(fs afero.Afero, dir string) {
	ginkgo.By("making the unpacked directory")
	unpackedBase := filepath.Join(dir, "k8s")
	gomega.Expect(fs.Mkdir(unpackedBase, 0755)).To(gomega.Succeed())

	ginkgo.By("making some fake (empty) versions")
	for _, set := range LocalVersions {
		for _, plat := range set.Platforms {
			gomega.Expect(fs.Mkdir(filepath.Join(unpackedBase, plat.BaseName(set.Version)), 0755)).To(gomega.Succeed())
		}
	}

	ginkgo.By("making some fake non-store paths")
	gomega.Expect(fs.Mkdir(filepath.Join(dir, "missing", "binaries"), 0755)).To(gomega.Succeed())

	gomega.Expect(fs.Mkdir(filepath.Join(dir, "wrong", "version"), 0755)).To(gomega.Succeed())
	gomega.Expect(fs.WriteFile(filepath.Join(dir, "wrong", "version", "kube-apiserver"), nil, 0755)).To(gomega.Succeed())
	gomega.Expect(fs.WriteFile(filepath.Join(dir, "wrong", "version", "kubectl"), nil, 0755)).To(gomega.Succeed())
	gomega.Expect(fs.WriteFile(filepath.Join(dir, "wrong", "version", "etcd"), nil, 0755)).To(gomega.Succeed())

	gomega.Expect(fs.Mkdir(filepath.Join(dir, "a", "good", "version"), 0755)).To(gomega.Succeed())
	gomega.Expect(fs.WriteFile(filepath.Join(dir, "a", "good", "version", "kube-apiserver"), nil, 0755)).To(gomega.Succeed())
	gomega.Expect(fs.WriteFile(filepath.Join(dir, "a", "good", "version", "kubectl"), nil, 0755)).To(gomega.Succeed())
	gomega.Expect(fs.WriteFile(filepath.Join(dir, "a", "good", "version", "etcd"), nil, 0755)).To(gomega.Succeed())
	// TODO: put the right files
}

// NewMockStore creates a new in-memory store, prepopulated with a set of packages
func NewMockStore() *store.Store {
	fs := afero.NewMemMapFs()
	storeRoot := ".test-binaries"

	initializeFakeStore(afero.Afero{Fs: fs}, storeRoot)

	return &store.Store{Root: afero.NewBasePathFs(fs, storeRoot)}
}
