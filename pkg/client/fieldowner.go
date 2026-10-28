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

// WithFieldOwner wraps a Client and adds the fieldOwner as the field
// manager to all write requests from this client. If additional [FieldOwner]
// options are specified on methods of this client, the value specified here
// will be overridden.
func WithFieldOwner(c Client, fieldOwner string) Client {
	owner := FieldOwner(fieldOwner)
	return withInjectedOptions(c, writeOptions{
		create: []CreateOption{owner},
		update: []UpdateOption{owner},
		patch:  []PatchOption{owner},
		apply:  []ApplyOption{owner},

		subResourceCreate: []SubResourceCreateOption{owner},
		subResourceUpdate: []SubResourceUpdateOption{owner},
		subResourcePatch:  []SubResourcePatchOption{owner},
		subResourceApply:  []SubResourceApplyOption{owner},
	}, writeOptions{})
}
