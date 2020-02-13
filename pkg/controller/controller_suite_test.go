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

package controller_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	crscheme "sigs.k8s.io/controller-runtime/pkg/scheme"
)

func TestSource(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "Controller Integration Suite", []Reporter{printer.NewlineReporter{}})
}

var testenv *envtest.Environment
var cfg *rest.Config
var clientset *kubernetes.Clientset

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.LoggerTo(GinkgoWriter, true))

	err := (&crscheme.Builder{
		GroupVersion: schema.GroupVersion{Group: "chaosapps.metamagical.io", Version: "v1"},
	}).
		Register(&UnconventionalListType{}, &UnconventionalListTypeList{}).
		AddToScheme(scheme.Scheme)
	Expect(err).To(BeNil())

	testenv = &envtest.Environment{
		CRDDirectoryPaths: []string{"testdata/crds"},
	}

	cfg, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())

	clientset, err = kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())

	// Prevent the metrics listener being created
	metrics.DefaultBindAddress = "0"

	close(done)
}, 60)

var _ = AfterSuite(func() {
	Expect(testenv.Stop()).To(Succeed())

	// Put the DefaultBindAddress back
	metrics.DefaultBindAddress = ":8080"
})

var _ runtime.Object = &UnconventionalListType{}
var _ runtime.Object = &UnconventionalListTypeList{}

// UnconventionalListType is used to test CRDs with List types that
// have a slice of pointers rather than a slice of literals.
type UnconventionalListType struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              string `json:"spec,omitempty"`
}

// DeepCopyObject implements runtime.Object
// Handwritten for simplicity.
func (u *UnconventionalListType) DeepCopyObject() runtime.Object {
	return u.DeepCopy()
}

func (u *UnconventionalListType) DeepCopy() *UnconventionalListType {
	return &UnconventionalListType{
		TypeMeta:   u.TypeMeta,
		ObjectMeta: *u.ObjectMeta.DeepCopy(),
		Spec:       u.Spec,
	}
}

// UnconventionalListTypeList is used to test CRDs with List types that
// have a slice of pointers rather than a slice of literals.
type UnconventionalListTypeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []*UnconventionalListType `json:"items"`
}

// DeepCopyObject implements runtime.Object
// Handwritten for simplicity.
func (u *UnconventionalListTypeList) DeepCopyObject() runtime.Object {
	return u.DeepCopy()
}

func (u *UnconventionalListTypeList) DeepCopy() *UnconventionalListTypeList {
	out := &UnconventionalListTypeList{
		TypeMeta: u.TypeMeta,
		ListMeta: *u.ListMeta.DeepCopy(),
	}
	for _, item := range u.Items {
		out.Items = append(out.Items, item.DeepCopy())
	}
	return out
}
