package env

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/store"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/versions"
)

// SelectLocalVersion returns the latest local version that matches the provided spec and platform,
// or nil if no such version is available.
//
// Note that a nil error does not guarantee that a version was found!
func (e *Env) SelectLocalVersion(ctx context.Context, spec versions.Spec, platform versions.Platform) (store.Item, error) {
	localVersions, err := e.Store.List(ctx, store.Filter{Version: spec, Platform: platform})
	if err != nil {
		return store.Item{}, err
	}
	// NB(tomasaschan): this assumes the following of the contract for store.List
	// * only results matching the provided filter are returned
	// * they are returned sorted, with the newest version first in the list
	// Within these constraints, if the slice is non-empty, the first item is the one,
	// we want, and there's no need to iterate through the items again.
	if len(localVersions) > 0 {
		// copy to avoid holding on to the entire slice
		result := localVersions[0]
		return result, nil
	}

	return store.Item{}, nil
}

// PathTo returns the local path to the assets directory for the provided version and platform
func (e *Env) PathTo(version *versions.Concrete, platform versions.Platform) string {
	return e.Store.Path(store.Item{Version: *version, Platform: platform})
}
