package handler

import (
	"context"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// WithPredicates returns an EventHandler that only calls the underlying handler if all the given predicates return true.
func WithPredicates(handler EventHandler, predicates ...predicate.Predicate) EventHandler {
	return &Funcs{
		CreateFunc: func(ctx context.Context, createEvent event.CreateEvent, queue workqueue.RateLimitingInterface) {
			for _, predicate := range predicates {
				if !predicate.Create(createEvent) {
					return
				}
			}
			handler.Create(ctx, createEvent, queue)
		},
		UpdateFunc: func(ctx context.Context, updateEvent event.UpdateEvent, queue workqueue.RateLimitingInterface) {
			for _, predicate := range predicates {
				if !predicate.Update(updateEvent) {
					return
				}
			}
			handler.Update(ctx, updateEvent, queue)
		},
		DeleteFunc: func(ctx context.Context, deleteEvent event.DeleteEvent, queue workqueue.RateLimitingInterface) {
			for _, predicate := range predicates {
				if !predicate.Delete(deleteEvent) {
					return
				}
			}
			handler.Delete(ctx, deleteEvent, queue)
		},
		GenericFunc: func(ctx context.Context, genericEvent event.GenericEvent, queue workqueue.RateLimitingInterface) {
			for _, predicate := range predicates {
				if !predicate.Generic(genericEvent) {
					return
				}
			}
			handler.Generic(ctx, genericEvent, queue)
		},
	}
}
