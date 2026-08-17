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

package readerconsistency

import (
	"context"
	"testing"
	"testing/synctest"

	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func configMap(key client.ObjectKey, uid types.UID, rv string) *corev1.ConfigMap {
	return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Namespace:       key.Namespace,
		Name:            key.Name,
		UID:             uid,
		ResourceVersion: rv,
	}}
}

func TestWaitForGetWaitsForMinRV(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := NewWithT(t)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		h := NewHandler(func(int64) {}, logr.Discard())
		key := client.ObjectKey{Namespace: "default", Name: "foo"}

		result := make(chan error, 1)
		go func() { result <- h.WaitForGet(ctx, key, 10) }()

		synctest.Wait()
		g.Expect(result).NotTo(Receive())

		h.OnAdd(configMap(key, "uid-1", "9"), false)
		synctest.Wait()
		g.Expect(result).NotTo(Receive())

		h.OnAdd(configMap(key, "uid-1", "10"), false)
		synctest.Wait()
		g.Expect(result).To(Receive(BeNil()))
	})
}

func TestWaitForGetReturnsContextErrorIfMinRVIsNeverObserved(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := NewWithT(t)
		ctx, cancel := context.WithCancel(t.Context())

		h := NewHandler(func(int64) {}, logr.Discard())
		key := client.ObjectKey{Namespace: "default", Name: "foo"}

		result := make(chan error, 1)
		go func() { result <- h.WaitForGet(ctx, key, 10) }()

		synctest.Wait()
		g.Expect(result).NotTo(Receive())

		cancel()
		synctest.Wait()
		g.Expect(result).To(Receive(MatchError(context.Canceled)))
	})
}

func TestWaitForGetWaitsForPendingDeletesOfItsKeyOnly(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := NewWithT(t)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		h := NewHandler(func(int64) {}, logr.Discard())
		key := client.ObjectKey{Namespace: "default", Name: "foo"}
		otherKey := client.ObjectKey{Namespace: "default", Name: "bar"}

		h.AddPendingDelete(key, "uid-1")
		h.AddPendingDelete(key, "uid-2")
		h.AddPendingDelete(otherKey, "uid-3")

		result := make(chan error, 1)
		go func() { result <- h.WaitForGet(ctx, key, 0) }()
		synctest.Wait()

		g.Expect(result).NotTo(Receive())

		h.OnDelete(configMap(key, "uid-1", "10"))
		synctest.Wait()
		g.Expect(result).NotTo(Receive())

		h.OnDelete(configMap(key, "uid-2", "11"))
		synctest.Wait()
		g.Expect(result).To(Receive(BeNil()))
	})
}

func TestWaitForGetDoesNotWaitForPendingDeletesAddedAfterItWasCalled(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := NewWithT(t)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		h := NewHandler(func(int64) {}, logr.Discard())
		key := client.ObjectKey{Namespace: "default", Name: "foo"}

		h.AddPendingDelete(key, "uid-1")

		result := make(chan error, 1)
		go func() { result <- h.WaitForGet(ctx, key, 0) }()
		synctest.Wait()

		g.Expect(result).NotTo(Receive())

		h.AddPendingDelete(key, "uid-2")
		h.OnDelete(configMap(key, "uid-1", "10"))
		synctest.Wait()
		g.Expect(result).To(Receive(BeNil()))
	})
}

func TestWaitForListWaitsForMinRV(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := NewWithT(t)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		h := NewHandler(func(int64) {}, logr.Discard())
		key := client.ObjectKey{Namespace: "default", Name: "foo"}

		result := make(chan error, 1)
		go func() { result <- h.WaitForList(ctx, 10) }()
		synctest.Wait()

		g.Expect(result).NotTo(Receive())

		h.OnAdd(configMap(key, "uid-1", "9"), false)
		synctest.Wait()
		g.Expect(result).NotTo(Receive())

		h.OnAdd(configMap(key, "uid-1", "10"), false)
		synctest.Wait()
		g.Expect(result).To(Receive(BeNil()))
	})
}

func TestWaitForListReturnsContextErrorIfMinRVIsNeverObserved(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := NewWithT(t)
		ctx, cancel := context.WithCancel(t.Context())

		h := NewHandler(func(int64) {}, logr.Discard())

		result := make(chan error, 1)
		go func() { result <- h.WaitForList(ctx, 10) }()
		synctest.Wait()

		g.Expect(result).NotTo(Receive())

		cancel()
		synctest.Wait()
		g.Expect(result).To(Receive(MatchError(context.Canceled)))
	})
}

func TestWaitForListWaitsForPendingDeletesOfAllKeys(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := NewWithT(t)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		h := NewHandler(func(int64) {}, logr.Discard())
		key := client.ObjectKey{Namespace: "default", Name: "foo"}
		otherKey := client.ObjectKey{Namespace: "other", Name: "bar"}

		h.AddPendingDelete(key, "uid-1")
		h.AddPendingDelete(otherKey, "uid-2")

		result := make(chan error, 1)
		go func() { result <- h.WaitForList(ctx, 0) }()

		synctest.Wait()
		g.Expect(result).NotTo(Receive())

		h.OnDelete(configMap(key, "uid-1", "10"))
		synctest.Wait()
		g.Expect(result).NotTo(Receive())

		h.OnDelete(configMap(otherKey, "uid-2", "11"))
		synctest.Wait()
		g.Expect(result).To(Receive(BeNil()))
	})
}

func TestWaitForListDoesNotWaitForPendingDeletesAddedAfterItWasCalled(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := NewWithT(t)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		h := NewHandler(func(int64) {}, logr.Discard())
		key := client.ObjectKey{Namespace: "default", Name: "foo"}
		otherKey := client.ObjectKey{Namespace: "other", Name: "bar"}

		h.AddPendingDelete(key, "uid-1")

		result := make(chan error, 1)
		go func() { result <- h.WaitForList(ctx, 0) }()
		synctest.Wait()

		g.Expect(result).NotTo(Receive())

		h.AddPendingDelete(otherKey, "uid-2")
		h.OnDelete(configMap(key, "uid-1", "10"))
		synctest.Wait()
		g.Expect(result).To(Receive(BeNil()))
	})
}

func TestWaitForListIsUnblockedByRemovePendingDelete(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := NewWithT(t)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		h := NewHandler(func(int64) {}, logr.Discard())
		key := client.ObjectKey{Namespace: "default", Name: "foo"}

		h.AddPendingDelete(key, "uid-1")

		result := make(chan error, 1)
		go func() { result <- h.WaitForList(ctx, 0) }()

		synctest.Wait()
		g.Expect(result).NotTo(Receive())

		h.RemovePendingDelete(key, "uid-1")
		synctest.Wait()
		g.Expect(result).To(Receive(BeNil()))
	})
}

func TestWaitForGetIsUnblockedByRemovePendingDelete(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := NewWithT(t)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		h := NewHandler(func(int64) {}, logr.Discard())
		key := client.ObjectKey{Namespace: "default", Name: "foo"}

		h.AddPendingDelete(key, "uid-1")

		result := make(chan error, 1)
		go func() { result <- h.WaitForGet(ctx, key, 0) }()

		synctest.Wait()
		g.Expect(result).NotTo(Receive())

		h.RemovePendingDelete(key, "uid-1")
		synctest.Wait()
		g.Expect(result).To(Receive(BeNil()))
	})
}
