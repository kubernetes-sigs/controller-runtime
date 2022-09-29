package admission

import (
	"context"
	goerrors "errors"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/admissiontest"

	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
)

var fakeValidatorWarnVK = schema.GroupVersionKind{Group: "foo.test.org", Version: "v1", Kind: "fakeValidatorWarn"}

var _ = Describe("validatingWarnHandler", func() {

	decoder, _ := NewDecoder(scheme.Scheme)

	Context("when dealing with successful results without warning", func() {
		f := &admissiontest.FakeValidatorWarn{ErrorToReturn: nil, GVKToReturn: fakeValidatorWarnVK, WarningsToReturn: nil}
		handler := validatingWarnHandler{validatorWarn: f, decoder: decoder}

		It("should return 200 in response when create succeeds", func() {
			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validatorWarn,
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
						Object: handler.validatorWarn,
					},
					OldObject: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validatorWarn,
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
						Object: handler.validatorWarn,
					},
				},
			})
			Expect(response.Allowed).Should(BeTrue())
			Expect(response.Result.Code).Should(Equal(int32(http.StatusOK)))
		})
	})

	const warningMessage = "warning message"
	const anotherWarningMessage = "another warning message"
	Context("when dealing with successful results with warning", func() {
		f := &admissiontest.FakeValidatorWarn{ErrorToReturn: nil, GVKToReturn: fakeValidatorWarnVK, WarningsToReturn: []string{
			warningMessage,
			anotherWarningMessage,
		}}
		handler := validatingWarnHandler{validatorWarn: f, decoder: decoder}

		It("should return 200 in response when create succeeds, with warning messages", func() {
			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validatorWarn,
					},
				},
			})

			Expect(response.Allowed).Should(BeTrue())
			Expect(response.Result.Code).Should(Equal(int32(http.StatusOK)))
			Expect(response.AdmissionResponse.Warnings).Should(ContainElement(warningMessage))
			Expect(response.AdmissionResponse.Warnings).Should(ContainElement(anotherWarningMessage))
		})

		It("should return 200 in response when update succeeds, with warning messages", func() {

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					Object: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validatorWarn,
					},
					OldObject: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validatorWarn,
					},
				},
			})
			Expect(response.Allowed).Should(BeTrue())
			Expect(response.Result.Code).Should(Equal(int32(http.StatusOK)))
			Expect(response.AdmissionResponse.Warnings).Should(ContainElement(warningMessage))
			Expect(response.AdmissionResponse.Warnings).Should(ContainElement(anotherWarningMessage))
		})

		It("should return 200 in response when delete succeeds, with warning messages", func() {

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Delete,
					OldObject: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validatorWarn,
					},
				},
			})
			Expect(response.Allowed).Should(BeTrue())
			Expect(response.Result.Code).Should(Equal(int32(http.StatusOK)))
			Expect(response.AdmissionResponse.Warnings).Should(ContainElement(warningMessage))
			Expect(response.AdmissionResponse.Warnings).Should(ContainElement(anotherWarningMessage))
		})
	})

	Context("when dealing with Status errors", func() {

		expectedError := &apierrors.StatusError{
			ErrStatus: metav1.Status{
				Message: "some message",
				Code:    http.StatusUnprocessableEntity,
			},
		}
		f := &admissiontest.FakeValidatorWarn{ErrorToReturn: expectedError, GVKToReturn: fakeValidatorVK, WarningsToReturn: nil}
		handler := validatingWarnHandler{validatorWarn: f, decoder: decoder}

		It("should propagate the Status from ValidateCreate's return value to the HTTP response", func() {

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validatorWarn,
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
						Object: handler.validatorWarn,
					},
					OldObject: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validatorWarn,
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
						Object: handler.validatorWarn,
					},
				},
			})

			Expect(response.Allowed).Should(BeFalse())
			Expect(response.Result.Code).Should(Equal(expectedError.Status().Code))
			Expect(*response.Result).Should(Equal(expectedError.Status()))

		})

	})

	Context("when dealing with non-status errors, without warning messages", func() {

		expectedError := goerrors.New("some error")
		f := &admissiontest.FakeValidatorWarn{ErrorToReturn: expectedError, GVKToReturn: fakeValidatorVK}
		handler := validatingWarnHandler{validatorWarn: f, decoder: decoder}

		It("should return 403 response when ValidateCreate with error message embedded", func() {

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validatorWarn,
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
						Object: handler.validatorWarn,
					},
					OldObject: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validatorWarn,
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
						Object: handler.validatorWarn,
					},
				},
			})
			Expect(response.Allowed).Should(BeFalse())
			Expect(response.Result.Code).Should(Equal(int32(http.StatusForbidden)))
			Expect(string(response.Result.Reason)).Should(Equal(expectedError.Error()))
		})
	})

	Context("when dealing with non-status errors, with warning messages", func() {

		expectedError := goerrors.New("some error")
		f := &admissiontest.FakeValidatorWarn{ErrorToReturn: expectedError, GVKToReturn: fakeValidatorVK, WarningsToReturn: []string{warningMessage, anotherWarningMessage}}
		handler := validatingWarnHandler{validatorWarn: f, decoder: decoder}

		It("should return 403 response when ValidateCreate with error message embedded", func() {

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validatorWarn,
					},
				},
			})
			Expect(response.Allowed).Should(BeFalse())
			Expect(response.Result.Code).Should(Equal(int32(http.StatusForbidden)))
			Expect(string(response.Result.Reason)).Should(Equal(expectedError.Error()))
			Expect(response.AdmissionResponse.Warnings).Should(ContainElement(warningMessage))
			Expect(response.AdmissionResponse.Warnings).Should(ContainElement(anotherWarningMessage))
		})

		It("should return 403 response when ValidateUpdate returns non-APIStatus error", func() {

			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					Object: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validatorWarn,
					},
					OldObject: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validatorWarn,
					},
				},
			})
			Expect(response.Allowed).Should(BeFalse())
			Expect(response.Result.Code).Should(Equal(int32(http.StatusForbidden)))
			Expect(string(response.Result.Reason)).Should(Equal(expectedError.Error()))
			Expect(response.AdmissionResponse.Warnings).Should(ContainElement(warningMessage))
			Expect(response.AdmissionResponse.Warnings).Should(ContainElement(anotherWarningMessage))

		})

		It("should return 403 response when ValidateDelete returns non-APIStatus error", func() {
			response := handler.Handle(context.TODO(), Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Delete,
					OldObject: runtime.RawExtension{
						Raw:    []byte("{}"),
						Object: handler.validatorWarn,
					},
				},
			})
			Expect(response.Allowed).Should(BeFalse())
			Expect(response.Result.Code).Should(Equal(int32(http.StatusForbidden)))
			Expect(string(response.Result.Reason)).Should(Equal(expectedError.Error()))
			Expect(response.AdmissionResponse.Warnings).Should(ContainElement(warningMessage))
			Expect(response.AdmissionResponse.Warnings).Should(ContainElement(anotherWarningMessage))

		})
	})

})
