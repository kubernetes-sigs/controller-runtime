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

package pkg

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/runtime/scheme"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// firstMate is the Schema for the firstmates API
// +k8s:openapi-gen=true
type FirstMate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FirstMateSpec   `json:"spec,omitempty"`
	Status FirstMateStatus `json:"status,omitempty"`
}

// FirstMateSpec defines the desired state of firstMate
type FirstMateSpec struct {
	Height     *int  `json:"height,omitempty"`
	Experience int   `json:"experience,omitempty"`
	Crew       int32 `json:"crew,omitempty"`
}

// FirstMateStatus defines the observed state of firstMate
type FirstMateStatus struct {
	Location string
}

// ValidateCreate implements webhookutil.Validator so a webhook will be registered for the type
func (f *FirstMate) ValidateCreate() error {
	if f.Spec.Crew <= 0 {
		return fmt.Errorf("crew must be greater than 0")
	}
	return nil
}

// ValidateUpdate implements webhookutil.Validator so a webhook will be registered for the type
func (f *FirstMate) ValidateUpdate(old runtime.Object) error {
	if f.Spec.Crew <= 0 {
		return fmt.Errorf("crew must be greater than 0")
	}
	return nil
}

// Default implements webhookutil.Defaulter so a webhook will be registered for the type
func (f *FirstMate) Default() {
	if *f.Spec.Height == 0 {
		height := 10
		f.Spec.Height = &height
	}
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FirstMateList contains a list of firstMate
type FirstMateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FirstMate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FirstMate{}, &FirstMateList{})
}

var (
	// SchemeGroupVersion is group version used to register these objects
	SchemeGroupVersion = schema.GroupVersion{Group: "crew.example.com", Version: "v1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: SchemeGroupVersion}

	// AddToScheme is required by pkg/client/...
	AddToScheme = SchemeBuilder.AddToScheme
)

// Resource is required by pkg/client/listers/...
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}
