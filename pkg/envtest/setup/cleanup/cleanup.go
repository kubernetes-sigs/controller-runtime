package cleanup

import (
	"context"
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/env"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/store"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/versions"
)

// Result is a list of version-platform pairs that were removed from the store.
type Result []store.Item

// Cleanup removes binary packages from disk for all version-platform pairs that match the parameters
//
// Note that both the item collection and the error might be non-nil, if some packages were successfully
// removed (they will be listed in the first return value) and some failed (the errors will be collected
// in the second).
func Cleanup(ctx context.Context, spec versions.Spec, options ...Option) (Result, error) {
	cfg := configure(options...)

	env, err := env.New(cfg.envOpts...)
	if err != nil {
		return nil, err
	}

	if err := env.Store.Initialize(ctx); err != nil {
		return nil, err
	}

	items, err := env.Store.Remove(ctx, store.Filter{Version: spec, Platform: cfg.platform})
	if errors.Is(err, store.ErrUnableToList) {
		return nil, err
	}

	// store.Remove returns an error if _any_ item failed to be removed,
	// but it also reports any items that were removed without errors.
	// Therefore, both items and err might be non-nil at the same time.
	return items, err
}
