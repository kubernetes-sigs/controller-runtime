/*
Copyright 2024 The Kubernetes Authors.

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

package client

// WithFieldValidation wraps a Client and configures field validation, by
// default, for all write requests from this client. Users can override field
// validation for individual write requests.
//
// This wrapper has no effect on apply requests, as they do not support a
// custom fieldValidation setting, it is always strict.
func WithFieldValidation(c Client, validation FieldValidation) Client {
	return withInjectedOptions(c, writeOptions{
		create: []CreateOption{validation},
		update: []UpdateOption{validation},
		patch:  []PatchOption{validation},

		subResourceCreate: []SubResourceCreateOption{validation},
		subResourceUpdate: []SubResourceUpdateOption{validation},
		subResourcePatch:  []SubResourcePatchOption{validation},
	}, writeOptions{})
}
