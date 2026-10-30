/*
Copyright The Kubernetes Authors.

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

package client

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache/cacheapi"
	"sigs.k8s.io/controller-runtime/pkg/client/internal/writebarrier"
)

// The purpose of this file is purely to export a few private identifiers from package
// client into package client_test. This is needed because the consistent client tests
// must be in package client_test to avoid an import cycle, since they import the fake
// client and the interceptor, both of which import pkg/client.

// UpstreamClient is what NewConsistentClient wraps. It mirrors the unexported
// upstreamClient interface, but with an exported delete so that it can be
// implemented outside of this package.
type UpstreamClient interface {
	Client

	DeleteWithResult(ctx context.Context, obj Object, opts ...DeleteOption) (runtime.Object, error)
}

type upstreamClientShim struct {
	UpstreamClient
}

func (u upstreamClientShim) delete(ctx context.Context, obj Object, opts ...DeleteOption) (runtime.Object, error) {
	return u.DeleteWithResult(ctx, obj, opts...)
}

// NewConsistentClient constructs a consistent client on top of an arbitrary upstream.
func NewConsistentClient(
	upstream UpstreamClient,
	informers cacheapi.Informers,
	newWriteBarrier func() writebarrier.WriteBarrier,
) Client {
	return newConsistentClient(upstreamClientShim{upstream}, informers, newWriteBarrier, logr.Discard())
}
