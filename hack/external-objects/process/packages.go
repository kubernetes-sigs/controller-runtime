package process

import (
	"strings"
	"path"

	"sigs.k8s.io/controller-runtime/hack/external-objects/locate"
)

// OwnedPackages returns all packages whose names match the given prefix.
func OwnedPackages(allPackages []*locate.PackageInfo, prefix string, skipVendor bool) []*locate.PackageInfo {
	vendorPath := path.Join(prefix, "vendor")
	var owned []*locate.PackageInfo
	for _, pkg := range allPackages {
		if pkg.BuildInfo.Goroot || !strings.HasPrefix(pkg.BuildInfo.ImportPath, prefix) {
			continue
		}
		if skipVendor && strings.HasPrefix(pkg.BuildInfo.ImportPath, vendorPath) {
			continue
		}
		owned = append(owned, pkg)
	}

	return owned
}
