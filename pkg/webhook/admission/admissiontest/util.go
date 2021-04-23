package admissiontest

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// FakeValidator provides fake validating webhook functionality for testing
// It implements the admission.Validator interface and
// rejects all requests with the same configured error
// or passes if ErrorToReturn is nil.
type FakeValidator struct {
	// ErrorToReturn is the error for which the FakeValidator rejects all requests
	ErrorToReturn error `json:"ErrorToReturn,omitempty"`
	// GVKToReturn is the GroupVersionKind that the webhook operates on
	GVKToReturn schema.GroupVersionKind
}

// ValidateCreate implements admission.Validator
func (v *FakeValidator) ValidateCreate() error {
	return v.ErrorToReturn
}

// ValidateUpdate implements admission.Validator
func (v *FakeValidator) ValidateUpdate(old runtime.Object) error {
	return v.ErrorToReturn
}

// ValidateDelete implements admission.Validator
func (v *FakeValidator) ValidateDelete() error {
	return v.ErrorToReturn
}

// GetObjectKind implements admission.Validator
func (v *FakeValidator) GetObjectKind() schema.ObjectKind { return v }

// DeepCopyObject implements admission.Validator
func (v *FakeValidator) DeepCopyObject() runtime.Object {
	return &FakeValidator{ErrorToReturn: v.ErrorToReturn, GVKToReturn: v.GVKToReturn}
}

// GroupVersionKind implements admission.Validator
func (v *FakeValidator) GroupVersionKind() schema.GroupVersionKind {
	return v.GVKToReturn
}

// SetGroupVersionKind implements admission.Validator
func (v *FakeValidator) SetGroupVersionKind(gvk schema.GroupVersionKind) {
	v.GVKToReturn = gvk
}
