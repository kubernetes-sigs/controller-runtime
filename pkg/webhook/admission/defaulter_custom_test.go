/*
Copyright 2021 The Kubernetes Authors.
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

package admission

import (
	"context"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gomodules.xyz/jsonpatch/v2"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ = Describe("Defaulter Handler", func() {

	It("should remove unknown fields when DefaulterRemoveUnknownFields is passed", func() {
		obj := &TestDefaulter{}
		handler := WithCustomDefaulter(admissionScheme, obj, &TestCustomDefaulter{}, DefaulterRemoveUnknownOrOmitableFields)

		resp := handler.Handle(context.TODO(), Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object: runtime.RawExtension{
					Raw: []byte(`{"newField":"foo", "totalReplicas":5}`),
				},
			},
		})
		Expect(resp.Allowed).Should(BeTrue())
		Expect(resp.Patches).To(HaveLen(3))
		Expect(resp.Patches).To(ContainElements(
			jsonpatch.JsonPatchOperation{
				Operation: "add",
				Path:      "/replica",
				Value:     2.0,
			},
			jsonpatch.JsonPatchOperation{
				Operation: "remove",
				Path:      "/newField",
			},
			jsonpatch.JsonPatchOperation{
				Operation: "remove",
				Path:      "/totalReplicas",
			},
		))
		Expect(resp.Result.Code).Should(Equal(int32(http.StatusOK)))
	})

	It("should preserve unknown fields by default", func() {
		obj := &TestDefaulter{}
		handler := WithCustomDefaulter(admissionScheme, obj, &TestCustomDefaulter{})

		resp := handler.Handle(context.TODO(), Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object: runtime.RawExtension{
					Raw: []byte(`{"newField":"foo", "totalReplicas":5}`),
				},
			},
		})
		Expect(resp.Allowed).Should(BeTrue())
		Expect(resp.Patches).To(HaveLen(2))
		Expect(resp.Patches).To(ContainElements(
			jsonpatch.JsonPatchOperation{
				Operation: "add",
				Path:      "/replica",
				Value:     2.0,
			},
			jsonpatch.JsonPatchOperation{
				Operation: "remove",
				Path:      "/totalReplicas",
			},
		))
		Expect(resp.Result.Code).Should(Equal(int32(http.StatusOK)))
	})

	It("should return ok if received delete verb in defaulter handler", func() {
		obj := &TestDefaulter{}
		handler := WithCustomDefaulter(admissionScheme, obj, &TestCustomDefaulter{})
		resp := handler.Handle(context.TODO(), Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Delete,
				OldObject: runtime.RawExtension{
					Raw: []byte("{}"),
				},
			},
		})
		Expect(resp.Allowed).Should(BeTrue())
		Expect(resp.Result.Code).Should(Equal(int32(http.StatusOK)))
	})
})

// TestDefaulter.
var _ runtime.Object = &TestDefaulter{}

type TestDefaulter struct {
	Replica       int `json:"replica,omitempty"`
	TotalReplicas int `json:"totalReplicas,omitempty"`
}

var testDefaulterGVK = schema.GroupVersionKind{Group: "foo.test.org", Version: "v1", Kind: "TestDefaulter"}

func (d *TestDefaulter) GetObjectKind() schema.ObjectKind { return d }
func (d *TestDefaulter) DeepCopyObject() runtime.Object {
	return &TestDefaulter{
		Replica:       d.Replica,
		TotalReplicas: d.TotalReplicas,
	}
}

func (d *TestDefaulter) GroupVersionKind() schema.GroupVersionKind {
	return testDefaulterGVK
}

func (d *TestDefaulter) SetGroupVersionKind(gvk schema.GroupVersionKind) {}

var _ runtime.Object = &TestDefaulterList{}

type TestDefaulterList struct{}

func (*TestDefaulterList) GetObjectKind() schema.ObjectKind { return nil }
func (*TestDefaulterList) DeepCopyObject() runtime.Object   { return nil }

// TestCustomDefaulter
type TestCustomDefaulter struct{}

func (d *TestCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	o := obj.(*TestDefaulter)
	if o.Replica < 2 {
		o.Replica = 2
	}
	o.TotalReplicas = 0
	return nil
}
