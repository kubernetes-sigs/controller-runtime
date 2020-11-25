/*
Copyright 2020 The Kubernetes Authors.

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
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
)

var (
	targetGVK = schema.GroupVersionKind{Group: "test.kubebuilder.io", Version: "v1beta1", Kind: "SomeCR"}
)

func TestSource(t *testing.T) {
	RegisterFailHandler(Fail)
	suiteName := "Cache Internal Test Suite"
	RunSpecsWithDefaultAndCustomReporters(t, suiteName, []Reporter{printer.NewlineReporter{}, printer.NewProwReporter(suiteName)})
}

var _ = Describe("Cache reader", func() {
	Describe("Disable deep copy", func() {
		var c *CacheReader
		BeforeEach(func() {
			c = &CacheReader{groupVersionKind: targetGVK}
		})

		objWithoutGVK := &unstructured.Unstructured{}
		objWithGVK := &unstructured.Unstructured{}
		objWithGVK.SetGroupVersionKind(targetGVK)

		It("should deep copy for object without gvk if listOptions not set disableDeepCopy", func() {
			listOps := client.ListOptions{}
			Expect(c.disableDeepCopy(objWithoutGVK, listOps)).To(Equal(false))
		})

		It("should deep copy for object with gvk if listOptions not set disableDeepCopy", func() {
			listOps := client.ListOptions{}
			Expect(c.disableDeepCopy(objWithGVK, listOps)).To(Equal(false))
		})

		It("should deep copy for object without gvk if listOptions set disableDeepCopy and IgnoreGVK is false", func() {
			listOps := client.ListOptions{UnsafeDisableCacheDeepCopy: &client.UnsafeDisableCacheDeepCopy{}}
			Expect(c.disableDeepCopy(objWithoutGVK, listOps)).To(Equal(false))
		})

		It("should not deep copy for object with gvk if listOptions set disableDeepCopy and IgnoreGVK is false", func() {
			listOps := client.ListOptions{UnsafeDisableCacheDeepCopy: &client.UnsafeDisableCacheDeepCopy{}}
			Expect(c.disableDeepCopy(objWithGVK, listOps)).To(Equal(true))
		})

		It("should not deep copy for object without gvk if listOptions set disableDeepCopy and IgnoreGVK is true", func() {
			listOps := client.ListOptions{UnsafeDisableCacheDeepCopy: &client.UnsafeDisableCacheDeepCopy{IgnoreGVK: true}}
			Expect(c.disableDeepCopy(objWithoutGVK, listOps)).To(Equal(true))
		})

		It("should not deep copy for object with gvk if listOptions set disableDeepCopy and IgnoreGVK is true", func() {
			listOps := client.ListOptions{UnsafeDisableCacheDeepCopy: &client.UnsafeDisableCacheDeepCopy{IgnoreGVK: true}}
			Expect(c.disableDeepCopy(objWithGVK, listOps)).To(Equal(true))
		})
	})
})
