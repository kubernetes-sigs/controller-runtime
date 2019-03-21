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

package client

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
)

// Applier declaratively ensures that objects match some particular
// configuration.  It does this by using server-side apply.
// Each applier represents a different "concern" that owns a set of
// fields, so different reconcilers should generally have different
// appliers.
type Applier struct {
	Name string
	Writer
}

// InjectClient automatically receives a client, if desired.
func (a *Applier) InjectClient(cl Client) error {
	a.Writer = cl
	return nil
}

// Ensure makes sure that the given object is configured according
// to the configuration passed in (using server-side apply).
//
// To use Ensure, construct an object that has the fields set that
// this particular applier cares about.  Fields and map elements that
// previously were set by this applier will be automatically removed
// if they're no longer present, new fields will be added, and existing
// ones updated.
//
// The server does this by keeping track of "ownership" of a particular
// field, and thus you should never call call client.Get and then Ensure.
func (a *Applier) Ensure(ctx context.Context, obj runtime.Object) error {
	return a.Patch(ctx, obj, Apply, ForceOwnership, FieldManager(a.Name))
}

// TODO(directxman12): figure out a nice workflow for automatically populating
// this -- we could get name from the controller name, for instance.
