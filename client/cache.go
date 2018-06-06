package client

import (
	"context"
	"fmt"
	"reflect"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/tools/cache"

	logf "github.com/kubernetes-sigs/kubebuilder/pkg/log"
	"github.com/kubernetes-sigs/kubebuilder/pkg/informer"
)

var log = logf.KBLog.WithName("object-cache")

// ObjectCache is a ReadInterface
var _ ReadInterface = &ObjectCache{}

type ObjectCache struct {
	cachesByType map[reflect.Type]*SingleObjectCache
}

func ObjectCacheFromInformers(informers map[schema.GroupVersionKind]cache.SharedIndexInformer, scheme *runtime.Scheme) *ObjectCache {
	res := NewObjectCache()
	for gvk, informer := range informers {
		obj, err := scheme.New(gvk)
		if err != nil {
			log.Error(err, "could not register informer in ObjectCache for GVK", "GroupVersionKind", gvk)
			continue
		}
		res.RegisterCache(obj, gvk, informer.GetIndexer())
	}
	return res
}

func NewObjectCache() *ObjectCache {
	return &ObjectCache{
		cachesByType: make(map[reflect.Type]*SingleObjectCache),
	}
}

func (c *ObjectCache) RegisterCache(obj runtime.Object, gvk schema.GroupVersionKind, store cache.Indexer) {
	objType := reflect.TypeOf(obj)
	c.cachesByType[objType] = &SingleObjectCache{
		Indexer: store,
		GroupVersionKind: gvk,
	}
}

func (c *ObjectCache) CacheFor(obj runtime.Object) (*SingleObjectCache, bool) {
	objType := reflect.TypeOf(obj)
	cache, isKnown := c.cachesByType[objType]
	return cache, isKnown
}

func (c *ObjectCache) Get(ctx context.Context, key ObjectKey, out runtime.Object) error {
	cache, isKnown := c.CacheFor(out)
	if !isKnown {
		return fmt.Errorf("no cache for objects of type %T, must have asked for an watch/informer first", out)
	}
	return cache.Get(ctx, key, out)
}

func (c *ObjectCache) List(ctx context.Context, opts *ListOptions, out runtime.Object) error {
	itemsPtr, err := apimeta.GetItemsPtr(out)
	if err != nil {
		return nil
	}
	// http://knowyourmeme.com/memes/this-is-fine
	outType := reflect.Indirect(reflect.ValueOf(itemsPtr)).Type().Elem()
	cache, isKnown := c.cachesByType[outType]
	if !isKnown {
		return fmt.Errorf("no cache for objects of type %T", out)
	}
	return cache.List(ctx, opts, out)
}

// SingleObjectCache is a ReadInterface
var _ ReadInterface = &SingleObjectCache{}

// SingleObjectCache is a ReadInterface that retrieves objects
// from a single local cache populated by a watch.
type SingleObjectCache struct {
	// Indexer is the underlying indexer wrapped by this cache.
	Indexer cache.Indexer
	// GroupVersionKind is the group-version-kind of the resource.
	GroupVersionKind schema.GroupVersionKind
}

func (c *SingleObjectCache) Get(_ context.Context, key ObjectKey, out runtime.Object) error {
	storeKey := objectKeyToStoreKey(key)
	obj, exists, err := c.Indexer.GetByKey(storeKey)
	if err != nil {
		return err
	}
	if !exists {
		// Resource gets transformed into Kind in the error anyway, so this is fine
		return errors.NewNotFound(schema.GroupResource{
			Group: c.GroupVersionKind.Group,
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

func (c *SingleObjectCache) List(ctx context.Context, opts *ListOptions, out runtime.Object) error {
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

	outItems := make([]runtime.Object, 0, len(objs))
	for _, item := range objs {
		obj, isObj := item.(runtime.Object)
		if !isObj {
			return fmt.Errorf("cache contained %T, which is not an Object", obj)
		}
		meta, err := apimeta.Accessor(obj)
		if err != nil {
			return err
		}
		if labelSel != nil {
			lbls := labels.Set(meta.GetLabels())
			if !labelSel.Matches(lbls) {
				continue
			}
		}
		outItems = append(outItems, obj.DeepCopyObject())
	}
	if err := apimeta.SetList(out, outItems); err != nil {
		return err
	}
	return nil
}

// TODO: Make an interface with this function that has an Informers as an object on the struct
// that automatically calls InformerFor and passes in the Indexer into IndexByField

// noNamespaceNamespace is used as the "namespace" when we want to list across all namespaces
const allNamespacesNamespace = "__all_namespaces"

type InformerFieldIndexer struct {
	Informers informer.Informers
}

func (i *InformerFieldIndexer) IndexField(obj runtime.Object, field string, extractValue IndexerFunc) error {
	informer, err := i.Informers.InformerFor(obj)
	if err != nil {
		return err
	}
	return IndexByField(informer.GetIndexer(), field, extractValue)
}

// IndexByField adds an indexer to the underlying cache, using extraction function to get
// value(s) from the given field.  This index can then be used by passing a field selector
// to List. For one-to-one compatibility with "normal" field selectors, only return one value.
// The values may be anything.  They will automatically be prefixed with the namespace of the
// given object, if present.  The objects passed are guaranteed to be objects of the correct type.
func IndexByField(indexer cache.Indexer, field string, extractor IndexerFunc) error {
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

	if err := indexer.AddIndexers(cache.Indexers{fieldIndexName(field): indexFunc}); err != nil {
		return err
	}
	return nil
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
// It's akin to MetaNamespaceKeyFunc.  It's seperate from
// String to allow keeping the key format easily in sync with
// MetaNamespaceKeyFunc.
func objectKeyToStoreKey(k ObjectKey) string {
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

// SplitReaderWriter forms an interface Interface by composing separate
// read and write interfaces.  This way, you can have an Interface that
// reads from a cache and writes to the API server.
type SplitReaderWriter struct {
	ReadInterface
	WriteInterface
}
