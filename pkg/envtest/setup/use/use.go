package use

import (
	"context"
	"errors"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/env"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/store"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/versions"
)

// Result summarizes the output of the Use workflow
type Result struct {
	Version  versions.Spec
	Platform versions.Platform
	Hash     *versions.Hash
	Path     string
}

// ErrNoMatchingVersion is returned when the spec matches no available
// version; available is defined both by versions being published at all,
// but also by other options such as NoDownload.
var ErrNoMatchingVersion = errors.New("no matching version found")

// Use selects an appropriate version based on the user's spec, downloads it if needed,
// and returns the path to the binary asset directory.
func Use(ctx context.Context, version versions.Spec, options ...Option) (Result, error) {
	cfg := configure(options...)

	env, err := env.New(cfg.envOpts...)
	if err != nil {
		return Result{}, err
	}

	if cfg.assetPath != "" {
		if v, ok := env.TryUseAssetsFromPath(ctx, version, cfg.assetPath); ok {
			return Result{
				Version:  v,
				Platform: cfg.platform,
				Path:     cfg.assetPath,
			}, nil
		}
	}

	selectedLocal, err := env.SelectLocalVersion(ctx, version, cfg.platform)
	if err != nil {
		return Result{}, err
	}

	if cfg.noDownload {
		if selectedLocal != (store.Item{}) {
			return toResult(env, selectedLocal, nil), nil
		}

		return Result{}, fmt.Errorf("%w: no local version matching %s found, but you specified NoDownload()", ErrNoMatchingVersion, version)
	}

	if !cfg.forceDownload && !version.CheckLatest && selectedLocal != (store.Item{}) {
		return toResult(env, selectedLocal, nil), nil
	}

	selectedVersion, selectedPlatform, err := env.SelectRemoteVersion(ctx, version, cfg.platform)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrNoMatchingVersion, err)
	}

	if selectedLocal != (store.Item{}) && !selectedVersion.NewerThan(selectedLocal.Version) {
		return Result{
			Path:     env.PathTo(&selectedLocal.Version, selectedLocal.Platform),
			Version:  versions.Spec{Selector: selectedLocal.Version},
			Platform: selectedLocal.Platform,
		}, nil
	}

	if err := env.FetchRemoteVersion(ctx, selectedVersion, selectedPlatform, cfg.verifySum); err != nil {
		return Result{}, err
	}

	return Result{
		Version:  versions.Spec{Selector: *selectedVersion},
		Platform: selectedPlatform.Platform,
		Path:     env.PathTo(selectedVersion, selectedPlatform.Platform),
		Hash:     selectedPlatform.Hash,
	}, nil
}

func toResult(env *env.Env, item store.Item, hash *versions.Hash) Result {
	return Result{
		Version:  versions.Spec{Selector: item.Version},
		Platform: item.Platform,
		Path:     env.PathTo(&item.Version, item.Platform),
		Hash:     hash,
	}
}
