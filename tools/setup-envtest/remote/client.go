/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package remote

import (
	"context"
	"io"

	"sigs.k8s.io/controller-runtime/tools/setup-envtest/versions"
)

// Client is an interface to get and list envtest binary archives.
type Client interface {
	ListVersions(ctx context.Context) ([]versions.Set, error)

	GetVersion(ctx context.Context, version versions.Concrete, platform versions.PlatformItem, out io.Writer) error

	FetchSum(ctx context.Context, ver versions.Concrete, pl *versions.PlatformItem) error
}
