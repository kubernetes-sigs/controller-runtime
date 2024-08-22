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

var _ = Describe("CustomDefaulter Handler", func() {

	It("should should not lose unknown fields", func() {
		obj := &TestObject{}
		handler := WithCustomDefaulter(admissionScheme, obj, &TestCustomDefaulter{})

		resp := handler.Handle(context.TODO(), Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object: runtime.RawExtension{
					Raw: []byte(`{"newField":"foo"}`),
				},
			},
		})
		Expect(resp.Allowed).Should(BeTrue())
		Expect(resp.Patches).To(Equal([]jsonpatch.JsonPatchOperation{{
			Operation: "add",
			Path:      "/replica",
			Value:     2.0,
		}}))
		Expect(resp.Result.Code).Should(Equal(int32(http.StatusOK)))
	})

	It("should return ok if received delete verb in defaulter handler", func() {
		obj := &TestObject{}
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

type TestCustomDefaulter struct{}

func (d *TestCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	tObj := obj.(*TestObject)
	if tObj.Replica < 2 {
		tObj.Replica = 2
	}
	return nil
}

type TestObject struct {
	Replica int `json:"replica,omitempty"`
}

func (o *TestObject) GetObjectKind() schema.ObjectKind { return o }
func (o *TestObject) DeepCopyObject() runtime.Object {
	return &TestObject{
		Replica: o.Replica,
	}
}

func (o *TestObject) GroupVersionKind() schema.GroupVersionKind {
	return testDefaulterGVK
}

func (o *TestObject) SetGroupVersionKind(gvk schema.GroupVersionKind) {}

type TestObjectList struct{}

func (*TestObjectList) GetObjectKind() schema.ObjectKind { return nil }
func (*TestObjectList) DeepCopyObject() runtime.Object   { return nil }
