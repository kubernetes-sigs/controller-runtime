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

package main

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ client.Client = &NamespacedClient{}

type NamespacedClient struct {
	clusterName string
	client.Client
}

func (n *NamespacedClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if key.Namespace != "default" {
		return apierrors.NewNotFound(schema.GroupResource{}, key.Name)
	}
	key.Namespace = n.clusterName
	if err := n.Client.Get(ctx, key, obj, opts...); err != nil {
		return err
	}
	obj.SetNamespace("default")
	return nil
}

func (n *NamespacedClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	var copts client.ListOptions
	for _, o := range opts {
		o.ApplyToList(&copts)
	}
	if copts.Namespace != "default" {
		return apierrors.NewNotFound(schema.GroupResource{}, copts.Namespace)
	}
	if err := n.Client.List(ctx, list, append(opts, client.InNamespace(n.clusterName))...); err != nil {
		return err
	}
	return meta.EachListItem(list, func(obj runtime.Object) error {
		obj.(client.Object).SetNamespace("default")
		return nil
	})
}

func (n *NamespacedClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	panic("implement me")
}

func (n *NamespacedClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	panic("implement me")
}

func (n *NamespacedClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	panic("implement me")
}

func (n *NamespacedClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	panic("implement me")
}

func (n *NamespacedClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	panic("implement me")
}

func (n *NamespacedClient) Status() client.SubResourceWriter {
	panic("implement me")
}

func (n *NamespacedClient) SubResource(subResource string) client.SubResourceClient {
	panic("implement me")
}
