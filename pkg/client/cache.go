package client

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

	"github.com/kubernetes-sigs/controller-runtime/pkg/internal/informer"
	logf "github.com/kubernetes-sigs/controller-runtime/pkg/runtime/log"
)

var log = logf.KBLog.WithName("object-cache")

// objectCache is a ReadInterface
var _ ReadInterface = &objectCache{}

// objectCache is a Kubernetes Object cache populated from Informers
type objectCache struct {
	cachesByType map[reflect.Type]*singleObjectCache
	scheme       *runtime.Scheme
}

var _ Cache = &objectCache{}

// Cache implements ReadInterface by reading objects from a cache populated by Informers
type Cache interface {
	ReadInterface
	informer.Callback
}

// NewObjectCache returns a new objectCache populated from informers
func NewObjectCache(
	informers map[schema.GroupVersionKind]cache.SharedIndexInformer,
	scheme *runtime.Scheme) Cache {
	res := &objectCache{
		cachesByType: make(map[reflect.Type]*singleObjectCache),
		scheme:       scheme,
	}
	res.AddInformers(informers)
	return res
}

// AddInformers adds new informers to the objectCache
func (o *objectCache) AddInformers(informers map[schema.GroupVersionKind]cache.SharedIndexInformer) {
	if informers == nil {
		return
	}
	for gvk, informer := range informers {
		o.AddInformer(gvk, informer)
	}
}

// Call implements the informer.Callback so that the cache can be populate with new Informers as they are added
func (o *objectCache) Call(gvk schema.GroupVersionKind, c cache.SharedIndexInformer) {
	o.AddInformer(gvk, c)
}

// AddInformer adds an informer to the objectCache
func (o *objectCache) AddInformer(gvk schema.GroupVersionKind, c cache.SharedIndexInformer) {
	obj, err := o.scheme.New(gvk)
	if err != nil {
		log.Error(err, "could not register informer in objectCache for GVK", "GroupVersionKind", gvk)
		return
	}
	if _, found := o.cacheFor(obj); found {
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

func (o *objectCache) cacheFor(obj runtime.Object) (*singleObjectCache, bool) {
	objType := reflect.TypeOf(obj)
	cache, isKnown := o.cachesByType[objType]
	return cache, isKnown
}

// Get implements client.ReadInterface
func (o *objectCache) Get(ctx context.Context, key ObjectKey, out runtime.Object) error {
	cache, isKnown := o.cacheFor(out)
	if !isKnown {
		return fmt.Errorf("no cache for objects of type %T, must have asked for an watch/informer first", out)
	}
	return cache.Get(ctx, key, out)
}

// List implements client.ReadInterface
func (o *objectCache) List(ctx context.Context, opts *ListOptions, out runtime.Object) error {
	itemsPtr, err := apimeta.GetItemsPtr(out)
	if err != nil {
		return nil
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
var _ ReadInterface = &singleObjectCache{}

// singleObjectCache is a ReadInterface that retrieves objects
// from a single local cache populated by a watch.
type singleObjectCache struct {
	// Indexer is the underlying indexer wrapped by this cache.
	Indexer cache.Indexer
	// GroupVersionKind is the group-version-kind of the resource.
	GroupVersionKind schema.GroupVersionKind
}

// Get implements client.Interface
func (c *singleObjectCache) Get(_ context.Context, key ObjectKey, out runtime.Object) error {
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

// List implements client.Interface
func (c *singleObjectCache) List(ctx context.Context, opts *ListOptions, out runtime.Object) error {
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
	return apimeta.SetList(out, outItems)
}

// TODO: Make an interface with this function that has an Informers as an object on the struct
// that automatically calls InformerFor and passes in the Indexer into indexByField

// noNamespaceNamespace is used as the "namespace" when we want to list across all namespaces
const allNamespacesNamespace = "__all_namespaces"

// InformerFieldIndexer provides an in-memory index of Object fields
type InformerFieldIndexer struct {
	Informers informer.Informers
}

// IndexField adds an indexer to the underlying cache, using extraction function to get
// value(s) from the given field.  This index can then be used by passing a field selector
// to List. For one-to-one compatibility with "normal" field selectors, only return one value.
// The values may be anything.  They will automatically be prefixed with the namespace of the
// given object, if present.  The objects passed are guaranteed to be objects of the correct type.
func (i *InformerFieldIndexer) IndexField(obj runtime.Object, field string, extractValue IndexerFunc) error {
	informer, err := i.Informers.InformerFor(obj)
	if err != nil {
		return err
	}
	return indexByField(informer.GetIndexer(), field, extractValue)
}

func indexByField(indexer cache.Indexer, field string, extractor IndexerFunc) error {
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
