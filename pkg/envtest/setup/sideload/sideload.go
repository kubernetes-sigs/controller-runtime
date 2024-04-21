package sideload

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/env"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/store"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/versions"
)

// Sideload reads a binary package from an input stream and stores it in the environment store.
func Sideload(ctx context.Context, version versions.Spec, options ...Option) error {
	cfg := configure(options...)

	if !version.IsConcrete() || cfg.platform.IsWildcard() {
		return fmt.Errorf("must specify a concrete version and platform to sideload; got version %s, platform %s", version, cfg.platform)
	}

	env, err := env.New(cfg.envOpts...)
	if err != nil {
		return err
	}

	if err := env.Store.Initialize(ctx); err != nil {
		return err
	}

	log, err := logr.FromContext(ctx)
	if err != nil {
		return err
	}
	log.Info("sideloading from input stream", "version", version, "platform", cfg.platform)
	if err := env.Store.Add(ctx, store.Item{Version: *version.AsConcrete(), Platform: cfg.platform}, cfg.input); err != nil {
		return fmt.Errorf("sideload item to disk: %w", err)
	}

	return nil
}
