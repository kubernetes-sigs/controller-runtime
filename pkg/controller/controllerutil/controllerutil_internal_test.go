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

package controllerutil

import (
	"errors"
	"io/ioutil"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Controllerutil internal", func() {

	Describe("ReadInClusterNamespaceCached", func() {
		It("shouldn't provide a namespace if running outside of the cluster", func() {
			_, err := ReadInClusterNamespaceCached()
			// For the env test case, the controller is not run in a cluster
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ErrNotRunningInCluster)).To(Equal(true))
		})
		It("should return a namespace if file exists", func() {
			file, err := ioutil.TempFile("", "namespace")
			Expect(err).NotTo(HaveOccurred())
			_, err = file.WriteString("kube-system")
			Expect(err).NotTo(HaveOccurred())
			Expect(file.Close()).To(Succeed())
			inClusterNamespacePath = file.Name()
			namespace, err := ReadInClusterNamespaceCached()
			Expect(err).NotTo(HaveOccurred())
			Expect(namespace).To(Equal("kube-system"))
			Expect(os.Remove(file.Name())).To(Succeed())
			inClusterNamespacePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
		})

	})
})
