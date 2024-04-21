package setup

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/cleanup"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/list"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/sideload"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/use"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/versions"
)

// List implements the list workflow for listing local and remote versions
func List(ctx context.Context, version string, options ...list.Option) ([]list.Result, error) {
	spec, err := readSpec(version)
	if err != nil {
		return nil, err
	}

	return list.List(ctx, spec, options...)
}

// Use implements the use workflow for selecting and using a version of the environment.
//
// It will download a remote version if required (and options allow), and return the path to the binary asset directory.
func Use(ctx context.Context, version string, options ...use.Option) (use.Result, error) {
	spec, err := readSpec(version)
	if err != nil {
		return use.Result{}, err
	}

	return use.Use(ctx, spec, options...)
}

// Cleanup implements the cleanup workflow for removing versions of the environment.
func Cleanup(ctx context.Context, version string, options ...cleanup.Option) (cleanup.Result, error) {
	spec, err := readSpec(version)
	if err != nil {
		return cleanup.Result{}, err
	}

	return cleanup.Cleanup(ctx, spec, options...)
}

// Sideload reads a binary package from an input stream, and stores it where Use can find it
func Sideload(ctx context.Context, version string, options ...sideload.Option) error {
	spec, err := readSpec(version)
	if err != nil {
		return err
	}

	return sideload.Sideload(ctx, spec, options...)
}

func readSpec(version string) (versions.Spec, error) {
	switch version {
	case "", "latest":
		return versions.LatestVersion, nil
	case "latest-on-disk":
		return versions.AnyVersion, nil
	default:
		v, err := versions.FromExpr(version)
		if err != nil {
			return versions.Spec{}, fmt.Errorf("version must be a valid version, or simply 'latest' or 'latest-on-disk', but got %q: %w", version, err)
		}
		return v, nil
	}
}
