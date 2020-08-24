package admission

import (
	"context"
	goerrors "errors"
	"net/http"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"k8s.io/api/admission/v1beta1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
)

var _ = Describe("validatingHandler", func() {
	Describe("Handle", func() {
		It("should return 200 in response when create succeeds", func() {

			handler := createSucceedingValidatingHandler()

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: v1beta1.AdmissionRequest{
					Operation: v1beta1.Create,
					Object: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validator,
					},
				},
			})
			Expect(response.Allowed).Should(BeTrue())
			Expect(response.Result.Code).Should(Equal(int32(http.StatusOK)))
		})

		It("should return 400 in response when create fails on decode", func() {
			//TODO
		})

		It("should return response built with the Status object when ValidateCreate returns APIStatus error", func() {

			handler, expectedError := createValidatingHandlerWhichReturnsStatusError()

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: v1beta1.AdmissionRequest{
					Operation: v1beta1.Create,
					Object: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validator,
					},
				},
			})
			Expect(response.Allowed).Should(BeFalse())

			apiStatus, ok := expectedError.(apierrs.APIStatus)
			Expect(ok).Should(BeTrue())
			Expect(response.Result.Code).Should(Equal(apiStatus.Status().Code))
			Expect(*response.Result).Should(Equal(apiStatus.Status()))

		})

		It("should return 403 response when ValidateCreate returns non-APIStatus error", func() {

			handler, expectedError := createValidatingHandlerWhichReturnsRegularError()

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: v1beta1.AdmissionRequest{
					Operation: v1beta1.Create,
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

		It("should return 200 in response when update succeeds", func() {

			handler := createSucceedingValidatingHandler()

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: v1beta1.AdmissionRequest{
					Operation: v1beta1.Update,
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

		It("should return 400 in response when update fails on decoding new object", func() {
			//TODO
		})

		It("should return 400 in response when update fails on decoding old object", func() {
			//TODO
		})

		It("should return response built with the Status object when ValidateUpdate returns APIStatus error", func() {

			handler, expectedError := createValidatingHandlerWhichReturnsStatusError()

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: v1beta1.AdmissionRequest{
					Operation: v1beta1.Update,
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

			apiStatus, ok := expectedError.(apierrs.APIStatus)
			Expect(ok).Should(BeTrue())
			Expect(response.Result.Code).Should(Equal(apiStatus.Status().Code))
			Expect(*response.Result).Should(Equal(apiStatus.Status()))

		})

		It("should return 403 response when ValidateUpdate returns non-APIStatus error", func() {

			handler, expectedError := createValidatingHandlerWhichReturnsRegularError()

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: v1beta1.AdmissionRequest{
					Operation: v1beta1.Update,
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

		It("should return 200 in response when delete succeeds", func() {

			handler := createSucceedingValidatingHandler()

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: v1beta1.AdmissionRequest{
					Operation: v1beta1.Delete,
					OldObject: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validator,
					},
				},
			})
			Expect(response.Allowed).Should(BeTrue())
			Expect(response.Result.Code).Should(Equal(int32(http.StatusOK)))
		})

		It("should return 400 in response when delete fails on decode", func() {
			//TODO
		})

		It("should return response built with the Status object when ValidateDelete returns APIStatus error", func() {

			handler, expectedError := createValidatingHandlerWhichReturnsStatusError()

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: v1beta1.AdmissionRequest{
					Operation: v1beta1.Delete,
					OldObject: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validator,
					},
				},
			})
			Expect(response.Allowed).Should(BeFalse())

			apiStatus, ok := expectedError.(apierrs.APIStatus)
			Expect(ok).Should(BeTrue())
			Expect(response.Result.Code).Should(Equal(apiStatus.Status().Code))
			Expect(*response.Result).Should(Equal(apiStatus.Status()))

		})

		It("should return 403 response when ValidateDelete returns non-APIStatus error", func() {

			handler, expectedError := createValidatingHandlerWhichReturnsRegularError()

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: v1beta1.AdmissionRequest{
					Operation: v1beta1.Delete,
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

func createSucceedingValidatingHandler() *validatingHandler {
	decoder, _ := NewDecoder(scheme.Scheme)
	f := &fakeValidator{ErrorToReturn: nil}
	return &validatingHandler{f, decoder}
}

func createValidatingHandlerWhichReturnsRegularError() (validatingHandler, error) {
	decoder, _ := NewDecoder(scheme.Scheme)
	errToReturn := goerrors.New("some error")
	f := &fakeValidator{ErrorToReturn: errToReturn}
	return validatingHandler{f, decoder}, errToReturn
}

func createValidatingHandlerWhichReturnsStatusError() (validatingHandler, error) {
	decoder, _ := NewDecoder(scheme.Scheme)
	errToReturn := &apierrs.StatusError{
		ErrStatus: v1.Status{
			Message: "some message",
			Code:    http.StatusUnprocessableEntity,
		},
	}
	f := &fakeValidator{ErrorToReturn: errToReturn}
	return validatingHandler{f, decoder}, errToReturn
}
