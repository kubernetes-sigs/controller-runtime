package locate

import (
	"os"
	"path/filepath"
)

// TODO: convert this to just use the code from the Go builtins,
// to get full `...` matching.  See cmd/go/internal/search/search.go

// ImportProject imports the contents of a entire project, assuming
// the project is structured in the standard Kubernetes-style fashion,
// with code under `pkg`.  It returns whether or not this seemed to be
// a "project" like that, as well as any errors.  It can also be forced
// to ignore the lack of a pkg directory, and treat all subdirectories
// as packages (in which case it always treats paths as a "project").
func ImportProject(loader *FileLoader, projectPath string, force bool) (bool, error) {
	startPath := projectPath
	if !force {
		startPath = filepath.Join(projectPath, "pkg")
		if pathInfo, err := os.Stat(startPath); err != nil || !pathInfo.IsDir() {
			if err == nil || os.IsNotExist(err) {
				return false, nil
			}
			return false, err
		}
	}

	return true, filepath.Walk(startPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			// if SkipDir is returned on a non-directory, Walk skips the rest of the files
			// in the current directory.  We don't care about files, so we want to skip
			// them.
			return filepath.SkipDir
		}

		_, err = loader.Import(path)
		return err
	})
}
