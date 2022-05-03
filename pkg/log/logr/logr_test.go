/*
Copyright 2022 The Kubernetes Authors.

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

package logr

import (
	"bytes"

	"encoding/json"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// testStringer is a fmt.Stringer.
type testStringer struct{}

func (testStringer) String() string {
	return "value"
}

var _ = Describe("Logr logSink setup", func() {

	Context("when logging kubernetes objects", func() {
		var logOut *bytes.Buffer
		var zaplogger logr.Logger
		var logger logr.Logger

		Context("with logger created using zap.New", func() {
			BeforeEach(func() {
				logOut = new(bytes.Buffer)
				By("setting up the logger")
				// use production settings (false) to get just json output
				zaplogger = zap.New(zap.WriteTo(logOut), zap.UseDevMode(false))
				logger = NewKubeAwareLogger(zaplogger, true)

			})

			It("should log a standard namespaced Kubernetes object name and namespace", func() {
				pod := &corev1.Pod{}
				pod.Name = "some-pod"
				pod.Namespace = "some-ns"
				logger.Info("here's a kubernetes object", "thing", pod)

				outRaw := logOut.Bytes()
				res := map[string]interface{}{}
				Expect(json.Unmarshal(outRaw, &res)).To(Succeed())

				Expect(res).To(HaveKeyWithValue("thing", map[string]interface{}{
					"name":      pod.Name,
					"namespace": pod.Namespace,
				}))
			})

			It("should work fine with normal stringers", func() {
				logger.Info("here's a non-kubernetes stringer", "thing", testStringer{})
				outRaw := logOut.Bytes()
				res := map[string]interface{}{}
				Expect(json.Unmarshal(outRaw, &res)).To(Succeed())

				Expect(res).To(HaveKeyWithValue("thing", "value"))
			})

			It("should log a standard non-namespaced Kubernetes object name", func() {
				node := &corev1.Node{}
				node.Name = "some-node-1"
				logger.Info("here's a kubernetes object", "thing", node)

				outRaw := logOut.Bytes()
				res := map[string]interface{}{}
				Expect(json.Unmarshal(outRaw, &res)).To(Succeed())

				Expect(res).To(HaveKeyWithValue("thing", map[string]interface{}{
					"name": node.Name,
				}))
			})

			It("should log a standard Kubernetes object's kind, if set", func() {
				node := &corev1.Node{}
				node.Name = "some-node-2"
				node.APIVersion = "v1"
				node.Kind = "Node"
				logger.Info("here's a kubernetes object", "thing", node)

				outRaw := logOut.Bytes()
				res := map[string]interface{}{}
				Expect(json.Unmarshal(outRaw, &res)).To(Succeed())

				Expect(res).To(HaveKeyWithValue("thing", map[string]interface{}{
					"name":       node.Name,
					"apiVersion": "v1",
					"kind":       "Node",
				}))
			})

			It("should log a standard non-namespaced NamespacedName name", func() {
				name := types.NamespacedName{Name: "some-node-3"}
				logger.Info("here's a kubernetes object", "thing", name)

				outRaw := logOut.Bytes()
				res := map[string]interface{}{}
				Expect(json.Unmarshal(outRaw, &res)).To(Succeed())

				Expect(res).To(HaveKeyWithValue("thing", map[string]interface{}{
					"name": name.Name,
				}))
			})

			It("should log an unstructured Kubernetes object", func() {
				pod := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"name":      "some-pod",
							"namespace": "some-ns",
						},
					},
				}
				logger.Info("here's a kubernetes object", "thing", pod)

				outRaw := logOut.Bytes()
				res := map[string]interface{}{}
				Expect(json.Unmarshal(outRaw, &res)).To(Succeed())

				Expect(res).To(HaveKeyWithValue("thing", map[string]interface{}{
					"name":      "some-pod",
					"namespace": "some-ns",
				}))
			})

			It("should log a standard namespaced NamespacedName name and namespace", func() {
				name := types.NamespacedName{Name: "some-pod", Namespace: "some-ns"}
				logger.Info("here's a kubernetes object", "thing", name)

				outRaw := logOut.Bytes()
				res := map[string]interface{}{}
				Expect(json.Unmarshal(outRaw, &res)).To(Succeed())

				Expect(res).To(HaveKeyWithValue("thing", map[string]interface{}{
					"name":      name.Name,
					"namespace": name.Namespace,
				}))
			})

			It("should not panic with nil obj", func() {
				var pod *corev1.Pod
				logger.Info("here's a kubernetes object", "thing", pod)
				outRaw := logOut.Bytes()
				res := map[string]interface{}{}
				Expect(json.Unmarshal(outRaw, &res)).To(Succeed())
				Expect(res).To(HaveKey("thing"))
				Expect(res["things"]).To(BeNil())
			})

			It("should log a standard namespaced when using logrLogger.WithValues", func() {
				name := types.NamespacedName{Name: "some-pod", Namespace: "some-ns"}
				logger.WithValues("thing", name).Info("here's a kubernetes object")

				outRaw := logOut.Bytes()
				res := map[string]interface{}{}
				Expect(json.Unmarshal(outRaw, &res)).To(Succeed())

				Expect(res).To(HaveKeyWithValue("thing", map[string]interface{}{
					"name":      name.Name,
					"namespace": name.Namespace,
				}))
			})

			It("should log a standard Kubernetes objects when using logrLogger.WithValues", func() {
				node := &corev1.Node{}
				node.Name = "some-node"
				node.APIVersion = "v1"
				node.Kind = "Node"
				logger.WithValues("thing", node).Info("here's a kubernetes object")

				outRaw := logOut.Bytes()
				res := map[string]interface{}{}
				Expect(json.Unmarshal(outRaw, &res)).To(Succeed())

				Expect(res).To(HaveKeyWithValue("thing", map[string]interface{}{
					"name":       node.Name,
					"apiVersion": "v1",
					"kind":       "Node",
				}))
			})

			It("should log a standard unstructured Kubernetes object when using logrLogger.WithValues", func() {
				pod := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"name":      "some-pod",
							"namespace": "some-ns",
						},
					},
				}
				logger.WithValues("thing", pod).Info("here's a kubernetes object")

				outRaw := logOut.Bytes()
				res := map[string]interface{}{}
				Expect(json.Unmarshal(outRaw, &res)).To(Succeed())

				Expect(res).To(HaveKeyWithValue("thing", map[string]interface{}{
					"name":      "some-pod",
					"namespace": "some-ns",
				}))
			})
		})
	})
})
