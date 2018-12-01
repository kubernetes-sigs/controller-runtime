package filter

import (
	"path"
	"strings"

	"sigs.k8s.io/controller-runtime/hack/external-objects/locate"
)

// PackageInfoResolver knows how to resolve package info for a
// particular path.
type PackageInfoResolver interface {
	// PackageInfoFor resolves the package information for the
	// given package path (can be a filesystem path) retrieved
	// from a types.Package object imported by the source of this
	// resolver.  It may return nil for paths from other sources.
	PackageInfoFor(path string) *locate.PackageInfo

	// FetchPackageInfoFor resolves the package information for the
	// the given path (like PackageInfoFor), but may also attempt to
	// load basic package info if the given package info hasn't been
	// cached.
	FetchPackageInfoFor(path string) (*locate.PackageInfo, error)
}

// FilterEnvironment represents commonly required information for executing filters
type FilterEnvironment struct {
	// RootPackage is the base package to which all filtered packages and objects belong
	RootPackage *locate.PackageInfo

	// Loader can be used to locate import path information
	Loader PackageInfoResolver
	
	// VendorImportPrefix is the import path prefix
	// for the vendor directory (including the root package).
	// If empty, it will be ignored.
	VendorImportPrefix string
}

func NewFilterEnvironment(rootPackage *locate.PackageInfo, loader PackageInfoResolver, isProject bool) *FilterEnvironment {
	vendorPrefix := ""
	if isProject {
		vendorPrefix = path.Join(rootPackage.BuildInfo.ImportPath, "vendor") + "/"
	}

	return &FilterEnvironment{
		RootPackage: rootPackage,
		Loader: loader,
		VendorImportPrefix: vendorPrefix,
	}
}

// StripVendor strips the vendor prefix from the given path, if set and applicable.
func (e *FilterEnvironment) StripVendor(importPath string) string {
	if e.VendorImportPrefix != "" {
		return strings.TrimPrefix(importPath, e.VendorImportPrefix)
	}

	return importPath
}
