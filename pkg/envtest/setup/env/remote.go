package env

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/remote"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/store"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/versions"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// SelectRemoteVersion finds the latest remote version that matches the provided spec and platform.
func (e *Env) SelectRemoteVersion(ctx context.Context, spec versions.Spec, platform versions.Platform) (*versions.Concrete, versions.PlatformItem, error) {
	vs, err := e.Client.ListVersions(ctx)
	if err != nil {
		return nil, versions.PlatformItem{}, err
	}

	for _, v := range vs {
		if !spec.Matches(v.Version) {
			log.FromContext(ctx).V(1).Info("skipping non-matching version", "version", v.Version)
			continue
		}

		for _, p := range v.Platforms {
			if platform.Matches(p.Platform) {
				// copy to avoid holding on to the entire slice
				ver := v.Version
				return &ver, p, nil
			}
		}

		plats := make([]versions.Platform, 0)
		for _, p := range v.Platforms {
			plats = append(plats, p.Platform)
		}

		log.FromContext(ctx).Info("version not available for your platform; skipping", "version", v.Version, "platforms", plats)
	}

	return nil, versions.PlatformItem{}, fmt.Errorf("no applicable packages found for version %s and platform %s", spec, platform)
}

// FetchRemoteVersion downloads the specified version and platform binaries and extracts them to the appropriate path
//
// If verifySum is true, it will also fetch the md5 sum for the version and platform and check the hashsum of the downloaded archive.
func (e *Env) FetchRemoteVersion(ctx context.Context, version *versions.Concrete, platform versions.PlatformItem, verifySum bool) error {
	if verifySum && platform.Hash == nil {
		hash, err := e.FetchSum(ctx, *version, platform.Platform)
		if err != nil {
			return fmt.Errorf("fetch md5 sum for version %s, platform %s: %w", version, platform.Platform, err)
		}

		platform.Hash = hash
	} else if !verifySum {
		// turn off checksum verification
		platform.Hash = nil
	}

	_, useGCS := e.Client.(*remote.GCSClient) //nolint:staticcheck
	archiveOut, err := os.CreateTemp("", "*-"+platform.ArchiveName(useGCS, *version))
	if err != nil {
		return fmt.Errorf("open temporary download location: %w", err)
	}
	// cleanup defer needs to be the first one defined, so it's the last one to run
	packedPath := ""
	defer func() {
		if packedPath != "" {
			if err := os.Remove(packedPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				log.FromContext(ctx).V(1).Error(err, "Unable to clean up %s", packedPath)
			}
		}
	}()
	defer archiveOut.Close()

	packedPath = archiveOut.Name()
	if err := e.Client.GetVersion(ctx, *version, platform, archiveOut); err != nil {
		return fmt.Errorf("download archive: %w", err)
	}

	if err := archiveOut.Sync(); err != nil {
		return fmt.Errorf("flush downloaded file: %w", err)
	}

	if _, err := archiveOut.Seek(0, 0); err != nil {
		return fmt.Errorf("jump to start of archive: %w", err)
	}

	if err := e.Store.Add(ctx, store.Item{Version: *version, Platform: platform.Platform}, archiveOut); err != nil {
		return fmt.Errorf("store version to disk: %w", err)
	}

	return nil
}
