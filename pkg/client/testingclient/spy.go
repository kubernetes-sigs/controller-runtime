package testingclient

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

type SpyCall struct {
	GVK      schema.GroupVersionKind
	IsStatus bool
	Verb     string
	Key      client.ObjectKey
	Obj      client.Object
	List     client.ObjectList
	Patch    client.Patch
}

type Spy struct {
	Delegate client.Client
	Calls    chan<- SpyCall
	isStatus bool
}

var _ client.Client = Spy{}

func (s Spy) Scheme() *runtime.Scheme {
	return s.Delegate.Scheme()
}

func (s Spy) RESTMapper() meta.RESTMapper {
	return s.Delegate.RESTMapper()
}

func (s Spy) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	err := s.Delegate.Get(ctx, key, obj)
	s.Calls <- SpyCall{
		GVK:  mustGVKForObject(obj, s.Scheme()),
		Verb: "get",
		Key:  key,
		Obj:  obj.DeepCopyObject().(client.Object),
	}
	return err
}

func (s Spy) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	gvk, err := listGVK(list, s.Scheme())
	if err != nil {
		return err
	}

	listErr := s.Delegate.List(ctx, list, opts...)
	s.Calls <- SpyCall{
		GVK:  gvk,
		Verb: "list",
		List: list.DeepCopyObject().(client.ObjectList),
	}
	return listErr
}

func (s Spy) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	err := s.Delegate.Create(ctx, obj, opts...)
	s.Calls <- SpyCall{
		GVK:  mustGVKForObject(obj, s.Scheme()),
		Verb: "create",
		Obj:  obj.DeepCopyObject().(client.Object),
	}
	return err
}

func (s Spy) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	err := s.Delegate.Delete(ctx, obj, opts...)
	s.Calls <- SpyCall{
		GVK:  mustGVKForObject(obj, s.Scheme()),
		Verb: "delete",
		Obj:  obj.DeepCopyObject().(client.Object),
	}
	return err
}

func (s Spy) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	err := s.Delegate.Update(ctx, obj, opts...)
	s.Calls <- SpyCall{
		GVK:      mustGVKForObject(obj, s.Scheme()),
		IsStatus: s.isStatus,
		Verb:     "update",
		Obj:      obj.DeepCopyObject().(client.Object),
	}
	return err
}

func (s Spy) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	err := s.Delegate.Patch(ctx, obj, patch, opts...)
	s.Calls <- SpyCall{
		GVK:      mustGVKForObject(obj, s.Scheme()),
		IsStatus: s.isStatus,
		Verb:     "patch",
		Obj:      obj.DeepCopyObject().(client.Object),
		Patch:    patch,
	}
	return err
}

func (s Spy) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	err := s.Delegate.DeleteAllOf(ctx, obj, opts...)
	s.Calls <- SpyCall{
		GVK:  mustGVKForObject(obj, s.Scheme()),
		Verb: "deleteallof",
	}
	return err
}

func (s Spy) Status() client.StatusWriter {
	s.isStatus = true
	return s
}

func mustGVKForObject(obj runtime.Object, scheme *runtime.Scheme) schema.GroupVersionKind {
	gvk, err := apiutil.GVKForObject(obj, scheme)
	if err != nil {
		panic(fmt.Errorf("couldn't look up GVK for object (check Scheme): %w", err))
	}
	return gvk
}

func listGVK(list client.ObjectList, scheme *runtime.Scheme) (schema.GroupVersionKind, error) {
	listGvk := mustGVKForObject(list, scheme)

	if !strings.HasSuffix(listGvk.Kind, "List") {
		return schema.GroupVersionKind{}, fmt.Errorf("non-list type %T (kind %q) passed as output", list, listGvk)
	}
	// we need the non-list GVK, so chop off the "List" from the end of the kind
	gvk := listGvk
	gvk.Kind = gvk.Kind[:len(gvk.Kind)-len("List")]
	return gvk, nil
}
