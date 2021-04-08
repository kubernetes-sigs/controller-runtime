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
	utilpointer "k8s.io/utils/pointer"
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
	It("Should set Limit", func() {
		o := &client.ListOptions{Limit: int64(1)}
		newListOpts := &client.ListOptions{}
		o.ApplyToList(newListOpts)
		Expect(newListOpts).To(Equal(o))
	})
	It("Should set Continue", func() {
		o := &client.ListOptions{Continue: "foo"}
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

var _ = Describe("DeleteOptions", func() {
	It("Should set GracePeriodSeconds", func() {
		o := &client.DeleteOptions{GracePeriodSeconds: utilpointer.Int64Ptr(42)}
		newDeleteOpts := &client.DeleteOptions{}
		o.ApplyToDelete(newDeleteOpts)
		Expect(newDeleteOpts).To(Equal(o))
	})
	It("Should set Preconditions", func() {
		o := &client.DeleteOptions{Preconditions: &metav1.Preconditions{}}
		newDeleteOpts := &client.DeleteOptions{}
		o.ApplyToDelete(newDeleteOpts)
		Expect(newDeleteOpts).To(Equal(o))
	})
	It("Should set PropagationPolicy", func() {
		policy := metav1.DeletePropagationBackground
		o := &client.DeleteOptions{PropagationPolicy: &policy}
		newDeleteOpts := &client.DeleteOptions{}
		o.ApplyToDelete(newDeleteOpts)
		Expect(newDeleteOpts).To(Equal(o))
	})
	It("Should set Raw", func() {
		o := &client.DeleteOptions{Raw: &metav1.DeleteOptions{}}
		newDeleteOpts := &client.DeleteOptions{}
		o.ApplyToDelete(newDeleteOpts)
		Expect(newDeleteOpts).To(Equal(o))
	})
	It("Should set DryRun", func() {
		o := &client.DeleteOptions{DryRun: []string{"Hello", "Pippa"}}
		newDeleteOpts := &client.DeleteOptions{}
		o.ApplyToDelete(newDeleteOpts)
		Expect(newDeleteOpts).To(Equal(o))
	})
	It("Should not set anything", func() {
		o := &client.DeleteOptions{}
		newDeleteOpts := &client.DeleteOptions{}
		o.ApplyToDelete(newDeleteOpts)
		Expect(newDeleteOpts).To(Equal(o))
	})
})

var _ = Describe("UpdateOptions", func() {
	It("Should set DryRun", func() {
		o := &client.UpdateOptions{DryRun: []string{"Bye", "Pippa"}}
		newUpdateOpts := &client.UpdateOptions{}
		o.ApplyToUpdate(newUpdateOpts)
		Expect(newUpdateOpts).To(Equal(o))
	})
	It("Should set FieldManager", func() {
		o := &client.UpdateOptions{FieldManager: "Hello Boris"}
		newUpdateOpts := &client.UpdateOptions{}
		o.ApplyToUpdate(newUpdateOpts)
		Expect(newUpdateOpts).To(Equal(o))
	})
	It("Should set Raw", func() {
		o := &client.UpdateOptions{Raw: &metav1.UpdateOptions{}}
		newUpdateOpts := &client.UpdateOptions{}
		o.ApplyToUpdate(newUpdateOpts)
		Expect(newUpdateOpts).To(Equal(o))
	})
	It("Should not set anything", func() {
		o := &client.UpdateOptions{}
		newUpdateOpts := &client.UpdateOptions{}
		o.ApplyToUpdate(newUpdateOpts)
		Expect(newUpdateOpts).To(Equal(o))
	})
})

var _ = Describe("PatchOptions", func() {
	It("Should set DryRun", func() {
		o := &client.PatchOptions{DryRun: []string{"Bye", "Boris"}}
		newPatchOpts := &client.PatchOptions{}
		o.ApplyToPatch(newPatchOpts)
		Expect(newPatchOpts).To(Equal(o))
	})
	It("Should set Force", func() {
		o := &client.PatchOptions{Force: utilpointer.BoolPtr(true)}
		newPatchOpts := &client.PatchOptions{}
		o.ApplyToPatch(newPatchOpts)
		Expect(newPatchOpts).To(Equal(o))
	})
	It("Should set FieldManager", func() {
		o := &client.PatchOptions{FieldManager: "Hello Julian"}
		newPatchOpts := &client.PatchOptions{}
		o.ApplyToPatch(newPatchOpts)
		Expect(newPatchOpts).To(Equal(o))
	})
	It("Should set Raw", func() {
		o := &client.PatchOptions{Raw: &metav1.PatchOptions{}}
		newPatchOpts := &client.PatchOptions{}
		o.ApplyToPatch(newPatchOpts)
		Expect(newPatchOpts).To(Equal(o))
	})
	It("Should not set anything", func() {
		o := &client.PatchOptions{}
		newPatchOpts := &client.PatchOptions{}
		o.ApplyToPatch(newPatchOpts)
		Expect(newPatchOpts).To(Equal(o))
	})
})

var _ = Describe("DeleteAllOfOptions", func() {
	It("Should set ListOptions", func() {
		o := &client.DeleteAllOfOptions{ListOptions: client.ListOptions{Raw: &metav1.ListOptions{}}}
		newDeleteAllOfOpts := &client.DeleteAllOfOptions{}
		o.ApplyToDeleteAllOf(newDeleteAllOfOpts)
		Expect(newDeleteAllOfOpts).To(Equal(o))
	})
	It("Should set DeleleteOptions", func() {
		o := &client.DeleteAllOfOptions{DeleteOptions: client.DeleteOptions{GracePeriodSeconds: utilpointer.Int64Ptr(44)}}
		newDeleteAllOfOpts := &client.DeleteAllOfOptions{}
		o.ApplyToDeleteAllOf(newDeleteAllOfOpts)
		Expect(newDeleteAllOfOpts).To(Equal(o))
	})
})

var _ = Describe("MatchingLabels", func() {
	It("Should produce an invalid selector when given invalid input", func() {
		matchingLabels := client.MatchingLabels(map[string]string{"k": "axahm2EJ8Phiephe2eixohbee9eGeiyees1thuozi1xoh0GiuH3diewi8iem7Nui"})
		listOpts := &client.ListOptions{}
		matchingLabels.ApplyToList(listOpts)

		r, _ := listOpts.LabelSelector.Requirements()
		_, err := labels.NewRequirement(r[0].Key(), r[0].Operator(), r[0].Values().List())
		Expect(err).ToNot(BeNil())
		expectedErrMsg := `values[0][k]: Invalid value: "axahm2EJ8Phiephe2eixohbee9eGeiyees1thuozi1xoh0GiuH3diewi8iem7Nui": must be no more than 63 characters`
		Expect(err.Error()).To(Equal(expectedErrMsg))
	})
})
