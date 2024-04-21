package list

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/env"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/store"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/versions"
)

// Status indicates whether a version is available locally or remotely
type Status string

const (
	// Installed indicates that this version is installed on the local system
	Installed Status = "installed"
	// Available indicates that this version is available to download
	Available Status = "available"
)

// Result encapsulates a single item in the list of versions
type Result struct {
	Version  versions.Concrete
	Platform versions.Platform
	Status   Status
}

// List lists available versions, local and remote
func List(ctx context.Context, version versions.Spec, options ...Option) ([]Result, error) {
	cfg := configure(options...)
	env, err := env.New(cfg.envOpts...)
	if err != nil {
		return nil, err
	}

	if err := env.Store.Initialize(ctx); err != nil {
		return nil, err
	}

	vs, err := env.Store.List(ctx, store.Filter{Version: version, Platform: cfg.platform})
	if err != nil {
		return nil, fmt.Errorf("list installed versions: %w", err)
	}

	results := make([]Result, 0, len(vs))
	for _, v := range vs {
		results = append(results, Result{Version: v.Version, Platform: v.Platform, Status: Installed})
	}

	if cfg.localOnly {
		return results, nil
	}

	remoteVersions, err := env.Client.ListVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list available versions: %w", err)
	}

	for _, set := range remoteVersions {
		if !version.Matches(set.Version) {
			continue
		}
		slices.SortFunc(set.Platforms, func(a, b versions.PlatformItem) int {
			return cmp.Or(cmp.Compare(a.OS, b.OS), cmp.Compare(a.Arch, b.Arch))
		})
		for _, plat := range set.Platforms {
			if cfg.platform.Matches(plat.Platform) {
				results = append(results, Result{
					Version:  set.Version,
					Platform: plat.Platform,
					Status:   Available,
				})
			}
		}
	}

	return results, nil
}
