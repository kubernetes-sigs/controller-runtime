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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// NewNamespacedClient wraps an existing client enforcing the namespace value.
// All functions using this client will have the same namespace declared here.
func NewNamespacedClient(c Client, ns string) Client {
	return &namespacedClient{
		client:    c,
		namespace: ns,
	}
}

var _ Client = &namespacedClient{}

// namespacedClient is a Client that wraps another Client in order to enforce the specified namespace value.
type namespacedClient struct {
	namespace string
	client    Client
}

// Scheme returns the scheme this client is using.
func (n *namespacedClient) Scheme() *runtime.Scheme {
	return n.client.Scheme()
}

// RESTMapper returns the scheme this client is using.
func (n *namespacedClient) RESTMapper() meta.RESTMapper {
	return n.client.RESTMapper()
}

// isNamespaced returns true if the object is namespace scoped.
// For unstructured objects the gvk is found from the object itself.
func isNamespaced(c Client, obj runtime.Object) (bool, error) {
	var gvk schema.GroupVersionKind
	var err error

	_, isUnstructured := obj.(*unstructured.Unstructured)
	_, isUnstructuredList := obj.(*unstructured.UnstructuredList)

	isUnstructured = isUnstructured || isUnstructuredList
	if isUnstructured {
		gvk = obj.GetObjectKind().GroupVersionKind()
	} else {
		gvk, err = apiutil.GVKForObject(obj, c.Scheme())
		if err != nil {
			return false, err
		}
	}

	gk := schema.GroupKind{
		Group: gvk.Group,
		Kind:  gvk.Kind,
	}
	restmapping, err := c.RESTMapper().RESTMapping(gk)
	if err != nil {
		return false, fmt.Errorf("failed to get restmapping: %w", err)
	}
	scope := restmapping.Scope.Name()

	if scope == "" {
		return false, fmt.Errorf("Scope cannot be identified. Empty scope returned")
	}

	if scope != meta.RESTScopeNameRoot {
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

	isNamespaceScoped, err := isNamespaced(n.client, obj)
	if err != nil {
		return fmt.Errorf("error finding the scope of the object %v", err)
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

	isNamespaceScoped, err := isNamespaced(n.client, obj)
	if err != nil {
		return fmt.Errorf("error finding the scope of the object %v", err)
	}

	if isNamespaceScoped && metaObj.GetNamespace() != n.namespace {
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

	isNamespaceScoped, err := isNamespaced(n.client, obj)
	if err != nil {
		return fmt.Errorf("error finding the scope of the object %v", err)
	}

	if isNamespaceScoped && metaObj.GetNamespace() != n.namespace {
		metaObj.SetNamespace(n.namespace)
	}
	return n.client.Delete(ctx, obj, opts...)
}

// DeleteAllOf implements client.Client
func (n *namespacedClient) DeleteAllOf(ctx context.Context, obj runtime.Object, opts ...DeleteAllOfOption) error {
	isNamespaceScoped, err := isNamespaced(n.client, obj)
	if err != nil {
		return fmt.Errorf("error finding the scope of the object %v", err)
	}

	if isNamespaceScoped {
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

	isNamespaceScoped, err := isNamespaced(n.client, obj)
	if err != nil {
		return fmt.Errorf("error finding the scope of the object %v", err)
	}

	if isNamespaceScoped && metaObj.GetNamespace() != n.namespace {
		metaObj.SetNamespace(n.namespace)
	}
	return n.client.Patch(ctx, obj, patch, opts...)
}

// Get implements client.Client
func (n *namespacedClient) Get(ctx context.Context, key ObjectKey, obj runtime.Object) error {
	isNamespaceScoped, err := isNamespaced(n.client, obj)
	if err != nil {
		return fmt.Errorf("error finding the scope of the object %v", err)
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
	return &namespacedClientStatusWriter{StatusClient: n.client.Status(), namespace: n.namespace, namespacedclient: n}
}

// ensure namespacedClientStatusWriter implements client.StatusWriter
var _ StatusWriter = &namespacedClientStatusWriter{}

type namespacedClientStatusWriter struct {
	StatusClient     StatusWriter
	namespace        string
	namespacedclient Client
}

// Update implements client.StatusWriter
func (nsw *namespacedClientStatusWriter) Update(ctx context.Context, obj runtime.Object, opts ...UpdateOption) error {
	metaObj, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	isNamespaceScoped, err := isNamespaced(nsw.namespacedclient, obj)
	if err != nil {
		return fmt.Errorf("error finding the scope of the object %v", err)
	}

	if isNamespaceScoped && metaObj.GetNamespace() != nsw.namespace {
		metaObj.SetNamespace(nsw.namespace)
	}
	return nsw.StatusClient.Update(ctx, obj, opts...)
}

// Patch implements client.StatusWriter
func (nsw *namespacedClientStatusWriter) Patch(ctx context.Context, obj runtime.Object, patch Patch, opts ...PatchOption) error {
	metaObj, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	isNamespaceScoped, err := isNamespaced(nsw.namespacedclient, obj)
	if err != nil {
		return fmt.Errorf("error finding the scope of the object %v", err)
	}

	if isNamespaceScoped && metaObj.GetNamespace() != nsw.namespace {
		metaObj.SetNamespace(nsw.namespace)
	}
	return nsw.StatusClient.Patch(ctx, obj, patch, opts...)
}
