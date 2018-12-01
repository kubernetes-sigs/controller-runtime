package main

import (
	"flag"
	"fmt"
	"go/types"
	"os"
	"strings"

	"sigs.k8s.io/controller-runtime/hack/external-objects/filter"
	"sigs.k8s.io/controller-runtime/hack/external-objects/locate"
	"sigs.k8s.io/controller-runtime/hack/external-objects/process"
)

type typeIdent struct {
	path, name string
}

func whatRefs(loader *locate.FileLoader, allExternalRefs refsMap, filterEnv *filter.FilterEnvironment, namesToFind []string) {
	typesToFind := make(map[typeIdent]struct{}, len(namesToFind))
	for _, name := range namesToFind {
		nameParts := strings.Split(name, "#")
		if len(nameParts) != 2 {
			fmt.Fprintf(flag.CommandLine.Output(), "invalid type %s\n", name)
			flag.Usage()
			os.Exit(1)
		}
		typesToFind[typeIdent{name: nameParts[1], path: nameParts[0]}] = struct{}{}
	}

	for ref, sourceObjects := range allExternalRefs {
		pkgInfo := loader.PackageInfoFor(ref.Pkg().Path())
		ident := typeIdent{
			name: ref.Name(),
			path: filterEnv.StripVendor(pkgInfo.BuildInfo.ImportPath),
		}
		if _, isRelevant := typesToFind[ident]; !isRelevant {
			continue
		}
		fmt.Printf("\nexternal reference %q.%s\n", pkgInfo.BuildInfo.ImportPath, ref.Name())
		for _, sourceObject := range sourceObjects {
			fmt.Printf("\n  from %s\n    via ", sourceObject)
			v := &typePathRecorder{
				target: ref,
			}
			process.FilterMapTypesInObject(sourceObject, v.Record)
			for _, typ := range v.typeStack {
				fmt.Printf("%s\n        ", typ)
			}
		}
	}
}

type typePathRecorder struct {
	typeStack []types.Type
	done      bool
	target    *types.TypeName
}

func (s *typePathRecorder) Record(typ types.Type) interface{} {
	if s.done {
		return nil
	}

	if typ == nil {
		s.typeStack = s.typeStack[:len(s.typeStack)-1]
		return nil
	}

	if namedType, isNamedType := typ.(*types.Named); isNamedType && namedType.Obj() == s.target {
		s.done = true
		return nil
	}

	s.typeStack = append(s.typeStack, typ)

	return nil
}
