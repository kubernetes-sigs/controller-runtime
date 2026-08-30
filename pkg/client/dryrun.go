/*
Copyright 2020 The Kubernetes Authors.

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

// NewDryRunClient wraps an existing client and enforces DryRun mode
// on all mutating api calls.
//
// DryRunAll is applied as a forced option, so it takes precedence over any
// per call option and cannot be turned off by the caller.
func NewDryRunClient(c Client) Client {
	return withInjectedOptions(c, writeOptions{}, writeOptions{
		create:      []CreateOption{DryRunAll},
		update:      []UpdateOption{DryRunAll},
		patch:       []PatchOption{DryRunAll},
		apply:       []ApplyOption{DryRunAll},
		delete:      []DeleteOption{DryRunAll},
		deleteAllOf: []DeleteAllOfOption{DryRunAll},

		subResourceCreate: []SubResourceCreateOption{DryRunAll},
		subResourceUpdate: []SubResourceUpdateOption{DryRunAll},
		subResourcePatch:  []SubResourcePatchOption{DryRunAll},
		subResourceApply:  []SubResourceApplyOption{DryRunAll},
	})
}
