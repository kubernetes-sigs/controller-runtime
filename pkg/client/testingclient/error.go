package testingclient

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type resourceActionKey struct {
	action    string
	kind      schema.GroupVersionKind
	objectKey client.ObjectKey
}

var (
	AnyAction = "*"
	AnyKind   = schema.GroupVersionKind{Kind: "*"}
	AnyObject = client.ObjectKey{}
)

type ErrorInjector struct {
	delegate       client.Client
	errorsToReturn map[resourceActionKey]error
}

var _ client.Client = ErrorInjector{}

func NewErrorInjector(cl client.Client) *ErrorInjector {
	injectedErrors := make(map[resourceActionKey]error)

	return &ErrorInjector{
		delegate:       cl,
		errorsToReturn: injectedErrors,
	}
}

func (c ErrorInjector) Scheme() *runtime.Scheme {
	return c.delegate.Scheme()
}

func (c ErrorInjector) RESTMapper() meta.RESTMapper {
	return c.delegate.RESTMapper()
}

func (c ErrorInjector) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	err := c.getStubbedError("get", obj, key)
	if err != nil {
		return err
	}

	return c.delegate.Get(ctx, key, obj)
}

func (c ErrorInjector) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	panic("implement me")
}

func (c ErrorInjector) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	err := c.getStubbedError("create", obj, client.ObjectKeyFromObject(obj))
	if err != nil {
		return err
	}

	return c.delegate.Create(ctx, obj, opts...)
}

func (c ErrorInjector) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	err := c.getStubbedError("delete", obj, client.ObjectKeyFromObject(obj))
	if err != nil {
		return err
	}

	return c.delegate.Delete(ctx, obj, opts...)
}

func (c ErrorInjector) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	err := c.getStubbedError("update", obj, client.ObjectKeyFromObject(obj))
	if err != nil {
		return err
	}

	return c.delegate.Update(ctx, obj, opts...)
}

func (c ErrorInjector) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	err := c.getStubbedError("patch", obj, client.ObjectKeyFromObject(obj))
	if err != nil {
		return err
	}

	return c.delegate.Patch(ctx, obj, patch, opts...)
}

func (c ErrorInjector) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	panic("implement me")
}

func (c ErrorInjector) Status() client.StatusWriter {
	return c
}

func (c ErrorInjector) getStubbedError(action string, kind client.Object, objectKey client.ObjectKey) error {
	gvk := mustGVKForObject(kind, c.Scheme())

	for _, k := range []resourceActionKey{
		{action, gvk, objectKey},
		{action, gvk, AnyObject},
		{AnyAction, gvk, objectKey},
		{AnyAction, gvk, AnyObject},
		{action, AnyKind, AnyObject},
		{AnyAction, AnyKind, AnyObject},
	} {
		if err, ok := c.errorsToReturn[k]; ok {
			return err
		}
	}
	return nil
}

// InjectError will cause ErrorInjector to return an error for the given (action, kind, objectKey) tuple.
// To inject an error for all calls with `(action,kind)`, pass objectKey = AnyObject.
// To inject an error for all calls with `action`, pass kind = "*" and objectKey = AnyObject.
// To inject an error for any `action`, pass action = "*",  kind = "*" and objectKey = AnyObject.
func (c *ErrorInjector) InjectError(action string, kind client.Object, objectKey client.ObjectKey, injectedError error) {
	gvk := mustGVKForObject(kind, c.Scheme())
	c.errorsToReturn[resourceActionKey{action, gvk, objectKey}] = injectedError
}
