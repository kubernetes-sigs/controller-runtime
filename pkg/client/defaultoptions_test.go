/*
Copyright The Kubernetes Authors.

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

package client_test

import (
	"context"
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const defaultOwner = "default-owner"

// capture records the field manager and dry run that reached the underlying
// client, so each verb can be asserted the same way.
type capture struct {
	fieldManager string
	dryRun       []string
}

// newWrappedClient returns a client that injects FieldOwner(defaultOwner) as a
// default and DryRunAll as a forced option, plus the capture it records into.
func newWrappedClient() (client.Client, *capture) {
	rec := &capture{}
	fakeClient := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
		Create: func(_ context.Context, _ client.WithWatch, _ client.Object, opts ...client.CreateOption) error {
			o := &client.CreateOptions{}
			o.ApplyOptions(opts)
			*rec = capture{o.FieldManager, o.DryRun}
			return nil
		},
		Update: func(_ context.Context, _ client.WithWatch, _ client.Object, opts ...client.UpdateOption) error {
			o := &client.UpdateOptions{}
			o.ApplyOptions(opts)
			*rec = capture{o.FieldManager, o.DryRun}
			return nil
		},
		Patch: func(_ context.Context, _ client.WithWatch, _ client.Object, _ client.Patch, opts ...client.PatchOption) error {
			o := &client.PatchOptions{}
			o.ApplyOptions(opts)
			*rec = capture{o.FieldManager, o.DryRun}
			return nil
		},
		Apply: func(_ context.Context, _ client.WithWatch, _ runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
			o := &client.ApplyOptions{}
			for _, opt := range opts {
				opt.ApplyToApply(o)
			}
			*rec = capture{o.FieldManager, o.DryRun}
			return nil
		},
		Delete: func(_ context.Context, _ client.WithWatch, _ client.Object, opts ...client.DeleteOption) error {
			o := &client.DeleteOptions{}
			o.ApplyOptions(opts)
			*rec = capture{dryRun: o.DryRun}
			return nil
		},
		DeleteAllOf: func(_ context.Context, _ client.WithWatch, _ client.Object, opts ...client.DeleteAllOfOption) error {
			o := &client.DeleteAllOfOptions{}
			o.ApplyOptions(opts)
			*rec = capture{dryRun: o.DeleteOptions.DryRun}
			return nil
		},
		SubResourceCreate: func(_ context.Context, _ client.Client, _ string, _ client.Object, _ client.Object, opts ...client.SubResourceCreateOption) error {
			o := &client.SubResourceCreateOptions{}
			o.ApplyOptions(opts)
			*rec = capture{o.FieldManager, o.DryRun}
			return nil
		},
		SubResourceUpdate: func(_ context.Context, _ client.Client, _ string, _ client.Object, opts ...client.SubResourceUpdateOption) error {
			o := &client.SubResourceUpdateOptions{}
			o.ApplyOptions(opts)
			*rec = capture{o.FieldManager, o.DryRun}
			return nil
		},
		SubResourcePatch: func(_ context.Context, _ client.Client, _ string, _ client.Object, _ client.Patch, opts ...client.SubResourcePatchOption) error {
			o := &client.SubResourcePatchOptions{}
			o.ApplyOptions(opts)
			*rec = capture{o.FieldManager, o.DryRun}
			return nil
		},
		SubResourceApply: func(_ context.Context, _ client.Client, _ string, _ runtime.ApplyConfiguration, opts ...client.SubResourceApplyOption) error {
			o := &client.SubResourceApplyOptions{}
			for _, opt := range opts {
				opt.ApplyToSubResourceApply(o)
			}
			*rec = capture{o.FieldManager, o.DryRun}
			return nil
		},
	}).Build()

	wrapped := client.NewDryRunClient(client.WithFieldOwner(fakeClient, defaultOwner))
	return wrapped, rec
}

// TestInjectedOptions verifies for every write verb that the injected default
// is applied, the caller can override it, and the forced option always wins.
func TestInjectedOptionsCollapsesWrapping(t *testing.T) {
	c, rec := newWrappedClient()
	if err := c.Create(t.Context(), &corev1.ConfigMap{}); err != nil {
		t.Fatal(err)
	}
	if rec.fieldManager != defaultOwner {
		t.Errorf("fieldManager = %q, want %q", rec.fieldManager, defaultOwner)
	}
	if !slices.Equal(rec.dryRun, []string{metav1.DryRunAll}) {
		t.Errorf("dryRun = %v, want %v", rec.dryRun, []string{metav1.DryRunAll})
	}
}

func TestInjectedOptions(t *testing.T) {
	obj := &corev1.ConfigMap{}
	ac := corev1ac.ConfigMap("", "")
	forcedDryRun := []string{metav1.DryRunAll}

	tests := []struct {
		name string
		// hasFieldOwner is false for Delete and DeleteAllOf, which carry no
		// field manager.
		hasFieldOwner bool
		// invoke runs the verb with the given caller field owner (empty = none)
		// and caller dry run (nil = none).
		invoke func(ctx context.Context, c client.Client, callerOwner string, callerDryRun []string) error
	}{
		{name: "Create", hasFieldOwner: true, invoke: func(ctx context.Context, c client.Client, owner string, dr []string) error {
			var opts []client.CreateOption
			if owner != "" {
				opts = append(opts, client.FieldOwner(owner))
			}
			if dr != nil {
				opts = append(opts, &client.CreateOptions{DryRun: dr})
			}
			return c.Create(ctx, obj, opts...)
		}},
		{name: "Update", hasFieldOwner: true, invoke: func(ctx context.Context, c client.Client, owner string, dr []string) error {
			var opts []client.UpdateOption
			if owner != "" {
				opts = append(opts, client.FieldOwner(owner))
			}
			if dr != nil {
				opts = append(opts, &client.UpdateOptions{DryRun: dr})
			}
			return c.Update(ctx, obj, opts...)
		}},
		{name: "Patch", hasFieldOwner: true, invoke: func(ctx context.Context, c client.Client, owner string, dr []string) error {
			var opts []client.PatchOption
			if owner != "" {
				opts = append(opts, client.FieldOwner(owner))
			}
			if dr != nil {
				opts = append(opts, &client.PatchOptions{DryRun: dr})
			}
			return c.Patch(ctx, obj, client.Merge, opts...)
		}},
		{name: "Apply", hasFieldOwner: true, invoke: func(ctx context.Context, c client.Client, owner string, dr []string) error {
			var opts []client.ApplyOption
			if owner != "" {
				opts = append(opts, client.FieldOwner(owner))
			}
			if dr != nil {
				opts = append(opts, &client.ApplyOptions{DryRun: dr})
			}
			return c.Apply(ctx, ac, opts...)
		}},
		{name: "Delete", hasFieldOwner: false, invoke: func(ctx context.Context, c client.Client, _ string, dr []string) error {
			var opts []client.DeleteOption
			if dr != nil {
				opts = append(opts, &client.DeleteOptions{DryRun: dr})
			}
			return c.Delete(ctx, obj, opts...)
		}},
		{name: "DeleteAllOf", hasFieldOwner: false, invoke: func(ctx context.Context, c client.Client, _ string, dr []string) error {
			var opts []client.DeleteAllOfOption
			if dr != nil {
				o := &client.DeleteAllOfOptions{}
				o.DryRun = dr
				opts = append(opts, o)
			}
			return c.DeleteAllOf(ctx, obj, opts...)
		}},
		{name: "SubResourceCreate", hasFieldOwner: true, invoke: func(ctx context.Context, c client.Client, owner string, dr []string) error {
			var opts []client.SubResourceCreateOption
			if owner != "" {
				opts = append(opts, client.FieldOwner(owner))
			}
			if dr != nil {
				o := &client.SubResourceCreateOptions{}
				o.DryRun = dr
				opts = append(opts, o)
			}
			return c.SubResource("status").Create(ctx, obj, obj, opts...)
		}},
		{name: "SubResourceUpdate", hasFieldOwner: true, invoke: func(ctx context.Context, c client.Client, owner string, dr []string) error {
			var opts []client.SubResourceUpdateOption
			if owner != "" {
				opts = append(opts, client.FieldOwner(owner))
			}
			if dr != nil {
				o := &client.SubResourceUpdateOptions{}
				o.DryRun = dr
				opts = append(opts, o)
			}
			return c.SubResource("status").Update(ctx, obj, opts...)
		}},
		{name: "SubResourcePatch", hasFieldOwner: true, invoke: func(ctx context.Context, c client.Client, owner string, dr []string) error {
			var opts []client.SubResourcePatchOption
			if owner != "" {
				opts = append(opts, client.FieldOwner(owner))
			}
			if dr != nil {
				o := &client.SubResourcePatchOptions{}
				o.DryRun = dr
				opts = append(opts, o)
			}
			return c.SubResource("status").Patch(ctx, obj, client.Merge, opts...)
		}},
		{name: "SubResourceApply", hasFieldOwner: true, invoke: func(ctx context.Context, c client.Client, owner string, dr []string) error {
			var opts []client.SubResourceApplyOption
			if owner != "" {
				opts = append(opts, client.FieldOwner(owner))
			}
			if dr != nil {
				o := &client.SubResourceApplyOptions{}
				o.DryRun = dr
				opts = append(opts, o)
			}
			return c.SubResource("status").Apply(ctx, ac, opts...)
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("default applied, forced wins", func(t *testing.T) {
				c, rec := newWrappedClient()
				if err := tc.invoke(t.Context(), c, "", nil); err != nil {
					t.Fatalf("%s: %v", tc.name, err)
				}
				if tc.hasFieldOwner && rec.fieldManager != defaultOwner {
					t.Errorf("fieldManager = %q, want default %q", rec.fieldManager, defaultOwner)
				}
				if !slices.Equal(rec.dryRun, forcedDryRun) {
					t.Errorf("dryRun = %v, want forced %v", rec.dryRun, forcedDryRun)
				}
			})

			if tc.hasFieldOwner {
				t.Run("caller overrides default", func(t *testing.T) {
					c, rec := newWrappedClient()
					if err := tc.invoke(t.Context(), c, "caller-owner", nil); err != nil {
						t.Fatalf("%s: %v", tc.name, err)
					}
					if rec.fieldManager != "caller-owner" {
						t.Errorf("fieldManager = %q, want caller %q", rec.fieldManager, "caller-owner")
					}
					if !slices.Equal(rec.dryRun, forcedDryRun) {
						t.Errorf("dryRun = %v, want forced %v", rec.dryRun, forcedDryRun)
					}
				})
			}

			t.Run("forced wins over caller dry run", func(t *testing.T) {
				c, rec := newWrappedClient()
				if err := tc.invoke(t.Context(), c, "", []string{"NotAll"}); err != nil {
					t.Fatalf("%s: %v", tc.name, err)
				}
				if !slices.Equal(rec.dryRun, forcedDryRun) {
					t.Errorf("dryRun = %v, want forced %v to win over caller", rec.dryRun, forcedDryRun)
				}
			})
		})
	}
}
