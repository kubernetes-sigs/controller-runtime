package testingclient

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"sigs.k8s.io/controller-runtime/pkg/client"
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
	err := c.getStubbedError(GetVerb, mustGVKForObject(obj, c.Scheme()), key)
	if err != nil {
		return err
	}

	return c.delegate.Get(ctx, key, obj)
}

// List will only match against errors injected for AnyObject.
func (c ErrorInjector) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	gvk, err := listGVK(list, c.Scheme())
	if err != nil {
		return err
	}

	err = c.getStubbedError(ListVerb, gvk, AnyObject)
	if err != nil {
		return err
	}
	return c.delegate.List(ctx, list, opts...)
}

func (c ErrorInjector) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	err := c.getStubbedError(CreateVerb, mustGVKForObject(obj, c.Scheme()), client.ObjectKeyFromObject(obj))
	if err != nil {
		return err
	}

	return c.delegate.Create(ctx, obj, opts...)
}

func (c ErrorInjector) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	err := c.getStubbedError(DeleteVerb, mustGVKForObject(obj, c.Scheme()), client.ObjectKeyFromObject(obj))
	if err != nil {
		return err
	}

	return c.delegate.Delete(ctx, obj, opts...)
}

func (c ErrorInjector) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	err := c.getStubbedError(UpdateVerb, mustGVKForObject(obj, c.Scheme()), client.ObjectKeyFromObject(obj))
	if err != nil {
		return err
	}

	return c.delegate.Update(ctx, obj, opts...)
}

func (c ErrorInjector) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	err := c.getStubbedError(PatchVerb, mustGVKForObject(obj, c.Scheme()), client.ObjectKeyFromObject(obj))
	if err != nil {
		return err
	}

	return c.delegate.Patch(ctx, obj, patch, opts...)
}

// DeleteAllOf will only match against errors injected for AnyObject.
func (c ErrorInjector) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	err := c.getStubbedError(DeleteAllVerb, mustGVKForObject(obj, c.Scheme()), client.ObjectKeyFromObject(obj))
	if err != nil {
		return err
	}

	return c.delegate.DeleteAllOf(ctx, obj, opts...)
}

func (c ErrorInjector) Status() client.StatusWriter {
	return c
}

func (c ErrorInjector) getStubbedError(action Verb, gvk schema.GroupVersionKind, objectKey client.ObjectKey) error {
	for _, k := range []resourceActionKey{
		{action, gvk, objectKey},         // (1) 0 wildcards
		{action, gvk, AnyObject},         // (2) 1 wildcard
		{AnyVerb, gvk, objectKey},        // (3) 1 wildcard
		{action, anyKindGVK, objectKey},  // (4) 1 wildcard
		{AnyVerb, gvk, AnyObject},        // (5) 2 wildcards
		{action, anyKindGVK, AnyObject},  // (6) 2 wildcards
		{AnyVerb, anyKindGVK, objectKey}, // (7) 2 wildcards
		{AnyVerb, anyKindGVK, AnyObject}, // (8) 3 wildcards
	} {
		if err, ok := c.errorsToReturn[k]; ok {
			return err
		}
	}
	return nil
}

// InjectError will cause ErrorInjector to return an error for the given (verb, kind, objectKey) tuple.
// Wildcards are supported for each part of the tuple:
// Pass objectKey = AnyObject to match any object identity.
// Pass kind = AnyKind to match any type of object.
// Pass verb = AnyVerb to match any client verb.
func (c *ErrorInjector) InjectError(action Verb, kind client.Object, objectKey client.ObjectKey, injectedError error) {
	gvk := anyKindGVK
	if kind != AnyKind {
		gvk = mustGVKForObject(kind, c.Scheme())
	}
	c.errorsToReturn[resourceActionKey{action, gvk, objectKey}] = injectedError
}
