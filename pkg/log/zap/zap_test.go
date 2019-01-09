/*
Copyright 2018 The Kubernetes Authors.

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

package zap

import (
	"bytes"
	"encoding/json"
	"io/ioutil"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	kapi "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// testStringer is a fmt.Stringer
type testStringer struct{}

func (testStringer) String() string {
	return "value"
}

// fakeSyncWriter is a fake zap.SyncerWriter that lets us test if sync was called
type fakeSyncWriter bool

func (w *fakeSyncWriter) Write(p []byte) (int, error) {
	return len(p), nil
}
func (w *fakeSyncWriter) Sync() error {
	*w = true
	return nil
}

// logInfo is the information for a particular fakeLogger message
type logInfo struct {
	name []string
	tags []interface{}
	msg  string
}

// fakeLoggerRoot is the root object to which all fakeLoggers record their messages.
type fakeLoggerRoot struct {
	messages []logInfo
}

// fakeLogger is a fake implementation of logr.Logger that records
// messages, tags, and names,
// just records the name.
type fakeLogger struct {
	name []string
	tags []interface{}

	root *fakeLoggerRoot
}

func (f *fakeLogger) WithName(name string) logr.Logger {
	names := append([]string(nil), f.name...)
	names = append(names, name)
	return &fakeLogger{
		name: names,
		tags: f.tags,
		root: f.root,
	}
}

func (f *fakeLogger) WithValues(vals ...interface{}) logr.Logger {
	tags := append([]interface{}(nil), f.tags...)
	tags = append(tags, vals...)
	return &fakeLogger{
		name: f.name,
		tags: tags,
		root: f.root,
	}
}

func (f *fakeLogger) Error(err error, msg string, vals ...interface{}) {
	tags := append([]interface{}(nil), f.tags...)
	tags = append(tags, "error", err)
	tags = append(tags, vals...)
	f.root.messages = append(f.root.messages, logInfo{
		name: append([]string(nil), f.name...),
		tags: tags,
		msg:  msg,
	})
}

func (f *fakeLogger) Info(msg string, vals ...interface{}) {
	tags := append([]interface{}(nil), f.tags...)
	tags = append(tags, vals...)
	f.root.messages = append(f.root.messages, logInfo{
		name: append([]string(nil), f.name...),
		tags: tags,
		msg:  msg,
	})
}

func (f *fakeLogger) Enabled() bool             { return true }
func (f *fakeLogger) V(lvl int) logr.InfoLogger { return f }

var _ = Describe("Zap logger setup", func() {
	Context("with the default output", func() {
		It("shouldn't fail when setting up production", func() {
			Expect(Logger(false)).NotTo(BeNil())
		})

		It("shouldn't fail when setting up development", func() {
			Expect(Logger(true)).NotTo(BeNil())
		})
	})

	Context("with custom non-sync output", func() {
		It("shouldn't fail when setting up production", func() {
			Expect(LoggerTo(ioutil.Discard, false)).NotTo(BeNil())
		})

		It("shouldn't fail when setting up development", func() {
			Expect(LoggerTo(ioutil.Discard, true)).NotTo(BeNil())
		})
	})

	Context("when logging kubernetes objects", func() {
		var logOut *bytes.Buffer
		var logger logr.Logger

		BeforeEach(func() {
			logOut = new(bytes.Buffer)
			By("setting up the logger")
			// use production settings (false) to get just json output
			logger = LoggerTo(logOut, false)
		})

		It("should log a standard namespaced Kubernetes object name and namespace", func() {
			pod := &kapi.Pod{}
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
			node := &kapi.Node{}
			node.Name = "some-node"
			logger.Info("here's a kubernetes object", "thing", node)

			outRaw := logOut.Bytes()
			res := map[string]interface{}{}
			Expect(json.Unmarshal(outRaw, &res)).To(Succeed())

			Expect(res).To(HaveKeyWithValue("thing", map[string]interface{}{
				"name": node.Name,
			}))
		})

		It("should log a standard Kubernetes object's kind, if set", func() {
			node := &kapi.Node{}
			node.Name = "some-node"
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
			name := types.NamespacedName{Name: "some-node"}
			logger.Info("here's a kubernetes object", "thing", name)

			outRaw := logOut.Bytes()
			res := map[string]interface{}{}
			Expect(json.Unmarshal(outRaw, &res)).To(Succeed())

			Expect(res).To(HaveKeyWithValue("thing", map[string]interface{}{
				"name": name.Name,
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
	})
})
