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

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCombine(t *testing.T) {
	// Defaults come first, per call options in the middle, forced options last.
	got := combine([]int{1}, []int{2, 3}, []int{4})
	if want := []int{1, 2, 3, 4}; !reflect.DeepEqual(got, want) {
		t.Errorf("combine() = %v, want %v", got, want)
	}

	// With no defaults and no forced options the per call slice is returned as is.
	mid := []int{2, 3}
	if got := combine(nil, mid, nil); !reflect.DeepEqual(got, mid) {
		t.Errorf("combine() = %v, want %v", got, mid)
	}
}

// recordingClient records the options passed to the write verbs it is asked
// about, so tests can assert what the wrapper injected.
type recordingClient struct {
	Client
	createOpts []CreateOption
	sub        *recordingSubResourceClient
}

func (r *recordingClient) Create(_ context.Context, _ Object, opts ...CreateOption) error {
	r.createOpts = opts
	return nil
}

func (r *recordingClient) SubResource(_ string) SubResourceClient { return r.sub }

type recordingSubResourceClient struct {
	SubResourceClient
	createOpts []SubResourceCreateOption
}

func (r *recordingSubResourceClient) Create(_ context.Context, _ Object, _ Object, opts ...SubResourceCreateOption) error {
	r.createOpts = opts
	return nil
}

func newRecordingClient() *recordingClient {
	return &recordingClient{sub: &recordingSubResourceClient{}}
}

func TestInjectedOptionsDefaultIsOverridable(t *testing.T) {
	rec := newRecordingClient()
	c := WithFieldOwner(rec, "default-owner")

	if err := c.Create(t.Context(), &corev1.ConfigMap{}, FieldOwner("caller-owner")); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	got := (&CreateOptions{}).ApplyOptions(rec.createOpts)
	if got.FieldManager != "caller-owner" {
		t.Errorf("FieldManager = %q, want the per call value %q to override the default", got.FieldManager, "caller-owner")
	}
}

func TestInjectedOptionsForcedWinsOverCaller(t *testing.T) {
	rec := newRecordingClient()
	c := NewDryRunClient(rec)

	// The caller tries to set a different DryRun value; the forced DryRunAll
	// is applied last and must win.
	if err := c.Create(t.Context(), &corev1.ConfigMap{}, &CreateOptions{DryRun: []string{"NotAll"}}); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	got := (&CreateOptions{}).ApplyOptions(rec.createOpts)
	if want := []string{metav1.DryRunAll}; !reflect.DeepEqual(got.DryRun, want) {
		t.Errorf("DryRun = %v, want forced %v", got.DryRun, want)
	}
}

func TestInjectedOptionsNestedForcedAndDefault(t *testing.T) {
	rec := newRecordingClient()
	// DryRun wraps FieldOwner: the forced dry run must still win, and the field
	// owner default must still be applied.
	c := NewDryRunClient(WithFieldOwner(rec, "owner"))

	if err := c.Create(t.Context(), &corev1.ConfigMap{}); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	got := (&CreateOptions{}).ApplyOptions(rec.createOpts)
	if got.FieldManager != "owner" {
		t.Errorf("FieldManager = %q, want %q", got.FieldManager, "owner")
	}
	if want := []string{metav1.DryRunAll}; !reflect.DeepEqual(got.DryRun, want) {
		t.Errorf("DryRun = %v, want %v", got.DryRun, want)
	}
}

func TestInjectedOptionsSubResource(t *testing.T) {
	rec := newRecordingClient()
	c := WithFieldOwner(rec, "owner")

	if err := c.SubResource("status").Create(t.Context(), &corev1.ConfigMap{}, &corev1.ConfigMap{}); err != nil {
		t.Fatalf("SubResource Create() error: %v", err)
	}

	got := (&SubResourceCreateOptions{}).ApplyOptions(rec.sub.createOpts)
	if got.FieldManager != "owner" {
		t.Errorf("subresource FieldManager = %q, want %q", got.FieldManager, "owner")
	}
}
