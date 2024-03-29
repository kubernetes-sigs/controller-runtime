//go:build !ignore_autogenerated

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CustomControllerManagerConfiguration) DeepCopyInto(out *CustomControllerManagerConfiguration) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ControllerManagerConfigurationSpec.DeepCopyInto(&out.ControllerManagerConfigurationSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CustomControllerManagerConfiguration.
func (in *CustomControllerManagerConfiguration) DeepCopy() *CustomControllerManagerConfiguration {
	if in == nil {
		return nil
	}
	out := new(CustomControllerManagerConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CustomControllerManagerConfiguration) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
