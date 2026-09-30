/*
Copyright 2026 The Kubernetes Authors.

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

package internal

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CacheReader", func() {
	var (
		ctx       = context.Background()
		podGVK    = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}
		cachedPod *corev1.Pod
		reader    *CacheReader
	)

	BeforeEach(func() {
		cachedPod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
				Labels:    map[string]string{"app": "test"},
			},
		}
		indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
			cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
		})
		Expect(indexer.Add(cachedPod)).To(Succeed())
		reader = &CacheReader{
			indexer:          indexer,
			groupVersionKind: podGVK,
			scopeName:        apimeta.RESTScopeNameNamespace,
		}
	})

	DescribeTable("sets GVK on Get without writing it into the indexer",
		func(disableDeepCopy bool, getOpts []client.GetOption) {
			reader.disableDeepCopy = disableDeepCopy

			out := &corev1.Pod{}
			Expect(reader.Get(ctx, client.ObjectKeyFromObject(cachedPod), out, getOpts...)).To(Succeed())

			Expect(out.GroupVersionKind()).To(Equal(podGVK))
			Expect(cachedPod.GroupVersionKind()).To(Equal(schema.GroupVersionKind{}))
			Expect(out.Labels).To(Equal(cachedPod.Labels))
		},
		Entry("deep copy enabled", false, nil),
		Entry("reader disables deep copy", true, nil),
		Entry("Get option disables deep copy", false, []client.GetOption{client.UnsafeDisableDeepCopy}),
	)

	DescribeTable("sets GVK on List items without writing it into the indexer",
		func(disableDeepCopy bool, listOpts []client.ListOption) {
			reader.disableDeepCopy = disableDeepCopy

			out := &corev1.PodList{}
			Expect(reader.List(ctx, out, listOpts...)).To(Succeed())

			Expect(out.Items).To(HaveLen(1))
			Expect(out.Items[0].GroupVersionKind()).To(Equal(podGVK))
			Expect(cachedPod.GroupVersionKind()).To(Equal(schema.GroupVersionKind{}))
			Expect(out.Items[0].Labels).To(Equal(cachedPod.Labels))
		},
		Entry("deep copy enabled", false, nil),
		Entry("reader disables deep copy", true, nil),
		Entry("List option disables deep copy", false, []client.ListOption{client.UnsafeDisableDeepCopy}),
	)

	It("keeps nested fields shared when deep copy is disabled", func() {
		reader.disableDeepCopy = true

		got := &corev1.Pod{}
		Expect(reader.Get(ctx, client.ObjectKeyFromObject(cachedPod), got)).To(Succeed())
		got.Labels["mutated"] = "true"
		Expect(cachedPod.Labels).To(HaveKeyWithValue("mutated", "true"))

		listed := &corev1.PodList{}
		Expect(reader.List(ctx, listed, &client.ListOptions{UnsafeDisableDeepCopy: ptr.To(true)})).To(Succeed())
		listed.Items[0].Labels["listed"] = "true"
		Expect(cachedPod.Labels).To(HaveKeyWithValue("listed", "true"))
	})

	It("does not share nested fields when deep copy is enabled", func() {
		got := &corev1.Pod{}
		Expect(reader.Get(ctx, client.ObjectKeyFromObject(cachedPod), got)).To(Succeed())
		got.Labels["mutated"] = "true"
		Expect(cachedPod.Labels).NotTo(HaveKey("mutated"))

		listed := &corev1.PodList{}
		Expect(reader.List(ctx, listed)).To(Succeed())
		listed.Items[0].Labels["listed"] = "true"
		Expect(cachedPod.Labels).NotTo(HaveKey("listed"))
	})
})
