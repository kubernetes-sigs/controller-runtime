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

	"k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/tools/cache"

	"github.com/kubernetes-sigs/controller-runtime/pkg/client"
	logf "github.com/kubernetes-sigs/controller-runtime/pkg/runtime/log"
	toolscache "k8s.io/client-go/tools/cache"
)

var log = logf.KBLog.WithName("object-cache")

// objectCache is a ReadInterface
var _ client.ReadInterface = &objectCache{}

// objectCache is a Kubernetes Object cache populated from Informers
type objectCache struct {
	cachesByType map[reflect.Type]*singleObjectCache
	scheme       *runtime.Scheme
	informers    *informers
}

var _ client.ReadInterface = &objectCache{}

// addInformer adds an informer to the objectCache
func (o *objectCache) addInformer(gvk schema.GroupVersionKind, c cache.SharedIndexInformer) {
	obj, err := o.scheme.New(gvk)
	if err != nil {
		log.Error(err, "could not register informer in objectCache for GVK", "GroupVersionKind", gvk)
		return
	}
	if o.has(obj) {
		return
	}
	o.registerCache(obj, gvk, c.GetIndexer())
}

func (o *objectCache) registerCache(obj runtime.Object, gvk schema.GroupVersionKind, store cache.Indexer) {
	objType := reflect.TypeOf(obj)
	o.cachesByType[objType] = &singleObjectCache{
		Indexer:          store,
		GroupVersionKind: gvk,
	}
}

func (o *objectCache) has(obj runtime.Object) bool {
	objType := reflect.TypeOf(obj)
	_, found := o.cachesByType[objType]
	return found
}

func (o *objectCache) init(obj runtime.Object) error {
	i, err := o.informers.GetInformer(obj)
	if err != nil {
		return err
	}
	if o.informers.started {
		log.Info("Waiting to sync cache for type.", "Type", fmt.Sprintf("%T", obj))
		toolscache.WaitForCacheSync(o.informers.stop, i.HasSynced)
		log.Info("Finished to syncing cache for type.", "Type", fmt.Sprintf("%T", obj))
	} else {
		return fmt.Errorf("must start Cache before calling Get or List %s %s",
			"Object", fmt.Sprintf("%T", obj))
	}
	return nil
}

func (o *objectCache) cacheFor(obj runtime.Object) (*singleObjectCache, error) {
	if !o.informers.started {
		return nil, fmt.Errorf("must start Cache before calling Get or List %s %s",
			"Object", fmt.Sprintf("%T", obj))
	}
	objType := reflect.TypeOf(obj)

	cache, isKnown := o.cachesByType[objType]
	if !isKnown {
		return nil, fmt.Errorf("no Cache found for %T - must call GetInformer", obj)
	}
	return cache, nil
}

// Get implements populatingClient.ReadInterface
func (o *objectCache) Get(ctx context.Context, key client.ObjectKey, out runtime.Object) error {
	// Make sure there is a Cache for this type
	if !o.has(out) {
		err := o.init(out)
		if err != nil {
			return err
		}
	}

	cache, err := o.cacheFor(out)
	if err != nil {
		return err
	}

	return cache.Get(ctx, key, out)
}

// List implements populatingClient.ReadInterface
func (o *objectCache) List(ctx context.Context, opts *client.ListOptions, out runtime.Object) error {
	itemsPtr, err := apimeta.GetItemsPtr(out)
	if err != nil {
		return nil
	}

	ro, ok := itemsPtr.(runtime.Object)
	if ok && !o.has(ro) {
		err = o.init(ro)
		if err != nil {
			return err
		}
	}

	// http://knowyourmeme.com/memes/this-is-fine
	outType := reflect.Indirect(reflect.ValueOf(itemsPtr)).Type().Elem()
	cache, isKnown := o.cachesByType[outType]
	if !isKnown {
		return fmt.Errorf("no cache for objects of type %T", out)
	}
	return cache.List(ctx, opts, out)
}

// singleObjectCache is a ReadInterface
var _ client.ReadInterface = &singleObjectCache{}

// singleObjectCache is a ReadInterface that retrieves objects
// from a single local cache populated by a watch.
type singleObjectCache struct {
	// Indexer is the underlying indexer wrapped by this cache.
	Indexer cache.Indexer
	// GroupVersionKind is the group-version-kind of the resource.
	GroupVersionKind schema.GroupVersionKind
}

// Get implements populatingClient.Client
func (c *singleObjectCache) Get(_ context.Context, key client.ObjectKey, out runtime.Object) error {
	storeKey := objectKeyToStoreKey(key)
	obj, exists, err := c.Indexer.GetByKey(storeKey)
	if err != nil {
		return err
	}
	if !exists {
		// Resource gets transformed into Kind in the error anyway, so this is fine
		return errors.NewNotFound(schema.GroupResource{
			Group:    c.GroupVersionKind.Group,
			Resource: c.GroupVersionKind.Kind,
		}, key.Name)
	}
	if _, isObj := obj.(runtime.Object); !isObj {
		return fmt.Errorf("cache contained %T, which is not an Object", obj)
	}

	// deep copy to avoid mutating cache
	// TODO(directxman12): revisit the decision to always deepcopy
	obj = obj.(runtime.Object).DeepCopyObject()

	// TODO(directxman12): this is a terrible hack, pls fix
	// (we should have deepcopyinto)
	outVal := reflect.ValueOf(out)
	objVal := reflect.ValueOf(obj)
	if !objVal.Type().AssignableTo(outVal.Type()) {
		return fmt.Errorf("cache had type %s, but %s was asked for", objVal.Type(), outVal.Type())
	}
	reflect.Indirect(outVal).Set(reflect.Indirect(objVal))
	return nil
}

// List implements populatingClient.Client
func (c *singleObjectCache) List(ctx context.Context, opts *client.ListOptions, out runtime.Object) error {
	var objs []interface{}
	var err error

	if opts != nil && opts.FieldSelector != nil {
		// TODO(directxman12): support more complicated field selectors by
		// combining multiple indicies, GetIndexers, etc
		field, val, requiresExact := requiresExactMatch(opts.FieldSelector)
		if !requiresExact {
			return fmt.Errorf("non-exact field matches are not supported by the cache")
		}
		// list all objects by the field selector.  If this is namespaced and we have one, ask for the
		// namespaced index key.  Otherwise, ask for the non-namespaced variant by using the fake "all namespaces"
		// namespace.
		objs, err = c.Indexer.ByIndex(fieldIndexName(field), keyToNamespacedKey(opts.Namespace, val))
	} else if opts != nil && opts.Namespace != "" {
		objs, err = c.Indexer.ByIndex(cache.NamespaceIndex, opts.Namespace)
	} else {
		objs = c.Indexer.List()
	}
	if err != nil {
		return err
	}
	var labelSel labels.Selector
	if opts != nil && opts.LabelSelector != nil {
		labelSel = opts.LabelSelector
	}

	outItems, err := c.getListItems(objs, labelSel)
	if err != nil {
		return err
	}
	return apimeta.SetList(out, outItems)
}

func (c *singleObjectCache) getListItems(objs []interface{}, labelSel labels.Selector) ([]runtime.Object, error) {
	outItems := make([]runtime.Object, 0, len(objs))
	for _, item := range objs {
		obj, isObj := item.(runtime.Object)
		if !isObj {
			return nil, fmt.Errorf("cache contained %T, which is not an Object", obj)
		}
		meta, err := apimeta.Accessor(obj)
		if err != nil {
			return nil, err
		}
		if labelSel != nil {
			lbls := labels.Set(meta.GetLabels())
			if !labelSel.Matches(lbls) {
				continue
			}
		}
		outItems = append(outItems, obj.DeepCopyObject())
	}
	return outItems, nil
}

// TODO: Make an interface with this function that has an Informers as an object on the struct
// that automatically calls GetInformer and passes in the Indexer into indexByField

// noNamespaceNamespace is used as the "namespace" when we want to list across all namespaces
const allNamespacesNamespace = "__all_namespaces"

// IndexField adds an indexer to the underlying cache, using extraction function to get
// value(s) from the given field.  This index can then be used by passing a field selector
// to List. For one-to-one compatibility with "normal" field selectors, only return one value.
// The values may be anything.  They will automatically be prefixed with the namespace of the
// given object, if present.  The objects passed are guaranteed to be objects of the correct type.
func (i *informers) IndexField(obj runtime.Object, field string, extractValue client.IndexerFunc) error {
	informer, err := i.GetInformer(obj)
	if err != nil {
		return err
	}
	return indexByField(informer.GetIndexer(), field, extractValue)
}

func indexByField(indexer cache.Indexer, field string, extractor client.IndexerFunc) error {
	indexFunc := func(objRaw interface{}) ([]string, error) {
		// TODO(directxman12): check if this is the correct type?
		obj, isObj := objRaw.(runtime.Object)
		if !isObj {
			return nil, fmt.Errorf("object of type %T is not an Object", objRaw)
		}
		meta, err := apimeta.Accessor(obj)
		if err != nil {
			return nil, err
		}
		ns := meta.GetNamespace()

		rawVals := extractor(obj)
		var vals []string
		if ns == "" {
			// if we're not doubling the keys for the namespaced case, just re-use what was returned to us
			vals = rawVals
		} else {
			// if we need to add non-namespaced versions too, double the length
			vals = make([]string, len(rawVals)*2)
		}
		for i, rawVal := range rawVals {
			// save a namespaced variant, so that we can ask
			// "what are all the object matching a given index *in a given namespace*"
			vals[i] = keyToNamespacedKey(ns, rawVal)
			if ns != "" {
				// if we have a namespace, also inject a special index key for listing
				// regardless of the object namespace
				vals[i+len(rawVals)] = keyToNamespacedKey("", rawVal)
			}
		}

		return vals, nil
	}

	return indexer.AddIndexers(cache.Indexers{fieldIndexName(field): indexFunc})
}

// fieldIndexName constructs the name of the index over the given field,
// for use with an Indexer.
func fieldIndexName(field string) string {
	return "field:" + field
}

// keyToNamespacedKey prefixes the given index key with a namespace
// for use in field selector indexes.
func keyToNamespacedKey(ns string, baseKey string) string {
	if ns != "" {
		return ns + "/" + baseKey
	}
	return allNamespacesNamespace + "/" + baseKey
}

// objectKeyToStorageKey converts an object key to store key.
// It's akin to MetaNamespaceKeyFunc.  It's separate from
// String to allow keeping the key format easily in sync with
// MetaNamespaceKeyFunc.
func objectKeyToStoreKey(k client.ObjectKey) string {
	if k.Namespace == "" {
		return k.Name
	}
	return k.Namespace + "/" + k.Name
}

// requiresExactMatch checks if the given field selector is of the form `k=v` or `k==v`.
func requiresExactMatch(sel fields.Selector) (field, val string, required bool) {
	reqs := sel.Requirements()
	if len(reqs) != 1 {
		return "", "", false
	}
	req := reqs[0]
	if req.Operator != selection.Equals && req.Operator != selection.DoubleEquals {
		return "", "", false
	}
	return req.Field, req.Value, true
}
