// SPDX-License-Identifier: Apache-2.0
// Copyright 2024 The Kubernetes Authors

package remote

import (
	"context"
	"errors"
	"io"

	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/versions"
)

// ErrChecksumMismatch is returned when the checksum of the downloaded archive does not match the expected checksum.
var ErrChecksumMismatch = errors.New("checksum mismatch")

// Client is an interface to get and list envtest binary archives.
type Client interface {
	ListVersions(ctx context.Context) ([]versions.Set, error)

	GetVersion(ctx context.Context, version versions.Concrete, platform versions.PlatformItem, out io.Writer) error

	FetchSum(ctx context.Context, ver versions.Concrete, pl versions.Platform) (*versions.Hash, error)

	LatestVersion(ctx context.Context, spec versions.Spec, platform versions.Platform) (versions.Concrete, error)
}
