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

package client_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ListOptions", func() {
	It("Should set LabelSelector", func() {
		labelSelector, err := labels.Parse("a=b")
		Expect(err).NotTo(HaveOccurred())
		o := &client.ListOptions{LabelSelector: labelSelector}
		newListOpts := &client.ListOptions{}
		o.ApplyToList(newListOpts)
		Expect(newListOpts).To(Equal(o))
	})
	It("Should set FieldSelector", func() {
		o := &client.ListOptions{FieldSelector: fields.Nothing()}
		newListOpts := &client.ListOptions{}
		o.ApplyToList(newListOpts)
		Expect(newListOpts).To(Equal(o))
	})
	It("Should set Namespace", func() {
		o := &client.ListOptions{Namespace: "my-ns"}
		newListOpts := &client.ListOptions{}
		o.ApplyToList(newListOpts)
		Expect(newListOpts).To(Equal(o))
	})
	It("Should set Raw", func() {
		o := &client.ListOptions{Raw: &metav1.ListOptions{FieldSelector: "Hans"}}
		newListOpts := &client.ListOptions{}
		o.ApplyToList(newListOpts)
		Expect(newListOpts).To(Equal(o))
	})
	It("Should not set anything", func() {
		o := &client.ListOptions{}
		newListOpts := &client.ListOptions{}
		o.ApplyToList(newListOpts)
		Expect(newListOpts).To(Equal(o))
	})
})

var _ = Describe("CreateOptions", func() {
	It("Should set DryRun", func() {
		o := &client.CreateOptions{DryRun: []string{"Hello", "Theodore"}}
		newCreatOpts := &client.CreateOptions{}
		o.ApplyToCreate(newCreatOpts)
		Expect(newCreatOpts).To(Equal(o))
	})
	It("Should set FieldManager", func() {
		o := &client.CreateOptions{FieldManager: "FieldManager"}
		newCreatOpts := &client.CreateOptions{}
		o.ApplyToCreate(newCreatOpts)
		Expect(newCreatOpts).To(Equal(o))
	})
	It("Should set Raw", func() {
		o := &client.CreateOptions{Raw: &metav1.CreateOptions{DryRun: []string{"Bye", "Theodore"}}}
		newCreatOpts := &client.CreateOptions{}
		o.ApplyToCreate(newCreatOpts)
		Expect(newCreatOpts).To(Equal(o))
	})
	It("Should not set anything", func() {
		o := &client.CreateOptions{}
		newCreatOpts := &client.CreateOptions{}
		o.ApplyToCreate(newCreatOpts)
		Expect(newCreatOpts).To(Equal(o))
	})
})
