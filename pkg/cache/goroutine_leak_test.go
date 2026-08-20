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

package cache

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// mockCache is a mock implementation of Cache for testing
type mockCache struct {
	startFunc func(ctx context.Context) error
}

func (m *mockCache) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return nil
}
func (m *mockCache) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return nil
}
func (m *mockCache) GetInformer(ctx context.Context, obj client.Object, opts ...InformerGetOption) (Informer, error) {
	return nil, nil
}
func (m *mockCache) GetInformerForKind(ctx context.Context, gvk schema.GroupVersionKind, opts ...InformerGetOption) (Informer, error) {
	return nil, nil
}
func (m *mockCache) RemoveInformer(ctx context.Context, obj client.Object) error {
	return nil
}
func (m *mockCache) Start(ctx context.Context) error {
	if m.startFunc != nil {
		return m.startFunc(ctx)
	}
	<-ctx.Done()
	return nil
}
func (m *mockCache) WaitForCacheSync(ctx context.Context) bool {
	return true
}
func (m *mockCache) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	return nil
}

// TestMultiNamespaceCacheStart_GoroutineLeak_MultipleErrors tests that goroutines
// don't leak when multiple caches return errors simultaneously
func TestMultiNamespaceCacheStart_GoroutineLeak_MultipleErrors(t *testing.T) {
	defer goleak.VerifyNone(t)

	// Create a multiNamespaceCache with multiple caches that all return errors
	errTest := errors.New("test error")
	namespaceToCache := map[string]Cache{
		"ns1": &mockCache{startFunc: func(ctx context.Context) error { return errTest }},
		"ns2": &mockCache{startFunc: func(ctx context.Context) error { return errTest }},
		"ns3": &mockCache{startFunc: func(ctx context.Context) error { return errTest }},
	}

	c := &multiNamespaceCache{
		namespaceToCache: namespaceToCache,
	}

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	// Start should return an error
	err := c.Start(ctx)
	if err == nil {
		t.Fatal("expected error from Start(), got nil")
	}

	// Give time for goroutines to leak if they will
	time.Sleep(50 * time.Millisecond)
}

// TestMultiNamespaceCacheStart_GoroutineLeak_ContextCancelWithError tests that
// goroutines don't leak when context is cancelled while a cache returns an error
func TestMultiNamespaceCacheStart_GoroutineLeak_ContextCancelWithError(t *testing.T) {
	defer goleak.VerifyNone(t)

	// Create caches where one returns error slowly, others wait for context
	var wg sync.WaitGroup
	slowCacheStarted := make(chan struct{})

	slowCache := &mockCache{
		startFunc: func(ctx context.Context) error {
			close(slowCacheStarted)
			// Wait a bit then return error
			time.Sleep(30 * time.Millisecond)
			return errors.New("slow error")
		},
	}

	normalCache := &mockCache{
		startFunc: func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		},
	}

	namespaceToCache := map[string]Cache{
		"ns1": slowCache,
		"ns2": normalCache,
		"ns3": normalCache,
	}

	c := &multiNamespaceCache{
		namespaceToCache: namespaceToCache,
	}

	// Use a short timeout to trigger context cancellation
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	wg.Go(func() {
		_ = c.Start(ctx)
	})

	// Wait for slow cache to start
	<-slowCacheStarted

	// Wait for Start to return
	wg.Wait()

	// Give time for goroutines to leak if they will
	time.Sleep(50 * time.Millisecond)
}

// TestMultiNamespaceCacheStart_ErrorWithHealthyChildren tests that when one cache
// returns an error, the healthy children are cancelled and Start returns promptly.
// This is the critical test case that verifies errgroup behavior.
func TestMultiNamespaceCacheStart_ErrorWithHealthyChildren(t *testing.T) {
	defer goleak.VerifyNone(t)

	errSentinel := errors.New("sentinel error")
	errorCacheReturned := make(chan struct{})
	healthyChildCancelled := make(chan struct{})

	errorCache := &mockCache{
		startFunc: func(ctx context.Context) error {
			defer close(errorCacheReturned)
			return errSentinel
		},
	}

	healthyCache := &mockCache{
		startFunc: func(ctx context.Context) error {
			<-ctx.Done()
			close(healthyChildCancelled)
			return nil
		},
	}

	namespaceToCache := map[string]Cache{
		"error":   errorCache,
		"healthy": healthyCache,
	}

	c := &multiNamespaceCache{
		namespaceToCache: namespaceToCache,
	}

	// Use WithCancel (not WithTimeout) to simulate production behavior
	// where the manager's context lives for the process lifetime
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- c.Start(ctx)
	}()

	// Wait for error cache to return
	<-errorCacheReturned

	// Start should return promptly with the error
	select {
	case err := <-done:
		if !errors.Is(err, errSentinel) {
			t.Fatalf("expected sentinel error, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start() did not return promptly after error - it hung waiting for healthy children")
	}

	// Healthy child should have been cancelled
	select {
	case <-healthyChildCancelled:
		// Good, healthy child observed cancellation
	case <-time.After(1 * time.Second):
		t.Fatal("healthy child did not observe context cancellation")
	}

	// Give time for goroutines to clean up
	time.Sleep(50 * time.Millisecond)
}

// TestDelegatingByGVKCacheStart_GoroutineLeak_MultipleErrors tests that goroutines
// don't leak when multiple caches return errors simultaneously
func TestDelegatingByGVKCacheStart_GoroutineLeak_MultipleErrors(t *testing.T) {
	defer goleak.VerifyNone(t)

	errTest := errors.New("test error")
	caches := map[schema.GroupVersionKind]Cache{
		{Group: "apps", Version: "v1", Kind: "Deployment"}:  &mockCache{startFunc: func(ctx context.Context) error { return errTest }},
		{Group: "apps", Version: "v1", Kind: "StatefulSet"}: &mockCache{startFunc: func(ctx context.Context) error { return errTest }},
		{Group: "apps", Version: "v1", Kind: "DaemonSet"}:   &mockCache{startFunc: func(ctx context.Context) error { return errTest }},
	}

	c := &delegatingByGVKCache{
		caches:       caches,
		defaultCache: &mockCache{startFunc: func(ctx context.Context) error { return errTest }},
	}

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	err := c.Start(ctx)
	if err == nil {
		t.Fatal("expected error from Start(), got nil")
	}

	time.Sleep(50 * time.Millisecond)
}

// TestDelegatingByGVKCacheStart_GoroutineLeak_ContextCancelWithError tests the
// specific case where wg.Wait() can deadlock
func TestDelegatingByGVKCacheStart_GoroutineLeak_ContextCancelWithError(t *testing.T) {
	defer goleak.VerifyNone(t)

	slowCacheStarted := make(chan struct{})
	slowCache := &mockCache{
		startFunc: func(ctx context.Context) error {
			close(slowCacheStarted)
			time.Sleep(30 * time.Millisecond)
			return errors.New("slow error")
		},
	}

	normalCache := &mockCache{
		startFunc: func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		},
	}

	caches := map[schema.GroupVersionKind]Cache{
		{Group: "apps", Version: "v1", Kind: "Deployment"}:  slowCache,
		{Group: "apps", Version: "v1", Kind: "StatefulSet"}: normalCache,
	}

	c := &delegatingByGVKCache{
		caches:       caches,
		defaultCache: normalCache,
	}

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		_ = c.Start(ctx)
		close(done)
	}()

	<-slowCacheStarted

	// Wait for Start to return with timeout
	select {
	case <-done:
		// Good, Start returned
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Start() deadlocked - wg.Wait() is blocking on leaked goroutines")
	}

	time.Sleep(50 * time.Millisecond)
}

// TestDelegatingByGVKCacheStart_ErrorWithHealthyChildren tests that when one cache
// returns an error, the healthy children are cancelled and Start returns promptly.
func TestDelegatingByGVKCacheStart_ErrorWithHealthyChildren(t *testing.T) {
	defer goleak.VerifyNone(t)

	errSentinel := errors.New("sentinel error")
	errorCacheReturned := make(chan struct{})
	healthyChild1Cancelled := make(chan struct{})
	healthyChild2Cancelled := make(chan struct{})

	errorCache := &mockCache{
		startFunc: func(ctx context.Context) error {
			defer close(errorCacheReturned)
			return errSentinel
		},
	}

	healthyCache1 := &mockCache{
		startFunc: func(ctx context.Context) error {
			<-ctx.Done()
			close(healthyChild1Cancelled)
			return nil
		},
	}

	healthyCache2 := &mockCache{
		startFunc: func(ctx context.Context) error {
			<-ctx.Done()
			close(healthyChild2Cancelled)
			return nil
		},
	}

	caches := map[schema.GroupVersionKind]Cache{
		{Group: "apps", Version: "v1", Kind: "Deployment"}:  errorCache,
		{Group: "apps", Version: "v1", Kind: "StatefulSet"}: healthyCache1,
	}

	c := &delegatingByGVKCache{
		caches:       caches,
		defaultCache: healthyCache2,
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- c.Start(ctx)
	}()

	<-errorCacheReturned

	select {
	case err := <-done:
		if !errors.Is(err, errSentinel) {
			t.Fatalf("expected sentinel error, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start() did not return promptly after error - it hung waiting for healthy children")
	}

	select {
	case <-healthyChild1Cancelled:
		// Good
	case <-time.After(1 * time.Second):
		t.Fatal("healthy child 1 did not observe context cancellation")
	}

	select {
	case <-healthyChild2Cancelled:
		// Good
	case <-time.After(1 * time.Second):
		t.Fatal("healthy child 2 did not observe context cancellation")
	}

	time.Sleep(50 * time.Millisecond)
}
