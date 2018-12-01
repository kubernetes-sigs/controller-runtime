package filter

import (
	"go/types"
	"strings"
	"fmt"

	"sigs.k8s.io/controller-runtime/hack/external-objects/process"
)

// FindExternalReferences returns all external references in the
// type signature of the given object, and all referenced types,
// in public types/fields/etc.
func FindExternalReferences(obj types.Object, env *FilterEnvironment) []*types.TypeName {
	externalRefsRaw := process.FilterMapTypesInObject(obj, ExternalReferencesFilter(env))
	externalRefs := make([]*types.TypeName, len(externalRefsRaw))
	for i, ref := range externalRefsRaw {
		externalRefs[i] = ref.(*types.TypeName)
	}

	return externalRefs
}

// ExternalReferencesFind returns a FilterMapFunc that finds all references to packages outside
// the given "root" package (and subpackages).  The returned objects are *types.TypeName objects.
// The loader is used to figure out actual import paths for packages.
func ExternalReferencesFilter(env *FilterEnvironment) process.FilterMapFunc {
	return func(typ types.Type) interface{} {
		if typ == nil {
			// we don't care about the end of a given type
			return nil
		}
		ref := GetExternalReference(env, typ)
		// avoid a present-but-nil-valued interface{} object
		if ref == nil {
			return nil
		}
		return ref
	}
}

// GetExternalReference checks the given type to see if it's a named type, and if so,
// if that name refers to a package outside the given root package's package hierarchy.
// If so, it returns that name, otherwise returning nil.  The given file loader is used
// to produce actual import paths for any type (since the package info might have relative
// filesystem paths, depending on where we're called from).  Packages in the GoRoot
// (the Go standard library) are skipped.
func GetExternalReference(env *FilterEnvironment, typ types.Type) *types.TypeName {
	namedType, isNamedType := typ.(*types.Named)
	if !isNamedType {
		return nil
	}

	typeName := namedType.Obj()

	if typeName.Pkg() == nil {
		// a built-in type like "error"
		return nil
	}

	packageInfo, err := env.Loader.FetchPackageInfoFor(typeName.Pkg().Path())
	if err != nil {
		panic(fmt.Sprintf("unknown package %s while proccessing %s: %v", typeName.Pkg().Path(), typ, err))
		return nil
	}
	if packageInfo.BuildInfo.Goroot {
		// skip builtins
		return nil
	}
	actualPackagePath := env.StripVendor(packageInfo.BuildInfo.ImportPath)
	if strings.HasPrefix(actualPackagePath, env.RootPackage.BuildInfo.ImportPath) {
		return nil
	}

	return typeName
}
