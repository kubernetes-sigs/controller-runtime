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

package ctrl_test

import (
	"testing"
	"time"

	"github.com/kubernetes-sigs/kubebuilder/pkg/informer"
	logf "github.com/kubernetes-sigs/kubebuilder/pkg/log"
	"github.com/kubernetes-sigs/kubebuilder/pkg/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestSource(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "controller Suite", []Reporter{test.NewlineReporter{}})
}

var testenv *test.TestEnvironment
var cfg *rest.Config
var clientset *kubernetes.Clientset
var icache informer.Informers

var _ = BeforeSuite(func() {
	logf.SetLogger(logf.ZapLogger(true))

	testenv = &test.TestEnvironment{}

	var err error
	cfg, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())

	time.Sleep(1 * time.Second)

	clientset, err = kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	testenv.Stop()
})
