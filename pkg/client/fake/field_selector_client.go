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

package fake

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

var _ client.Client = &fieldSelectorEnableClient{}

// NewFieldSelectorEnableClient wrapper a fake client that List method enables field selector
func NewFieldSelectorEnableClient(client client.Client, scheme *runtime.Scheme, obj runtime.Object, indexers map[string]client.IndexerFunc) (client.Client, error) {
	gvk, err := apiutil.GVKForObject(obj, scheme)
	if err != nil {
		return nil, err
	}

	return &fieldSelectorEnableClient{
		Client:   client,
		scheme:   scheme,
		gvk:      gvk,
		indexers: indexers,
	}, nil
}

type fieldSelectorEnableClient struct {
	client.Client
	scheme *runtime.Scheme

	gvk schema.GroupVersionKind

	indexers map[string]client.IndexerFunc
}

func (f *fieldSelectorEnableClient) List(ctx context.Context, list runtime.Object, opts ...client.ListOption) error {
	if err := f.Client.List(ctx, list, opts...); err != nil {
		return err
	}

	if len(opts) == 0 {
		return nil
	}

	objs, err := meta.ExtractList(list)
	if err != nil {
		return err
	}

	if len(objs) == 0 {
		return nil
	}

	if gvk, err := apiutil.GVKForObject(objs[0], f.scheme); err != nil {
		return err
	} else if gvk != f.gvk {
		return nil
	}

	options := client.ListOptions{}
	options.ApplyOptions(opts)

	if options.FieldSelector == nil {
		return nil
	}

	filtered := make([]runtime.Object, 0)
	for i := range objs {
		objFields := map[string]string{}

		for k, indexer := range f.indexers {
			values := indexer(objs[i])
			if len(values) != 1 {
				continue
			}
			objFields[k] = values[0]
		}

		if options.FieldSelector.Matches(fields.Set(objFields)) {
			filtered = append(filtered, objs[i])
		}
	}

	return meta.SetList(list, filtered)
}
