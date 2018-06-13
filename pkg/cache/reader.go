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

package cache

import (
	"context"
	"fmt"
	"reflect"

	"github.com/kubernetes-sigs/controller-runtime/pkg/client"
	"github.com/kubernetes-sigs/controller-runtime/pkg/client/apiutil"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ client.ReadInterface = &informerCache{}

// Get implements ReadInterface
func (ip *informerCache) Get(ctx context.Context, key client.ObjectKey, out runtime.Object) error {
	gvk, err := apiutil.GVKForObject(out, ip.Scheme)
	if err != nil {
		return err
	}

	cache, err := ip.InformersMap.Get(gvk, out)
	if err != nil {
		return err
	}
	return cache.Reader.Get(ctx, key, out)
}

// List implements ReadInterface
func (ip *informerCache) List(ctx context.Context, opts *client.ListOptions, out runtime.Object) error {
	itemsPtr, err := apimeta.GetItemsPtr(out)
	if err != nil {
		return nil
	}

	// http://knowyourmeme.com/memes/this-is-fine
	outType := reflect.Indirect(reflect.ValueOf(itemsPtr)).Type().Elem()
	cacheType, ok := outType.(runtime.Object)
	if !ok {
		return fmt.Errorf("cannot get cache for %T, its element is not a runtime.Object", out)
	}

	gvk, err := apiutil.GVKForObject(out, ip.Scheme)
	if err != nil {
		return err
	}

	cache, err := ip.InformersMap.Get(gvk, cacheType)
	if err != nil {
		return err
	}

	return cache.Reader.List(ctx, opts, out)
}
