package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/types"
	"sort"
	"strings"

	"sigs.k8s.io/controller-runtime/hack/external-objects/filter"
	"sigs.k8s.io/controller-runtime/hack/external-objects/locate"
)

var (
	useSignatures = flag.Bool("with-sig", true, "print public type signatures when finding all references")
)

func findAll(loader *locate.FileLoader, allExternalRefs refsMap, filterEnv *filter.FilterEnvironment) {
	// sort by package name, then type name
	sortedRefs := make([]*types.TypeName, 0, len(allExternalRefs))
	for ref := range allExternalRefs {
		sortedRefs = append(sortedRefs, ref)
	}
	sort.Slice(sortedRefs, func(i, j int) bool {
		refI := sortedRefs[i]
		refJ := sortedRefs[j]
		pkgInfoI := loader.PackageInfoFor(refI.Pkg().Path())
		pkgInfoJ := loader.PackageInfoFor(refJ.Pkg().Path())
		if pkgInfoI.BuildInfo.ImportPath == pkgInfoJ.BuildInfo.ImportPath {
			return refI.Name() < refJ.Name()
		}

		return pkgInfoI.BuildInfo.ImportPath < pkgInfoJ.BuildInfo.ImportPath
	})

	// print all the sorted, deduped external types
	for _, ref := range sortedRefs {
		pkgInfo := loader.PackageInfoFor(ref.Pkg().Path())
		importPath := filterEnv.StripVendor(pkgInfo.BuildInfo.ImportPath)
		if *skipNonKube && !strings.HasPrefix(importPath, "k8s.io/") {
			continue
		}
		fmt.Printf("%q.%s", importPath, ref.Name())
		if *useSignatures {
			fmt.Printf(" -- %s", typeSig(ref.Type().Underlying()))
		}
		fmt.Printf("\n")
	}
}

func typeSig(ref types.Type) string {
	// like types.TypeString, but only exported fields for structs (and no struct tags)
	switch typ := ref.(type) {
	case *types.Struct:
		var buff bytes.Buffer
		buff.WriteString("struct{")
		for i := 0; i < typ.NumFields(); i++ {
			field := typ.Field(i)
			if !field.Embedded() && !ast.IsExported(field.Name()) {
				continue
			}
			if i > 0 {
				buff.WriteString("; ")
			}
			if !field.Embedded() {
				if !ast.IsExported(field.Name()) {
					continue
				}
				buff.WriteString(field.Name())
				buff.WriteByte(' ')
			}
			buff.WriteString(typeSig(field.Type()))
		}
		buff.WriteString("}")
		return buff.String()
	default:
		return ref.String()
	}

}
