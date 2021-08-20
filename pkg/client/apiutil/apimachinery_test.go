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

// Package apiutil contains utilities for working with raw Kubernetes
// API machinery, such as creating RESTMappers and raw REST clients,
// and extracting the GVK of an object.
package apiutil

import (
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubernetesscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var exampleSchemeGroupVersion = schema.GroupVersion{Group: "example.com", Version: "v1"}

type ExampleCRD struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

func (e *ExampleCRD) DeepCopyObject() runtime.Object {
	panic("not implemented")
}

func TestCanUseProtobuf(t *testing.T) {
	exampleCRDScheme := runtime.NewScheme()

	builder := &scheme.Builder{GroupVersion: exampleSchemeGroupVersion}
	builder.Register(&ExampleCRD{})
	if err := builder.AddToScheme(exampleCRDScheme); err != nil {
		t.Fatalf("AddToScheme failed: %v", err)
	}

	schemes := map[string]*runtime.Scheme{
		"kubernetes":  kubernetesscheme.Scheme,
		"empty":       runtime.NewScheme(),
		"example.com": exampleCRDScheme,
	}
	grid := []struct {
		scheme             string
		gvk                schema.GroupVersionKind
		wantType           string
		wantCanUseProtobuf bool
	}{
		{
			scheme:             "kubernetes",
			gvk:                schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			wantType:           "v1.Pod",
			wantCanUseProtobuf: true,
		},
		{
			scheme:             "kubernetes",
			gvk:                schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Pod"},
			wantType:           "",
			wantCanUseProtobuf: false,
		},
		{
			scheme:             "empty",
			gvk:                schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			wantType:           "",
			wantCanUseProtobuf: false,
		},
		{
			scheme:             "example.com",
			gvk:                schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			wantType:           "",
			wantCanUseProtobuf: false,
		},
		{
			scheme:             "example.com",
			gvk:                schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "ExampleCRD"},
			wantType:           "apiutil.ExampleCRD",
			wantCanUseProtobuf: false,
		},
	}

	for _, g := range grid {
		t.Run(fmt.Sprintf("%#v", g), func(t *testing.T) {
			scheme := schemes[g.scheme]
			if scheme == nil {
				t.Errorf("scheme %q not found", g.scheme)
			}

			gotType := ""
			if t := scheme.AllKnownTypes()[g.gvk]; t != nil {
				gotType = t.String()
			}
			if gotType != g.wantType {
				t.Errorf("unexpected type got %v, want %v", gotType, g.wantType)
			}
			gotCanUseProtobuf := canUseProtobuf(scheme, g.gvk)
			if gotCanUseProtobuf != g.wantCanUseProtobuf {
				t.Errorf("canUseProtobuf(%#v, %#v) got %v, want %v", g.scheme, g.gvk, gotCanUseProtobuf, g.wantCanUseProtobuf)
			}
		})
	}
}

func TestCanUseProtobufForAllBuiltins(t *testing.T) {
	emptyScheme := runtime.NewScheme()

	allKnownTypes := kubernetesscheme.Scheme.AllKnownTypes()
	for gvk := range allKnownTypes {
		// Ignore internal bookkeeping types
		if gvk.Version == "__internal" || gvk.Group == "internal.apiserver.k8s.io" {
			continue
		}

		if !canUseProtobuf(kubernetesscheme.Scheme, gvk) {
			// If this fails on a k8s api library upgrade, we likely need to update isWellKnownKindThatSupportsProto for a new built-in group.
			t.Errorf("canUseProtobuf for built-in GVK %#v returned false, expected built-ins to support proto", gvk)
		}

		// If we don't have the type in the scheme, double check we don't try to use proto.
		if canUseProtobuf(emptyScheme, gvk) {
			t.Errorf("canUseProtobuf for built-in GVK %#v returned true with empty scheme, but empty scheme cannot support proto", gvk)
		}
	}
}
