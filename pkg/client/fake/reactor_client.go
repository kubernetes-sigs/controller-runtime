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

package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/internal/objectutil"
)

var (
//log = logf.RuntimeLog.WithName("fake-client")
)

type fakeReactorClient struct {
	testing.Fake
	scheme *runtime.Scheme
}

var _ client.Client = &fakeReactorClient{}

// NewFakeReactorClient creates a new fake client for testing.
// You can choose to initialize it with a slice of runtime.Object.
// Deprecated: use NewFakeReactorClientWithScheme.  You should always be
// passing an explicit Scheme.
func NewFakeReactorClient(initObjs ...runtime.Object) client.Client {
	return NewFakeReactorClientWithScheme(scheme.Scheme, initObjs...)
}

// NewFakeReactorClientWithScheme creates a new fake client with the given scheme
// for testing.
// You can choose to initialize it with a slice of runtime.Object.
func NewFakeReactorClientWithScheme(clientScheme *runtime.Scheme, initObjs ...runtime.Object) client.Client {
	tracker := testing.NewObjectTracker(clientScheme, scheme.Codecs.UniversalDecoder())
	for _, obj := range initObjs {
		if err := tracker.Add(obj); err != nil {
			panic(err)
		}
	}

	fc := &fakeReactorClient{scheme: clientScheme}
	fc.AddReactor("*", "*", testing.ObjectReaction(tracker))
	return fc
}

func (c *fakeReactorClient) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	gvr, err := getGVRFromObject(obj, c.scheme)
	if err != nil {
		return err
	}
	// TODO Is the defaultObj necessary?
	gvk, err := apiutil.GVKForObject(obj, scheme.Scheme)
	if err != nil {
		return err
	}
	defaultObj, err := scheme.Scheme.New(gvk)
	if err != nil {
		return fmt.Errorf("error creating a copy of %T: %v", obj, err)
	}
	o, err := c.Invokes(testing.NewGetAction(gvr, key.Namespace, key.Name), defaultObj)
	if err != nil {
		return err
	}
	j, err := json.Marshal(o)
	if err != nil {
		return err
	}
	decoder := scheme.Codecs.UniversalDecoder()
	_, _, err = decoder.Decode(j, nil, obj)
	return err
}

func (c *fakeReactorClient) List(ctx context.Context, obj runtime.Object, opts ...client.ListOptionFunc) error {
	gvk, err := apiutil.GVKForObject(obj, scheme.Scheme)
	if err != nil {
		return err
	}

	if !strings.HasSuffix(gvk.Kind, "List") {
		return fmt.Errorf("non-list type %T (kind %q) passed as output", obj, gvk)
	}
	// TODO Is the defaultObj necessary?
	defaultObj, err := scheme.Scheme.New(gvk)
	// we need the non-list GVK, so chop off the "List" from the end of the kind
	gvk.Kind = gvk.Kind[:len(gvk.Kind)-4]

	listOpts := client.ListOptions{}
	listOpts.ApplyOptions(opts)

	gvr, _ := meta.UnsafeGuessKindToResource(gvk)

	if err != nil {
		return fmt.Errorf("error creating a copy of %T: %v", obj, err)
	}
	o, err := c.Invokes(testing.NewListAction(gvr, gvk, listOpts.Namespace, *listOpts.AsListOptions()), defaultObj)
	if err != nil {
		return err
	}
	j, err := json.Marshal(o)
	if err != nil {
		return err
	}
	decoder := scheme.Codecs.UniversalDecoder()
	_, _, err = decoder.Decode(j, nil, obj)
	if err != nil {
		return err
	}

	if listOpts.LabelSelector != nil {
		objs, err := meta.ExtractList(obj)
		if err != nil {
			return err
		}
		filteredObjs, err := objectutil.FilterWithLabels(objs, listOpts.LabelSelector)
		if err != nil {
			return err
		}
		err = meta.SetList(obj, filteredObjs)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *fakeReactorClient) Create(ctx context.Context, obj runtime.Object, opts ...client.CreateOptionFunc) error {
	createOptions := &client.CreateOptions{}
	createOptions.ApplyOptions(opts)

	for _, dryRunOpt := range createOptions.DryRun {
		if dryRunOpt == metav1.DryRunAll {
			return nil
		}
	}

	gvr, err := getGVRFromObject(obj, c.scheme)
	if err != nil {
		return err
	}
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	// Is a nil defaultObj ok here?
	_, err = c.Invokes(testing.NewCreateAction(gvr, accessor.GetNamespace(), obj), nil)
	return err
}

func (c *fakeReactorClient) Delete(ctx context.Context, obj runtime.Object, opts ...client.DeleteOptionFunc) error {
	gvr, err := getGVRFromObject(obj, c.scheme)
	if err != nil {
		return err
	}
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	//TODO: implement propagation
	// Is a nil defaultObj ok here?
	_, err = c.Invokes(testing.NewDeleteAction(gvr, accessor.GetNamespace(), accessor.GetName()), nil)
	return err
}

func (c *fakeReactorClient) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOptionFunc) error {
	updateOptions := &client.UpdateOptions{}
	updateOptions.ApplyOptions(opts)

	for _, dryRunOpt := range updateOptions.DryRun {
		if dryRunOpt == metav1.DryRunAll {
			return nil
		}
	}

	gvr, err := getGVRFromObject(obj, c.scheme)
	if err != nil {
		return err
	}
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	// Is a nil defaultObj ok here?
	_, err = c.Invokes(testing.NewUpdateAction(gvr, accessor.GetNamespace(), obj), nil)
	return err
}

func (c *fakeReactorClient) Patch(ctx context.Context, obj runtime.Object, patch client.Patch, opts ...client.PatchOptionFunc) error {
	gvr, err := getGVRFromObject(obj, c.scheme)
	if err != nil {
		return err
	}
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	data, err := patch.Data(obj)
	if err != nil {
		return err
	}

	// TODO Is the defaultObj necessary?
	gvk, err := apiutil.GVKForObject(obj, scheme.Scheme)
	if err != nil {
		return err
	}
	defaultObj, err := scheme.Scheme.New(gvk)
	if err != nil {
		return fmt.Errorf("error creating a copy of %T: %v", obj, err)
	}

	o, err := c.Invokes(testing.NewPatchAction(gvr, accessor.GetNamespace(), accessor.GetName(), patch.Type(), data), defaultObj)

	j, err := json.Marshal(o)
	if err != nil {
		return err
	}
	decoder := scheme.Codecs.UniversalDecoder()
	_, _, err = decoder.Decode(j, nil, obj)
	return err
}

func (c *fakeReactorClient) Status() client.StatusWriter {
	return &fakeReactorStatusWriter{client: c}
}

// func getGVRFromObject(obj runtime.Object, scheme *runtime.Scheme) (schema.GroupVersionResource, error) {
// 	gvk, err := apiutil.GVKForObject(obj, scheme)
// 	if err != nil {
// 		return schema.GroupVersionResource{}, err
// 	}
// 	gvr, _ := meta.UnsafeGuessKindToResource(gvk)
// 	return gvr, nil
// }

type fakeReactorStatusWriter struct {
	client *fakeReactorClient
}

func (sw *fakeReactorStatusWriter) Update(ctx context.Context, obj runtime.Object) error {
	gvr, err := getGVRFromObject(obj, sw.client.scheme)
	if err != nil {
		return err
	}
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	// Is a nil defaultObj ok here?
	_, err = sw.client.Invokes(testing.NewUpdateSubresourceAction(gvr, "status", accessor.GetNamespace(), obj), nil)
	return err
}
