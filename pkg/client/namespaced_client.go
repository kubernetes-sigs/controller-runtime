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

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// NewNamespacedClient wraps an existing client enforcing the namespace value.
// All functions using this client will have the same namespace declared here.
func NewNamespacedClient(c Client, ns string, rmp meta.RESTMapper, sch *runtime.Scheme) Client {
	return &namespacedClient{
		client:     c,
		namespace:  ns,
		restmapper: rmp,
		scheme:     *sch,
	}
}

var _ Client = &namespacedClient{}

// namespacedClient is a Client that wraps another Client in order to enforce the specified namespace value.
type namespacedClient struct {
	namespace  string
	client     Client
	restmapper meta.RESTMapper
	scheme     runtime.Scheme
}

func getNamespace(restmapper meta.RESTMapper, obj runtime.Object, sch *runtime.Scheme) (bool, error) {
	// var sch = runtime.NewScheme()
	// // appsv1.AddToScheme(sch)
	// rbacv1.AddToScheme(sch)
	gvk, err := apiutil.GVKForObject(obj, sch)
	if err != nil {
		return false, err
	}
	if restmapper == nil {
		return false, err
	}

	// gvk := schema.GroupKind{
	// 	Group: obj.GetObjectKind().GroupVersionKind().Group,
	// 	Kind:  obj.GetObjectKind().GroupVersionKind().Kind,
	// }
	restmapping, err := restmapper.RESTMapping(gvk.GroupKind())
	if err != nil {
		return false, fmt.Errorf("error here restmapping %v", obj)
	}
	scope := restmapping.Scope.Name()

	if scope == "" {
		return false, nil
	}

	if scope != meta.RESTScopeNameNamespace {
		return true, nil
	}
	return false, nil
}

// Create implements clinet.Client
func (n *namespacedClient) Create(ctx context.Context, obj runtime.Object, opts ...CreateOption) error {
	metaObj, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	isNamespaceScoped, err := getNamespace(n.restmapper, obj, &n.scheme)
	if err != nil {
		return fmt.Errorf("erroring Here, %v", err)
	}
	if isNamespaceScoped {
		metaObj.SetNamespace(n.namespace)
	}
	return n.client.Create(ctx, obj, opts...)
}

// Update implements client.Client
func (n *namespacedClient) Update(ctx context.Context, obj runtime.Object, opts ...UpdateOption) error {
	metaObj, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	if n.namespace != "" && metaObj.GetNamespace() != n.namespace {
		metaObj.SetNamespace(n.namespace)
	}
	return n.client.Update(ctx, obj, opts...)
}

// Delete implements client.Client
func (n *namespacedClient) Delete(ctx context.Context, obj runtime.Object, opts ...DeleteOption) error {
	metaObj, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	if n.namespace != "" && metaObj.GetNamespace() != n.namespace {
		metaObj.SetNamespace(n.namespace)
	}
	return n.client.Delete(ctx, obj, opts...)
}

// DeleteAllOf implements client.Client
func (n *namespacedClient) DeleteAllOf(ctx context.Context, obj runtime.Object, opts ...DeleteAllOfOption) error {
	if n.namespace != "" {
		opts = append(opts, InNamespace(n.namespace))
	}
	return n.client.DeleteAllOf(ctx, obj, opts...)
}

// Patch implements client.Client
func (n *namespacedClient) Patch(ctx context.Context, obj runtime.Object, patch Patch, opts ...PatchOption) error {
	metaObj, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	if n.namespace != "" && metaObj.GetNamespace() != n.namespace {
		metaObj.SetNamespace(n.namespace)
	}
	return n.client.Patch(ctx, obj, patch, opts...)
}

// Get implements client.Client
func (n *namespacedClient) Get(ctx context.Context, key ObjectKey, obj runtime.Object) error {
	isNamespaceScoped, err := getNamespace(n.restmapper, obj, &n.scheme)
	if err != nil {
		return fmt.Errorf("erroring Here, %v %v", err, obj.GetObjectKind())
	}
	if isNamespaceScoped {
		key.Namespace = n.namespace
	}
	return n.client.Get(ctx, key, obj)
}

// List implements client.Client
func (n *namespacedClient) List(ctx context.Context, obj runtime.Object, opts ...ListOption) error {
	if n.namespace != "" {
		opts = append(opts, InNamespace(n.namespace))
	}
	return n.client.List(ctx, obj, opts...)
}

// Status implements client.StatusClient
func (n *namespacedClient) Status() StatusWriter {
	return &namespacedClientStatusWriter{client: n.client.Status(), namespace: n.namespace}
}

// ensure namespacedClientStatusWriter implements client.StatusWriter
var _ StatusWriter = &namespacedClientStatusWriter{}

type namespacedClientStatusWriter struct {
	client    StatusWriter
	namespace string
}

// Update implements client.StatusWriter
func (nsw *namespacedClientStatusWriter) Update(ctx context.Context, obj runtime.Object, opts ...UpdateOption) error {
	metaObj, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	if nsw.namespace != "" && metaObj.GetNamespace() != nsw.namespace {
		metaObj.SetNamespace(nsw.namespace)
	}
	return nsw.client.Update(ctx, obj, opts...)
}

// Patch implements client.StatusWriter
func (nsw *namespacedClientStatusWriter) Patch(ctx context.Context, obj runtime.Object, patch Patch, opts ...PatchOption) error {
	metaObj, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	if nsw.namespace != "" && metaObj.GetNamespace() != nsw.namespace {
		metaObj.SetNamespace(nsw.namespace)
	}
	return nsw.client.Patch(ctx, obj, patch, opts...)
}
