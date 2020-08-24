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

		When("create succeeds", func() {
			It("should return allowed in response", func() {

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
		})

		When("create fails on decode", func() {
			It("should return 400 in response", func() {
				//TODO
			})
		})

		When("ValidateCreate returns APIStatus error", func() {
			It("should return Status.Code and embed the Status object from APIStatus in the response", func() {

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
		})

		When("ValidateCreate returns any error", func() {
			It("should return 403 and embed the error message in a generated Status object in the response", func() {

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
		})

		When("update succeeds", func() {
			It("should return allowed in response", func() {

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
		})

		When("update fails on decoding new object", func() {
			It("should return 400 in response", func() {
				//TODO
			})
		})

		When("update fails on decoding old object", func() {
			It("should return 400 in response", func() {
				//TODO
			})
		})

		When("ValidateUpdate returns APIStatus error", func() {
			It("should return Status.Code and embed the Status object from APIStatus in the response", func() {

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
		})

		When("ValidateUpdate returns any error", func() {
			It("should return 403 and embed the error message in a generated Status object in the response", func() {

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
		})

		When("delete succeeds", func() {
			It("should return allowed in response", func() {

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
		})

		When("delete fails on decode", func() {
			It("should return 400 in response", func() {
				//TODO
			})
		})

		When("ValidateDelete returns APIStatus error", func() {
			It("should return Status.Code and embed the Status object from APIStatus in the response", func() {

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
		})

		When("ValidateDelete returns any error", func() {
			It("should return 403 and embed the error message in a generated Status object in the response", func() {

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
