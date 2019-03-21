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

package metrics

import (
	"testing"

	"go.opencensus.io/stats/view"
)

func TestRegisterUnregisterDefaultViews(t *testing.T) {
	RegisterDefaultViews()
	if view.Find(MeasureReconcileTotal.Name()) == nil {
		t.Errorf("Couldn't find view for ReconcileTotal")
	}
	if view.Find(MeasureReconcileErrors.Name()) == nil {
		t.Errorf("Couldn't find view for ReconcileErrors")
	}
	if view.Find(MeasureReconcileTime.Name()) == nil {
		t.Errorf("Couldn't find view for ReconcileTime")
	}

	UnregisterDefaultViews()
	if view.Find(MeasureReconcileTotal.Name()) != nil {
		t.Errorf("view for ReconcileTotal was not unregistered")
	}
	if view.Find(MeasureReconcileErrors.Name()) != nil {
		t.Errorf("view for ReconcileErrors was not unregistered")
	}
	if view.Find(MeasureReconcileTime.Name()) != nil {
		t.Errorf("view for ReconcileTime was not unregistered")
	}

}
