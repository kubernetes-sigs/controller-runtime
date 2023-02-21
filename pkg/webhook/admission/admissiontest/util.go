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

package admissiontest

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// FakeValidator provides fake validating webhook functionality for testing
// It implements the admission.Validator interface and
// rejects all requests with the same configured error
// or passes if ErrorToReturn is nil.
// And it would always return configured warning messages WarningsToReturn.
type FakeValidator struct {
	// ErrorToReturn is the error for which the FakeValidator rejects all requests
	ErrorToReturn error `json:"errorToReturn,omitempty"`
	// GVKToReturn is the GroupVersionKind that the webhook operates on
	GVKToReturn schema.GroupVersionKind
	// WarningsToReturn is the warnings for FakeValidator returns to all requests
	WarningsToReturn []string
}

func (v *FakeValidator) ValidateCreate() (warnings []string, err error) {
	return v.WarningsToReturn, v.ErrorToReturn
}

func (v *FakeValidator) ValidateUpdate(old runtime.Object) (warnings []string, err error) {
	return v.WarningsToReturn, v.ErrorToReturn
}

func (v *FakeValidator) ValidateDelete() (warnings []string, err error) {
	return v.WarningsToReturn, v.ErrorToReturn
}

func (v *FakeValidator) SetGroupVersionKind(gvk schema.GroupVersionKind) {
	v.GVKToReturn = gvk
}

func (v *FakeValidator) GroupVersionKind() schema.GroupVersionKind {
	return v.GVKToReturn
}

func (v *FakeValidator) GetObjectKind() schema.ObjectKind {
	return v
}

func (v *FakeValidator) DeepCopyObject() runtime.Object {
	return &FakeValidator{
		ErrorToReturn:    v.ErrorToReturn,
		GVKToReturn:      v.GVKToReturn,
		WarningsToReturn: v.WarningsToReturn,
	}
}
