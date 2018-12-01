package locate

import (
	"sync"
	"path/filepath"
	"fmt"

	"go/ast"
	"go/parser"
	"go/token"
	"go/build"
	"go/types"
	"go/importer"

	"github.com/go-logr/logr"
)

type PackageInfo struct {
	BuildInfo *build.Package
	Parsed *types.Package
}

// FileLoader knows loads normal Go files from a package,
// or from stdin, and turns them into file ASTs, feeding those
// into a channel for later use.
type FileLoader struct {
	fset *token.FileSet
	packages map[string]*PackageInfo

	mainImporter types.ImporterFrom
	cwd string

	log logr.Logger
}

func NewLoader(cwd string, log logr.Logger) *FileLoader {
	return &FileLoader{
		fset: token.NewFileSet(),
		packages: make(map[string]*PackageInfo),
		mainImporter: importer.For("gc", nil).(types.ImporterFrom),
		cwd: cwd,
		log: log.WithName("loader"),
	}
}

func (l *FileLoader) FileSet() *token.FileSet {
	return l.fset
}

// parse parses the given file source with the given name,
// returning a file AST.  parse may be called concurrently
func (l *FileLoader) parse(filename string, src interface{}) (*ast.File, error) {
	return parser.ParseFile(l.fset, filename, src, 0) // ok to access fset concurrently
}

// getPackageFiles gets the list of files belonging to a single package
func (l *FileLoader) getFilesFromPackage(path string) ([]*ast.File, error) {
	ctxt := build.Default
	packageInfo, err := ctxt.ImportDir(path, 0)
	if _, noGoPresent := err.(*build.NoGoError); err != nil && !noGoPresent {
		// try the main importer (builtins, etc)
		return nil, err
	}

	files := make([]*ast.File, len(packageInfo.GoFiles))
	errors := make([]error, len(packageInfo.GoFiles))

	var wg sync.WaitGroup
	for i, filename := range packageInfo.GoFiles {
		wg.Add(1)
		go func(i int, filepath string) {
			defer wg.Done()
			files[i], errors[i] = l.parse(filepath, nil)
		}(i, filepath.Join(path, filename))
	}
	wg.Wait()

	// if there are errors, return the first one for deterministic results
	for _, err := range errors {
		if err != nil {
			return nil, err
		}
	}

	return files, nil
}

func (l *FileLoader) Import(path string) (*types.Package, error) {
	return l.ImportFrom(path, "", 0)
}

// Import implements types.Importer
func (l *FileLoader) ImportFrom(path, dir string, mode types.ImportMode) (*types.Package, error) {
	// TODO(directxman12): we probably need the key to be {dir,path}, not just path
	pkg, present := l.packages[path]
	if present {
		if pkg == nil {
			return nil, fmt.Errorf("cycle on package %q", path)
		}

		return pkg.Parsed, nil
	}

	var parsedPkg *types.Package

	buildInfo, err := build.Default.Import(path, filepath.Join(l.cwd, dir), build.FindOnly)
	if err != nil {
		return nil, err
	}
	if buildInfo.Goroot {
		l.log.V(1).Info("falling back to built-in importer for builtin package", "package path", path)
		parsedPkg, err = l.mainImporter.ImportFrom(path, dir, mode)
		if err != nil {
			return nil, err
		}
	}

	// cache built-ins, too
	if parsedPkg != nil {
		l.packages[path] = &PackageInfo{
			BuildInfo: buildInfo,
			Parsed: parsedPkg,
		}
		return parsedPkg, nil
	}

	l.packages[path] = nil  // put an explicit placeholder in order to detect loops

	// TODO: deal with if dir is ""
	files, err := l.getFilesFromPackage(buildInfo.Dir)
	if err != nil {
		return nil, err
	}

	conf := types.Config{
		IgnoreFuncBodies: true,
		FakeImportC:      true,
		Importer:         l,
	}
	parsedPkg, err = conf.Check(path, l.fset, files, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to typecheck package %q: %v", path, err)
	}

	// actually set the cache
	l.packages[path] = &PackageInfo{
		BuildInfo: buildInfo,
		Parsed: parsedPkg,
	}

	return parsedPkg, nil
}

// Packages returns all packages known to this FileLoader.
func (l *FileLoader) Packages() []*PackageInfo {
	pkgs := make([]*PackageInfo, 0, len(l.packages))

	for _, pkg := range l.packages {
		pkgs = append(pkgs, pkg)
	}

	return pkgs
}

// PackageInfoFor returns the computed package information for the given package.
func (l *FileLoader) PackageInfoFor(pkg string) *PackageInfo {
	return l.packages[pkg]
}

// FetchPackageInfoFor returns the computed package information for the given package,
// or loads and saves basic information if not found (returning an error if do so failed).
func (l *FileLoader) FetchPackageInfoFor(pkg string) (*PackageInfo, error) {
	info, ok := l.packages[pkg]
	if !ok {
		buildInfo, err := build.Default.Import(pkg, l.cwd, build.FindOnly)
		if err != nil {
			return nil, err
		}
		info = &PackageInfo{
			BuildInfo: buildInfo,
		}
		l.packages[pkg] = info
	}

	return info, nil
}
