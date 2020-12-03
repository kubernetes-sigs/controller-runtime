package admission

import (
	"context"
	goerrors "errors"
	"net/http"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admission/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
)

var _ = Describe("validatingHandler", func() {

	decoder, _ := NewDecoder(scheme.Scheme)

	Context("when dealing with successful results", func() {

		f := &fakeValidator{ErrorToReturn: nil}
		handler := validatingHandler{validator: f, decoder: decoder}

		It("should return 200 in response when create succeeds", func() {

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validator,
					},
				},
			})

			Expect(response.Allowed).Should(BeTrue())
			Expect(response.Result.Code).Should(Equal(int32(http.StatusOK)))
		})

		It("should return 200 in response when update succeeds", func() {

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					Object: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validator,
					},
					OldObject: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validator,
					},
				},
			})
			Expect(response.Allowed).Should(BeTrue())
			Expect(response.Result.Code).Should(Equal(int32(http.StatusOK)))
		})

		It("should return 200 in response when delete succeeds", func() {

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Delete,
					OldObject: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validator,
					},
				},
			})
			Expect(response.Allowed).Should(BeTrue())
			Expect(response.Result.Code).Should(Equal(int32(http.StatusOK)))
		})

	})

	Context("when dealing with Status errors", func() {

		expectedError := &apierrs.StatusError{
			ErrStatus: metav1.Status{
				Message: "some message",
				Code:    http.StatusUnprocessableEntity,
			},
		}
		f := &fakeValidator{ErrorToReturn: expectedError}
		handler := validatingHandler{validator: f, decoder: decoder}

		It("should propagate the Status from ValidateCreate's return value to the HTTP response", func() {

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validator,
					},
				},
			})

			Expect(response.Allowed).Should(BeFalse())
			Expect(response.Result.Code).Should(Equal(expectedError.Status().Code))
			Expect(*response.Result).Should(Equal(expectedError.Status()))

		})

		It("should propagate the Status from ValidateUpdate's return value to the HTTP response", func() {

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					Object: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validator,
					},
					OldObject: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validator,
					},
				},
			})

			Expect(response.Allowed).Should(BeFalse())
			Expect(response.Result.Code).Should(Equal(expectedError.Status().Code))
			Expect(*response.Result).Should(Equal(expectedError.Status()))

		})

		It("should propagate the Status from ValidateDelete's return value to the HTTP response", func() {

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Delete,
					OldObject: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validator,
					},
				},
			})

			Expect(response.Allowed).Should(BeFalse())
			Expect(response.Result.Code).Should(Equal(expectedError.Status().Code))
			Expect(*response.Result).Should(Equal(expectedError.Status()))

		})

	})
	Context("when dealing with non-status errors", func() {

		expectedError := goerrors.New("some error")
		f := &fakeValidator{ErrorToReturn: expectedError}
		handler := validatingHandler{validator: f, decoder: decoder}

		It("should return 403 response when ValidateCreate with error message embedded", func() {

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validator,
					},
				},
			})
			Expect(response.Allowed).Should(BeFalse())
			Expect(response.Result.Code).Should(Equal(int32(http.StatusForbidden)))
			Expect(string(response.Result.Reason)).Should(Equal(expectedError.Error()))

		})

		It("should return 403 response when ValidateUpdate returns non-APIStatus error", func() {

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					Object: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validator,
					},
					OldObject: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validator,
					},
				},
			})
			Expect(response.Allowed).Should(BeFalse())
			Expect(response.Result.Code).Should(Equal(int32(http.StatusForbidden)))
			Expect(string(response.Result.Reason)).Should(Equal(expectedError.Error()))

		})

		It("should return 403 response when ValidateDelete returns non-APIStatus error", func() {

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Delete,
					OldObject: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validator,
					},
				},
			})
			Expect(response.Allowed).Should(BeFalse())
			Expect(response.Result.Code).Should(Equal(int32(http.StatusForbidden)))
			Expect(string(response.Result.Reason)).Should(Equal(expectedError.Error()))

		})

	})

	PIt("should return 400 in response when create fails on decode", func() {})

	PIt("should return 400 in response when update fails on decoding new object", func() {})

	PIt("should return 400 in response when update fails on decoding old object", func() {})

	PIt("should return 400 in response when delete fails on decode", func() {})

})

type fakeValidator struct {
	ErrorToReturn error `json:"ErrorToReturn,omitempty"`
}

var _ Validator = &fakeValidator{}

var fakeValidatorVK = schema.GroupVersionKind{Group: "foo.test.org", Version: "v1", Kind: "fakeValidator"}

func (v *fakeValidator) ValidateCreate() error {
	return v.ErrorToReturn
}

func (v *fakeValidator) ValidateUpdate(old runtime.Object) error {
	return v.ErrorToReturn
}

func (v *fakeValidator) ValidateDelete() error {
	return v.ErrorToReturn
}

func (v *fakeValidator) GetObjectKind() schema.ObjectKind { return v }

func (v *fakeValidator) DeepCopyObject() runtime.Object {
	return &fakeValidator{ErrorToReturn: v.ErrorToReturn}
}

func (v *fakeValidator) GroupVersionKind() schema.GroupVersionKind {
	return fakeValidatorVK
}

func (v *fakeValidator) SetGroupVersionKind(gvk schema.GroupVersionKind) {}
