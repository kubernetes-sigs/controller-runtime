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

package builder

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Supporting mocking out functions for testing.
var (
	newController = controller.New
	getGvk        = apiutil.GVKForObject
)

// project represents other forms that the we can use to
// send/receive a given resource (metadata-only, unstructured, etc).
type objectProjection int

const (
	// projectAsNormal doesn't change the object from the form given.
	projectAsNormal objectProjection = iota
	// projectAsMetadata turns this into an metadata-only watch.
	projectAsMetadata
)

// Builder builds a Controller.
type Builder struct {
	forInput         forInput
	forInputErr      error
	ownsInput        []watcher
	watchesInput     []watcher
	mgr              manager.Manager
	globalPredicates []predicate.Predicate[client.Object]
	ctrl             controller.Controller
	ctrlOptions      controller.Options
	name             string
}

func newObjectPredicate[T client.ObjectConstraint](p predicate.Predicate[T]) predicate.Predicate[client.Object] {
	return predicate.Funcs[client.Object]{
		CreateFunc: func(e event.CreateEvent[client.Object]) bool {
			ev := event.CreateEvent[T]{Object: e.Object.(T)}
			return p.Create(ev)
		},
		UpdateFunc: func(e event.UpdateEvent[client.Object]) bool {
			ev := event.UpdateEvent[T]{ObjectNew: e.ObjectNew.(T), ObjectOld: e.ObjectOld.(T)}
			return p.Update(ev)
		},
		DeleteFunc: func(e event.DeleteEvent[client.Object]) bool {
			ev := event.DeleteEvent[T]{Object: e.Object.(T), DeleteStateUnknown: e.DeleteStateUnknown}
			return p.Delete(ev)
		},
		GenericFunc: func(e event.GenericEvent[client.Object]) bool {
			ev := event.GenericEvent[T]{Object: e.Object.(T)}
			return p.Generic(ev)
		},
	}
}

func newPredicateFromObject[T client.ObjectConstraint](p predicate.Predicate[client.Object]) predicate.Predicate[T] {
	return predicate.Funcs[T]{
		CreateFunc: func(e event.CreateEvent[T]) bool {
			ev := event.CreateEvent[client.Object]{Object: e.Object}
			return p.Create(ev)
		},
		UpdateFunc: func(e event.UpdateEvent[T]) bool {
			ev := event.UpdateEvent[client.Object]{ObjectNew: e.ObjectNew, ObjectOld: e.ObjectOld}
			return p.Update(ev)
		},
		DeleteFunc: func(e event.DeleteEvent[T]) bool {
			ev := event.DeleteEvent[client.Object]{Object: e.Object, DeleteStateUnknown: e.DeleteStateUnknown}
			return p.Delete(ev)
		},
		GenericFunc: func(e event.GenericEvent[T]) bool {
			ev := event.GenericEvent[client.Object]{Object: e.Object}
			return p.Generic(ev)
		},
	}
}

func newObjectEventHandler[T client.ObjectConstraint](h handler.EventHandler[T]) handler.EventHandler[client.Object] {
	return handler.Funcs[client.Object]{
		CreateFunc: func(ctx context.Context, e event.CreateEvent[client.Object], wq workqueue.RateLimitingInterface) {
			ev := event.CreateEvent[T]{Object: e.Object.(T)}
			h.Create(ctx, ev, wq)
		},
		UpdateFunc: func(ctx context.Context, e event.UpdateEvent[client.Object], wq workqueue.RateLimitingInterface) {
			ev := event.UpdateEvent[T]{ObjectNew: e.ObjectNew.(T), ObjectOld: e.ObjectOld.(T)}
			h.Update(ctx, ev, wq)
		},
		DeleteFunc: func(ctx context.Context, e event.DeleteEvent[client.Object], wq workqueue.RateLimitingInterface) {
			ev := event.DeleteEvent[T]{Object: e.Object.(T), DeleteStateUnknown: e.DeleteStateUnknown}
			h.Delete(ctx, ev, wq)
		},
		GenericFunc: func(ctx context.Context, e event.GenericEvent[client.Object], wq workqueue.RateLimitingInterface) {
			ev := event.GenericEvent[T]{Object: e.Object.(T)}
			h.Generic(ctx, ev, wq)
		},
	}
}

func newEventHandlerForObject[T client.ObjectConstraint](h handler.EventHandler[client.Object]) handler.EventHandler[T] {
	return handler.Funcs[T]{
		CreateFunc: func(ctx context.Context, e event.CreateEvent[T], wq workqueue.RateLimitingInterface) {
			ev := event.CreateEvent[client.Object]{Object: e.Object}
			h.Create(ctx, ev, wq)
		},
		UpdateFunc: func(ctx context.Context, e event.UpdateEvent[T], wq workqueue.RateLimitingInterface) {
			ev := event.UpdateEvent[client.Object]{ObjectNew: e.ObjectNew, ObjectOld: e.ObjectOld}
			h.Update(ctx, ev, wq)
		},
		DeleteFunc: func(ctx context.Context, e event.DeleteEvent[T], wq workqueue.RateLimitingInterface) {
			ev := event.DeleteEvent[client.Object]{Object: e.Object, DeleteStateUnknown: e.DeleteStateUnknown}
			h.Delete(ctx, ev, wq)
		},
		GenericFunc: func(ctx context.Context, e event.GenericEvent[T], wq workqueue.RateLimitingInterface) {
			ev := event.GenericEvent[client.Object]{Object: e.Object}
			h.Generic(ctx, ev, wq)
		},
	}
}

// type objSource[T client.ObjectConstraint] struct {
// 	src source.Source[T]
// }
//
// func (s *objSource[T]) Start(ctx context.Context, h handler.EventHandler[T], eq workqueue.RateLimitingInterface, ps ...predicate.Predicate[T]) {
// 	h := newObjectEventHandler(h)
// 	ps := objectPredicates(ps)
// 	s.src.Start()
// }
//
// func newObjectSource[T client.ObjectConstraint](src source.Source[T]) source.Source[client.Object] {
// }

func objectPredicates[T client.ObjectConstraint](ps []predicate.Predicate[T]) []predicate.Predicate[client.Object] {
	result := make([]predicate.Predicate[client.Object], 0, len(ps))
	for _, p := range ps {
		result = append(result, newObjectPredicate(p))
	}
	return result
}

func predicateObjects[T client.ObjectConstraint](ps []predicate.Predicate[client.Object]) []predicate.Predicate[T] {
	result := make([]predicate.Predicate[T], 0, len(ps))
	for _, p := range ps {
		result = append(result, newPredicateFromObject[T](p))
	}
	return result
}

type watcher interface {
	watch(blder *Builder) error
}

type forInput interface {
	object() client.Object
	watcher
}

type Option func(*Builder)

// ControllerManagedBy returns a new controller builder that will be started by the provided Manager.
func ControllerManagedBy(m manager.Manager, r reconcile.Reconciler, opts ...Option) (controller.Controller, error) {
	blder := &Builder{mgr: m}
	for _, opt := range opts {
		opt(blder)
	}
	if r == nil {
		return nil, fmt.Errorf("must provide a non-nil Reconciler")
	}
	if blder.mgr == nil {
		return nil, fmt.Errorf("must provide a non-nil Manager")
	}
	if blder.forInputErr != nil {
		return nil, blder.forInputErr
	}

	// Set the ControllerManagedBy
	if err := blder.doController(r); err != nil {
		return nil, err
	}

	// Set the Watch
	if err := blder.doWatch(); err != nil {
		return nil, err
	}

	return blder.ctrl, nil
}

// ForInput represents the information set by For method.
type ForInput[T client.ObjectConstraint] struct {
	obj              T
	predicates       []predicate.Predicate[T]
	objectProjection objectProjection
}

func (f *ForInput[T]) watch(blder *Builder) error {
	obj, err := blder.project(f.obj, f.objectProjection)
	if err != nil {
		return err
	}
	src := source.Kind(blder.mgr.GetCache(), obj)
	hdler := &handler.EnqueueRequestForObject[client.Object]{}
	allPredicates := blder.globalPredicates
	for _, p := range f.predicates {
		allPredicates = append(allPredicates, newObjectPredicate(p))
	}
	return blder.ctrl.Watch(src, hdler, allPredicates...)
}

func (f *ForInput[T]) object() client.Object {
	return f.obj
}

// For defines the type of Object being *reconciled*, and configures the ControllerManagedBy to respond to create / delete /
// update events by *reconciling the object*.
// This is the equivalent of calling
// Watches(&source.Kind{Type: apiType}, &handler.EnqueueRequestForObject{}).
func For[T client.ObjectConstraint](object T, opts ...ForOption[T]) Option {
	return func(blder *Builder) {
		if blder.forInput != nil {
			blder.forInputErr = fmt.Errorf("For(...) should only be called once, could not assign multiple objects for reconciliation")
			return
		}
		input := ForInput[T]{obj: object}
		for _, opt := range opts {
			opt.ApplyToFor(&input)
		}

		blder.forInput = &input
	}
}

// OwnsInput represents the information set by Owns method.
type OwnsInput[T client.ObjectConstraint] struct {
	matchEveryOwner  bool
	obj              T
	predicates       []predicate.Predicate[T]
	objectProjection objectProjection
}

func (f *OwnsInput[T]) watch(blder *Builder) error {
	obj, err := blder.project(f.obj, f.objectProjection)
	if err != nil {
		return err
	}
	src := source.Kind(blder.mgr.GetCache(), obj)
	opts := []handler.OwnerOption[T]{}
	if !f.matchEveryOwner {
		opts = append(opts, handler.OnlyControllerOwner[T]())
	}
	hdler := handler.EnqueueRequestForOwner(
		blder.mgr.GetScheme(),
		blder.mgr.GetRESTMapper(),
		f.obj,
		opts...,
	)
	allPredicates := append([]predicate.Predicate[client.Object](nil), blder.globalPredicates...)
	allPredicates = append(allPredicates, objectPredicates(f.predicates)...)
	if err := blder.ctrl.Watch(src, hdler, allPredicates...); err != nil {
		return err
	}
	return nil
}

// Owns defines types of Objects being *generated* by the ControllerManagedBy, and configures the ControllerManagedBy to respond to
// create / delete / update events by *reconciling the owner object*.
//
// The default behavior reconciles only the first controller-type OwnerReference of the given type.
// Use Owns(object, builder.MatchEveryOwner) to reconcile all owners.
//
// By default, this is the equivalent of calling
// Watches(object, handler.EnqueueRequestForOwner([...], ownerType, OnlyControllerOwner())).
func Owns[T client.ObjectConstraint](object T, opts ...OwnsOption[T]) Option {
	return func(blder *Builder) {
		input := OwnsInput[T]{obj: object}
		for _, opt := range opts {
			opt.ApplyToOwns(&input)
		}

		blder.ownsInput = append(blder.ownsInput, &input)
	}
}

// WatchesInput represents the information set by Watches method.
type WatchesInput[T client.ObjectConstraint] struct {
	src              source.Source[T]
	eventhandler     handler.EventHandler[T]
	predicates       []predicate.Predicate[T]
	objectProjection objectProjection
}

type objectSource[T client.ObjectConstraint] struct {
	f func(context.Context, handler.EventHandler[T], workqueue.RateLimitingInterface, ...predicate.Predicate[T]) error
}

func (s *objectSource[T]) Start(ctx context.Context, hdler handler.EventHandler[T], wq workqueue.RateLimitingInterface, ps ...predicate.Predicate[T]) error {
	return s.f(ctx, hdler, wq, ps...)
}

func newObjectSource[T client.ObjectConstraint](src source.Source[T]) source.Source[client.Object] {
	return &objectSource[client.Object]{
		f: func(ctx context.Context, h handler.EventHandler[client.Object], wq workqueue.RateLimitingInterface, ps ...predicate.Predicate[client.Object]) error {
			predicates := predicateObjects[T](ps)
			handler := newEventHandlerForObject[T](h)
			return src.Start(ctx, handler, wq, predicates...)
		},
	}
}

func (w *WatchesInput[T]) watch(blder *Builder) error {
	allPredicates := append([]predicate.Predicate[client.Object](nil), blder.globalPredicates...)
	allPredicates = append(allPredicates, objectPredicates(w.predicates)...)

	// if err := blder.ctrl.Watch(w.src, newObjectEventHandler(w.eventhandler), allPredicates...); err != nil {
	// 	return err
	// }
	return nil
}

// Watches defines the type of Object to watch, and configures the ControllerManagedBy to respond to create / delete /
// update events by *reconciling the object* with the given EventHandler.
//
// This is the equivalent of calling
// WatchesRawSource(source.Kind(scheme, object), eventhandler, opts...).
func Watches[T client.ObjectConstraint](object T, eventhandler handler.EventHandler[T], opts ...WatchesOption[T]) Option {
	return func(blder *Builder) {
		src := source.Kind(blder.mgr.GetCache(), object)
		input := WatchesInput[T]{src: src, eventhandler: eventhandler}
		for _, opt := range opts {
			opt.ApplyToWatches(&input)
		}

		if input.objectProjection == projectAsMetadata {
			partial, err := blder.project(object, projectAsMetadata)
			if err != nil {
				panic("TODO")
			}
			src := source.Kind(blder.mgr.GetCache(), partial)
			input := WatchesInput[client.Object]{
				src:              src,
				eventhandler:     newObjectEventHandler(eventhandler),
				predicates:       objectPredicates(input.predicates),
				objectProjection: projectAsMetadata,
			}
			blder.watchesInput = append(blder.watchesInput, &input)
		}

		blder.watchesInput = append(blder.watchesInput, &input)
	}
}

// WatchesMetadata is the same as Watches, but forces the internal cache to only watch PartialObjectMetadata.
//
// This is useful when watching lots of objects, really big objects, or objects for which you only know
// the GVK, but not the structure.  You'll need to pass metav1.PartialObjectMetadata to the client
// when fetching objects in your reconciler, otherwise you'll end up with a duplicate structured or unstructured cache.
//
// When watching a resource with metadata only, for example the v1.Pod, you should not Get and List using the v1.Pod type.
// Instead, you should use the special metav1.PartialObjectMetadata type.
//
// ❌ Incorrect:
//
//	pod := &v1.Pod{}
//	mgr.GetClient().Get(ctx, nsAndName, pod)
//
// ✅ Correct:
//
//	pod := &metav1.PartialObjectMetadata{}
//	pod.SetGroupVersionKind(schema.GroupVersionKind{
//	    Group:   "",
//	    Version: "v1",
//	    Kind:    "Pod",
//	})
//	mgr.GetClient().Get(ctx, nsAndName, pod)
//
// In the first case, controller-runtime will create another cache for the
// concrete type on top of the metadata cache; this increases memory
// consumption and leads to race conditions as caches are not in sync.
func WatchesMetadata[T client.ObjectConstraint](object T, eventhandler handler.EventHandler[*metav1.PartialObjectMetadata], opts ...WatchesOption[*metav1.PartialObjectMetadata]) Option {
	return func(blder *Builder) {
		object := &metav1.PartialObjectMetadata{}
		object.SetGroupVersionKind(object.GroupVersionKind())
		opts = append(opts, OnlyMetadata[*metav1.PartialObjectMetadata]())
		Watches(object, eventhandler, opts...)
	}
}

// WatchesRawSource exposes the lower-level ControllerManagedBy Watches functions through the builder.
// Specified predicates are registered only for given source.
//
// STOP! Consider using For(...), Owns(...), Watches(...), WatchesMetadata(...) instead.
// This method is only exposed for more advanced use cases, most users should use higher level functions.
func WatchesRawSource[T client.ObjectConstraint](src source.Source[T], eventhandler handler.EventHandler[T], opts ...WatchesOption[T]) Option {
	return func(blder *Builder) {
		input := WatchesInput[T]{src: src, eventhandler: eventhandler}
		for _, opt := range opts {
			opt.ApplyToWatches(&input)
		}

		blder.watchesInput = append(blder.watchesInput, &input)
	}
}

// WithEventFilter sets the event filters, to filter which create/update/delete/generic events eventually
// trigger reconciliations.  For example, filtering on whether the resource version has changed.
// Given predicate is added for all watched objects.
// Defaults to the empty list.
func WithEventFilter[T client.ObjectConstraint](p predicate.Predicate[T]) Option {
	return func(blder *Builder) {
		blder.globalPredicates = append(blder.globalPredicates, newObjectPredicate(p))
	}
}

// WithOptions overrides the controller options use in doController. Defaults to empty.
func WithOptions(options controller.Options) Option {
	return func(blder *Builder) {
		blder.ctrlOptions = options
	}
}

// WithLogConstructor overrides the controller options's LogConstructor.
func WithLogConstructor(logConstructor func(*reconcile.Request) logr.Logger) Option {
	return func(blder *Builder) {
		blder.ctrlOptions.LogConstructor = logConstructor
	}
}

// Named sets the name of the controller to the given name.  The name shows up
// in metrics, among other things, and thus should be a prometheus compatible name
// (underscores and alphanumeric characters only).
//
// By default, controllers are named using the lowercase version of their kind.
func Named(name string) Option {
	return func(blder *Builder) {
		blder.name = name
	}
}

func (blder *Builder) project(obj client.Object, proj objectProjection) (client.Object, error) {
	switch proj {
	case projectAsNormal:
		return obj, nil
	case projectAsMetadata:
		return objectAsMetadata(obj, blder.mgr.GetScheme())
	default:
		panic(fmt.Sprintf("unexpected projection type %v on type %T, should not be possible since this is an internal field", proj, obj))
	}
}

func objectAsMetadata[T client.ObjectConstraint](obj T, scheme *runtime.Scheme) (*metav1.PartialObjectMetadata, error) {
	metaObj := &metav1.PartialObjectMetadata{}
	gvk, err := getGvk(obj, scheme)
	if err != nil {
		return nil, fmt.Errorf("unable to determine GVK of %T for a metadata-only watch: %w", obj, err)
	}
	metaObj.SetGroupVersionKind(gvk)
	return metaObj, nil
}

func (blder *Builder) doWatch() error {
	// Reconcile type
	if blder.forInput != nil {
		if err := blder.forInput.watch(blder); err != nil {
			return err
		}
	}

	// Watches the managed types
	if len(blder.ownsInput) > 0 && blder.forInput == nil {
		return errors.New("Owns() can only be used together with For()")
	}
	for _, own := range blder.ownsInput {
		if err := own.watch(blder); err != nil {
			return err
		}
	}

	// Do the watch requests
	if len(blder.watchesInput) == 0 && blder.forInput == nil {
		return errors.New("there are no watches configured, controller will never get triggered. Use For(), Owns() or Watches() to set them up")
	}
	for _, w := range blder.watchesInput {
		if err := w.watch(blder); err != nil {
			return err
		}
	}
	return nil
}

func (blder *Builder) getControllerName(gvk schema.GroupVersionKind, hasGVK bool) (string, error) {
	if blder.name != "" {
		return blder.name, nil
	}
	if !hasGVK {
		return "", errors.New("one of For() or Named() must be called")
	}
	return strings.ToLower(gvk.Kind), nil
}

func (blder *Builder) doController(r reconcile.Reconciler) error {
	globalOpts := blder.mgr.GetControllerOptions()

	ctrlOptions := blder.ctrlOptions
	if ctrlOptions.Reconciler == nil {
		ctrlOptions.Reconciler = r
	}

	// Retrieve the GVK from the object we're reconciling
	// to prepopulate logger information, and to optionally generate a default name.
	var gvk schema.GroupVersionKind
	hasGVK := blder.forInput != nil
	if hasGVK {
		var err error
		gvk, err = getGvk(blder.forInput.object(), blder.mgr.GetScheme())
		if err != nil {
			return err
		}
	}

	// Setup concurrency.
	if ctrlOptions.MaxConcurrentReconciles == 0 && hasGVK {
		groupKind := gvk.GroupKind().String()

		if concurrency, ok := globalOpts.GroupKindConcurrency[groupKind]; ok && concurrency > 0 {
			ctrlOptions.MaxConcurrentReconciles = concurrency
		}
	}

	// Setup cache sync timeout.
	if ctrlOptions.CacheSyncTimeout == 0 && globalOpts.CacheSyncTimeout > 0 {
		ctrlOptions.CacheSyncTimeout = globalOpts.CacheSyncTimeout
	}

	controllerName, err := blder.getControllerName(gvk, hasGVK)
	if err != nil {
		return err
	}

	// Setup the logger.
	if ctrlOptions.LogConstructor == nil {
		log := blder.mgr.GetLogger().WithValues(
			"controller", controllerName,
		)
		if hasGVK {
			log = log.WithValues(
				"controllerGroup", gvk.Group,
				"controllerKind", gvk.Kind,
			)
		}

		ctrlOptions.LogConstructor = func(req *reconcile.Request) logr.Logger {
			log := log
			if req != nil {
				if hasGVK {
					log = log.WithValues(gvk.Kind, klog.KRef(req.Namespace, req.Name))
				}
				log = log.WithValues(
					"namespace", req.Namespace, "name", req.Name,
				)
			}
			return log
		}
	}

	// Build the controller and return.
	blder.ctrl, err = newController(controllerName, blder.mgr, ctrlOptions)
	return err
}
