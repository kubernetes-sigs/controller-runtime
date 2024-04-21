package env

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/versions"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var expectedExecutables = []string{
	"kube-apiserver",
	"etcd",
	"kubectl",
}

// TryUseAssetsFromPath attempts to use the assets from the provided path if they match the spec.
// If they do not, or if some executable is missing, it returns an empty string.
func (e *Env) TryUseAssetsFromPath(ctx context.Context, spec versions.Spec, path string) (versions.Spec, bool) {
	v, err := versions.FromPath(path)
	if err != nil {
		ok, checkErr := e.hasAllExecutables(path)
		log.FromContext(ctx).Info("has all executables", "ok", ok, "err", checkErr)
		if checkErr != nil {
			log.FromContext(ctx).Error(errors.Join(err, checkErr), "Failed checking if assets path has all binaries, ignoring", "path", path)
			return versions.Spec{}, false
		} else if ok {
			// If the path has all executables, we can use it even if we can't parse the version.
			// The user explicitly asked for this path, so set the version to a wildcard so that
			// it passes checks downstream.
			return versions.AnyVersion, true
		}

		log.FromContext(ctx).Error(errors.Join(err, errors.New("some required binaries missing")), "Unable to use assets from path, ignoring", "path", path)
		return versions.Spec{}, false
	}

	if !spec.Matches(*v) {
		log.FromContext(ctx).Error(nil, "Assets path does not match spec, ignoring", "path", path, "spec", spec)
		return versions.Spec{}, false
	}

	if ok, err := e.hasAllExecutables(path); err != nil {
		log.FromContext(ctx).Error(err, "Failed checking if assets path has all binaries, ignoring", "path", path)
		return versions.Spec{}, false
	} else if !ok {
		log.FromContext(ctx).Error(nil, "Assets path is missing some executables, ignoring", "path", path)
		return versions.Spec{}, false
	}

	return versions.Spec{Selector: v}, true
}

func (e *Env) hasAllExecutables(path string) (bool, error) {
	for _, expected := range expectedExecutables {
		_, err := e.FS.Open(filepath.Join(path, expected))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return false, nil
			}
			return false, fmt.Errorf("check for existence of %s binary in %s: %w", expected, path, err)
		}
	}

	return true, nil
}
