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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type (
	// ConsistencyCache is the cache the consistent client records its writes in.
	ConsistencyCache = cache
	// KeyLock is the per key lock the consistent client serializes on.
	KeyLock = keyLock
	// KeyLocker is the implementation of KeyLock the client uses
	KeyLocker = keyLocker
)

// UpstreamClient is what NewConsistentClient wraps. It mirrors the unexported
// upstreamClient interface, but with an exported delete so that it can be
// implemented outside of this package.
type UpstreamClient interface {
	Client

	DeleteWithResult(ctx context.Context, obj Object, opts ...DeleteOption) (*unstructured.Unstructured, error)
}

type upstreamClientShim struct {
	UpstreamClient
}

func (u upstreamClientShim) delete(ctx context.Context, obj Object, opts ...DeleteOption) (*unstructured.Unstructured, error) {
	return u.DeleteWithResult(ctx, obj, opts...)
}

// NewConsistentClient constructs a consistent client on top of an arbitrary upstream.
// Passing a nil newKeyLock uses the same locks the production code uses.
func NewConsistentClient(upstream UpstreamClient, c ConsistencyCache, newKeyLock func() KeyLock) Client {
	return newConsistentClient(upstreamClientShim{upstream}, c, newKeyLock)
}

// NewKeyLocker returns the KeyLock implementation used in production.
func NewKeyLocker() KeyLock {
	return &keyLocker{}
}
