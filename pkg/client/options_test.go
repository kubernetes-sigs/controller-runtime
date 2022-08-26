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
	Describe("ApplyToList", func() {
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

	Describe("AsListOptions", func() {
		It("Should preserve the Raw LabelSelector field", func() {
			o := &metav1.ListOptions{
				LabelSelector: "a=b",
			}
			newListOpts := &client.ListOptions{Raw: o.DeepCopy()}
			Expect(newListOpts.AsListOptions()).To(Equal(o))
		})
		It("Should override the Raw LabelSelector field", func() {
			labelSelector, err := labels.Parse("a=b")
			Expect(err).NotTo(HaveOccurred())

			newListOpts := &client.ListOptions{
				LabelSelector: labelSelector,
				Raw: &metav1.ListOptions{
					LabelSelector: "b=c",
				},
			}
			Expect(newListOpts.AsListOptions()).To(Equal(
				&metav1.ListOptions{
					LabelSelector: "a=b",
				},
			))
		})
		It("Should preserve the Raw FieldSelector field", func() {
			o := &metav1.ListOptions{
				FieldSelector: "spec.nodeName=test-node-001",
			}
			newListOpts := &client.ListOptions{Raw: o.DeepCopy()}
			Expect(newListOpts.AsListOptions()).To(Equal(o))
		})
		It("Should override the Raw FieldSelector field", func() {
			labelSelector, err := labels.Parse("spec.nodeName=test-node-001")
			Expect(err).NotTo(HaveOccurred())

			newListOpts := &client.ListOptions{
				LabelSelector: labelSelector,
				Raw: &metav1.ListOptions{
					LabelSelector: "spec.nodeName=test-node-002",
				},
			}
			Expect(newListOpts.AsListOptions()).To(Equal(
				&metav1.ListOptions{
					LabelSelector: "spec.nodeName=test-node-001",
				},
			))
		})
	})
})

var _ = Describe("GetOptions", func() {
	Describe("ApplyToList", func() {
		It("Should set Raw", func() {
			o := &client.GetOptions{Raw: &metav1.GetOptions{ResourceVersion: "RV0"}}
			newGetOpts := &client.GetOptions{}
			o.ApplyToGet(newGetOpts)
			Expect(newGetOpts).To(Equal(o))
		})
	})
})

var _ = Describe("CreateOptions", func() {
	Describe("ApplyToList", func() {
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

	Describe("AsCreateOptions", func() {
		It("Should preserve the Raw DryRun field", func() {
			o := &metav1.CreateOptions{
				DryRun: []string{"foo"},
			}
			newCreateOpts := &client.CreateOptions{Raw: o.DeepCopy()}
			Expect(newCreateOpts.AsCreateOptions()).To(Equal(o))
		})
		It("Should override the Raw DryRun field", func() {
			newCreateOpts := &client.CreateOptions{
				DryRun: []string{"foo"},
				Raw: &metav1.CreateOptions{
					DryRun: []string{"bar"},
				},
			}
			Expect(newCreateOpts.AsCreateOptions()).To(Equal(
				&metav1.CreateOptions{
					DryRun: []string{"foo"},
				},
			))
		})
		It("Should preserve the Raw FieldManager field", func() {
			o := &metav1.CreateOptions{
				FieldManager: "foo",
			}
			newCreateOpts := &client.CreateOptions{Raw: o.DeepCopy()}
			Expect(newCreateOpts.AsCreateOptions()).To(Equal(o))
		})
		It("Should override the Raw FieldManager field", func() {
			newCreateOpts := &client.CreateOptions{
				FieldManager: "foo",
				Raw: &metav1.CreateOptions{
					FieldManager: "bar",
				},
			}
			Expect(newCreateOpts.AsCreateOptions()).To(Equal(
				&metav1.CreateOptions{
					FieldManager: "foo",
				},
			))
		})
	})
})

var _ = Describe("DeleteOptions", func() {
	Describe("ApplyToList", func() {
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

	Describe("AsDeleteOptions", func() {
		It("Should preserve the Raw GracePeriodSeconds field", func() {
			o := &metav1.DeleteOptions{
				GracePeriodSeconds: utilpointer.Int64Ptr(30),
			}
			newDeleteOpts := &client.DeleteOptions{Raw: o.DeepCopy()}
			Expect(newDeleteOpts.AsDeleteOptions()).To(Equal(o))
		})
		It("Should override the Raw GracePeriodSeconds field", func() {
			newDeleteOpts := &client.DeleteOptions{
				GracePeriodSeconds: utilpointer.Int64Ptr(30),
				Raw: &metav1.DeleteOptions{
					GracePeriodSeconds: utilpointer.Int64Ptr(90),
				},
			}
			Expect(newDeleteOpts.AsDeleteOptions()).To(Equal(
				&metav1.DeleteOptions{
					GracePeriodSeconds: utilpointer.Int64Ptr(30),
				},
			))
		})
		It("Should preserve the Raw Preconditions field", func() {
			o := &metav1.DeleteOptions{
				Preconditions: &metav1.Preconditions{
					ResourceVersion: utilpointer.StringPtr("foo"),
				},
			}
			newDeleteOpts := &client.DeleteOptions{Raw: o.DeepCopy()}
			Expect(newDeleteOpts.AsDeleteOptions()).To(Equal(o))
		})
		It("Should override the Raw Preconditions field", func() {
			newDeleteOpts := &client.DeleteOptions{
				Preconditions: &metav1.Preconditions{
					ResourceVersion: utilpointer.StringPtr("foo"),
				},
				Raw: &metav1.DeleteOptions{
					Preconditions: &metav1.Preconditions{
						ResourceVersion: utilpointer.StringPtr("bar"),
					},
				},
			}
			Expect(newDeleteOpts.AsDeleteOptions()).To(Equal(
				&metav1.DeleteOptions{
					Preconditions: &metav1.Preconditions{
						ResourceVersion: utilpointer.StringPtr("foo"),
					},
				},
			))
		})
		It("Should preserve the Raw PropagationPolicy field", func() {
			foregroundPropagationPolicy := metav1.DeletePropagationForeground
			o := &metav1.DeleteOptions{
				PropagationPolicy: &foregroundPropagationPolicy,
			}
			newDeleteOpts := &client.DeleteOptions{Raw: o.DeepCopy()}
			Expect(newDeleteOpts.AsDeleteOptions()).To(Equal(o))
		})
		It("Should override the Raw PropagationPolicy field", func() {
			foregroundPropagationPolicy := metav1.DeletePropagationForeground
			backgroundPropagationPolicy := metav1.DeletePropagationBackground
			newDeleteOpts := &client.DeleteOptions{
				PropagationPolicy: &foregroundPropagationPolicy,
				Raw: &metav1.DeleteOptions{
					PropagationPolicy: &backgroundPropagationPolicy,
				},
			}
			Expect(newDeleteOpts.AsDeleteOptions()).To(Equal(
				&metav1.DeleteOptions{
					PropagationPolicy: &foregroundPropagationPolicy,
				},
			))
		})
		It("Should preserve the Raw DryRun field", func() {
			o := &metav1.DeleteOptions{
				DryRun: []string{"foo"},
			}
			newDeleteOpts := &client.DeleteOptions{Raw: o.DeepCopy()}
			Expect(newDeleteOpts.AsDeleteOptions()).To(Equal(o))
		})
		It("Should override the Raw DryRun field", func() {
			newDeleteOpts := &client.DeleteOptions{
				DryRun: []string{"foo"},
				Raw: &metav1.DeleteOptions{
					DryRun: []string{"bar"},
				},
			}
			Expect(newDeleteOpts.AsDeleteOptions()).To(Equal(
				&metav1.DeleteOptions{
					DryRun: []string{"foo"},
				},
			))
		})
	})
})

var _ = Describe("UpdateOptions", func() {
	Describe("ApplyToList", func() {
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

	Describe("AsUpdateOptions", func() {
		It("Should preserve the Raw DryRun field", func() {
			o := &metav1.UpdateOptions{
				DryRun: []string{"foo"},
			}
			newUpdateOpts := &client.UpdateOptions{Raw: o.DeepCopy()}
			Expect(newUpdateOpts.AsUpdateOptions()).To(Equal(o))
		})
		It("Should override the Raw DryRun field", func() {
			newUpdateOpts := &client.UpdateOptions{
				DryRun: []string{"foo"},
				Raw: &metav1.UpdateOptions{
					DryRun: []string{"bar"},
				},
			}
			Expect(newUpdateOpts.AsUpdateOptions()).To(Equal(
				&metav1.UpdateOptions{
					DryRun: []string{"foo"},
				},
			))
		})
		It("Should preserve the Raw FieldManager field", func() {
			o := &metav1.UpdateOptions{
				FieldManager: "foo",
			}
			newUpdateOpts := &client.UpdateOptions{Raw: o.DeepCopy()}
			Expect(newUpdateOpts.AsUpdateOptions()).To(Equal(o))
		})
		It("Should override the Raw FieldManager field", func() {
			newUpdateOpts := &client.UpdateOptions{
				FieldManager: "foo",
				Raw: &metav1.UpdateOptions{
					FieldManager: "bar",
				},
			}
			Expect(newUpdateOpts.AsUpdateOptions()).To(Equal(
				&metav1.UpdateOptions{
					FieldManager: "foo",
				},
			))
		})
	})
})

var _ = Describe("PatchOptions", func() {
	Describe("ApplyToList", func() {
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

	Describe("AsPatchOptions", func() {
		It("Should preserve the Raw DryRun field", func() {
			o := &metav1.PatchOptions{
				DryRun: []string{"foo"},
			}
			newPatchOpts := &client.PatchOptions{Raw: o.DeepCopy()}
			Expect(newPatchOpts.AsPatchOptions()).To(Equal(o))
		})
		It("Should override the Raw DryRun field", func() {
			newPatchOpts := &client.PatchOptions{
				DryRun: []string{"foo"},
				Raw: &metav1.PatchOptions{
					DryRun: []string{"bar"},
				},
			}
			Expect(newPatchOpts.AsPatchOptions()).To(Equal(
				&metav1.PatchOptions{
					DryRun: []string{"foo"},
				},
			))
		})
		It("Should preserve the Raw Force field", func() {
			o := &metav1.PatchOptions{
				Force: utilpointer.BoolPtr(true),
			}
			newPatchOpts := &client.PatchOptions{Raw: o.DeepCopy()}
			Expect(newPatchOpts.AsPatchOptions()).To(Equal(o))
		})
		It("Should override the Raw Force field", func() {
			newPatchOpts := &client.PatchOptions{
				Force: utilpointer.BoolPtr(true),
				Raw: &metav1.PatchOptions{
					Force: utilpointer.BoolPtr(false),
				},
			}
			Expect(newPatchOpts.AsPatchOptions()).To(Equal(
				&metav1.PatchOptions{
					Force: utilpointer.BoolPtr(true),
				},
			))
		})
		It("Should preserve the Raw FieldManager field", func() {
			o := &metav1.PatchOptions{
				FieldManager: "foo",
			}
			newPatchOpts := &client.PatchOptions{Raw: o.DeepCopy()}
			Expect(newPatchOpts.AsPatchOptions()).To(Equal(o))
		})
		It("Should override the Raw FieldManager field", func() {
			newPatchOpts := &client.PatchOptions{
				FieldManager: "foo",
				Raw: &metav1.PatchOptions{
					FieldManager: "bar",
				},
			}
			Expect(newPatchOpts.AsPatchOptions()).To(Equal(
				&metav1.PatchOptions{
					FieldManager: "foo",
				},
			))
		})
	})
})

var _ = Describe("DeleteAllOfOptions", func() {
	It("Should set ListOptions", func() {
		o := &client.DeleteAllOfOptions{ListOptions: client.ListOptions{Raw: &metav1.ListOptions{}}}
		newDeleteAllOfOpts := &client.DeleteAllOfOptions{}
		o.ApplyToDeleteAllOf(newDeleteAllOfOpts)
		Expect(newDeleteAllOfOpts).To(Equal(o))
	})
	It("Should set DeleteOptions", func() {
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
