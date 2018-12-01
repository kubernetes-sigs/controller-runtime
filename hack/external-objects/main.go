package main

import (
	"flag"
	"fmt"
	"go/types"
	"os"

	"sigs.k8s.io/controller-runtime/hack/external-objects/filter"
	"sigs.k8s.io/controller-runtime/hack/external-objects/locate"
	"sigs.k8s.io/controller-runtime/hack/external-objects/process"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	forceTraverse = flag.Bool("f", false, "force traversing subdirectories when passed a path that doesn't look like a full project")
	skipNonKube   = flag.Bool("skip-non-k8s", true, "skip printing non-k8s.io external types")
	rootPath      = flag.String("root", ".", "root package path")
	log           = logf.Log.WithName("main")
)

type refsMap map[*types.TypeName][]types.Object

func main() {
	flag.Parse()
	logf.SetLogger(logf.ZapLogger(true))

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n%s (find|what-refs package#Name[ package#Name...]) [flags]\n", os.Args[0], os.Args[0])
		flag.PrintDefaults()
	}

	if flag.NArg() < 1 {
		fmt.Fprintf(flag.CommandLine.Output(), "must specify a command")
		flag.Usage()
		os.Exit(1)
	}

	cmd := flag.Arg(0)
	otherArgs := flag.Args()[1:]

	loader, isProject := setupLoader()
	allExternalRefs, filterEnv := getExternalRefs(loader, isProject)

	switch cmd {
	case "what-refs":
		whatRefs(loader, allExternalRefs, filterEnv, otherArgs)
	case "find":
		findAll(loader, allExternalRefs, filterEnv)
	default:
		fmt.Fprintf(flag.CommandLine.Output(), "unknown command %s\n", cmd)
		flag.Usage()
		os.Exit(1)
	}
}

func setupLoader() (*locate.FileLoader, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		log.Error(err, "can't figure out the current working directory")
		os.Exit(1)
	}
	loader := locate.NewLoader(cwd, log)

	// import the root package and all subpackages
	isProject, err := locate.ImportProject(loader, *rootPath, *forceTraverse)
	if err != nil {
		log.Error(err, "unable to import path", "is project", isProject)
		os.Exit(1)
	}

	// fall back to just the given package if needed (if we already loaded
	// the project, this does nothing)
	_, err = loader.Import(*rootPath)
	if err != nil {
		log.Error(err, "unable to import package")
		os.Exit(1)
	}
	log.V(1).Info("finished loading packages", "package path", *rootPath, "is project", isProject)

	return loader, isProject
}

func getExternalRefs(loader *locate.FileLoader, isProject bool) (refsMap, *filter.FilterEnvironment) {
	// set up the info we need to actually do the filtering
	projectPackage := loader.PackageInfoFor(*rootPath)
	filterEnv := filter.NewFilterEnvironment(projectPackage, loader, isProject)

	allExternalRefs := map[*types.TypeName][]types.Object{}

	// find all type-referencingObject pairs
	for _, pkg := range process.OwnedPackages(loader.Packages(), projectPackage.BuildInfo.ImportPath, isProject) {
		log := log.WithValues("import path", pkg.BuildInfo.ImportPath)
		log.V(1).Info("considering package")

		// for all objects in each of those
		for _, obj := range process.AllObjects(pkg) {
			// find the external references
			externalRefs := filter.FindExternalReferences(obj, filterEnv)
			if len(externalRefs) == 0 {
				continue
			}
			log := log.WithValues("object", obj)
			for _, ref := range externalRefs {
				// and add them to the set
				pkgInfo := loader.PackageInfoFor(ref.Pkg().Path())
				pkgPath := filterEnv.StripVendor(pkgInfo.BuildInfo.ImportPath)
				log.V(1).Info("external reference found", "package", pkgPath)
				allExternalRefs[ref] = append(allExternalRefs[ref], obj)
			}
		}
	}

	return allExternalRefs, filterEnv
}
