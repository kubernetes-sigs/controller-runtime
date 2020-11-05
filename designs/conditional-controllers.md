Controller Lifecycle Management
==========================

## Summary

Enable fine-grained control over the lifecycle of a controller, including the
ability to start/stop/restart controllers and their caches by exposing a way to
remove individual informers from the cache and working around restrictions that
currently prevent controllers from starting multiple times.

## Background/Motivation

Currently, the user does not have much controller over the lifecycle of a
controller. The user can add controllers to the manager and add informers to the
cache, but there is no way to remove either of these.

Additionally there are restrictions that prevent a controller from running
multiple times such as the clearing the watches slice for a controller after it
has started. The effect of this is that users have no clean way of restarting controllers
after they have stopped them.

Greater detail is given in the "Future Work / Use Cases" section, but a summary
of the motivating use-cases for the proposal is anytime a controller's watched resource
can be installed/uninstalled unexpectedly or whenever the administrator of a
controller is different from the administrator of the cluster (and thus has no
control over which resources are installed).

## Goals

controller-runtime should support starting/stopping/restarting controllers and
their caches on arbitrary conditions.

## Non-Goals

* An implementation of starting/stopping a controller based on a specific
condition (e.g. the existence of a CRD in discovery) does not need to be
implemented in controller-runtime (but the end consumer of controller-runtime
should be able to build it on their own).

* No further guarantees of multi-cluster support beyond what is already provided
  by controller-runtime.

## Proposal

The following proposal offers a solution for controller/cache restarting by:
1. Enabling the removal of individual informers.
2. Tracking the number of event handlers on an Informer.

This proposal focuses on solutions that are entirely contained in
controller-runtime. In the alternatives section, we discuss potential ways that
changes can be added to api-machinery code in core kubernetes to enable a
possibly cleaner SharedInformer interface for accomplishing our goals.

A proof-of-concept exists at
[#1180](https://github.com/kubernetes-sigs/controller-runtime/pull/1180).

### Informer Removal

The starting point for this proposal is Shomron’s proposed implementation of
individual informer removal.
[#936](https://github.com/kubernetes-sigs/controller-runtime/pull/936).

It extends the informer cache to expose a remove method:
```
// Remove removes an informer specified by the obj argument from the cache and
// stops it if it existed.
Remove(ctx context.Context, obj runtime.Object) error
```

This gets plumbed through to the informers map, where we remove an informer
entry from the informersByGVK

```
// Remove removes an informer entry and stops it if it was running.
func (ip *specificInformersMap) Remove(gvk schema.GroupVersionKind) {
  ...
  entry, ok := ip.informersByGVK[gvk]
  close(entry.stop)
  delete(ip.informersByGVK, gvk)
}
```

We support this by modifying the Start method to run the informer with 
a stop channel that can be stopped individually
or globally (as opposed to just globally).

```
// Start calls Run on each of the informers...
func (ip *specificInformersMap) Start(ctx context) {
...
for _, entry := range ip.informersByGVK{
  go entry.Start(ctx.Done())
}
...
}

// Start starts the informer managed by a MapEntry.
// Blocks until the informer stops. The informer can be stopped
// either individually (via the entry's stop channel) or globally
// via the provided stop argument.
func (e *MapEntry) Start(stop <-chan struct{}) {
  // Stop on either the whole map stopping or just this informer being removed.
  internalStop, cancel := anoyOf(stop, e.stop)
  defer cancel()
  e.Informer.Run(internalStop)
}
```

### EventHandler reference counting

The primary concern with adding individual informer removal is that it exposes
the risk of silently degrading controllers that are using the informer being
removed.

To mitigate this we need a way to track the number of EventHandlers added to a
given informer and ensure that the are none remaining when an informer is
removed.

The proposal here is to expand the informer interface used by the `MapEntry` in
the informers map:
```
// CountingInformer exposes a way to track the number of EventHandlers
//registered on an informer.

// The implementation of this should store a count that gets incremented every
// time AddEventHandler is called and decremented every time RemoveEventHandler is
// called.
type CountingInformer interface {
  cache.SharedIndexInformer
  CountEventHandlers() int
  RemoveEventHandler()
}
```

Now, we need a way to run a controller, such that the starting the controller
increments the EventHandler counts on the informer and stopping the controller
decrements it.

We can modify/expand the `Source` interface, such that we can call
`Start()` with a context that blocks on ctx.Done() and calls RemoveEventHandler
on the informer once the context is closed, because the controller has been
stopped so the EventHandler no longer exists on the informer.

```
// StoppableSource expands the Source interface to add a start function that
// blocks on the context's Done channel, so that we know when the controller has
// been stopped and can remove/decrement the EventHandler count on the informer
// appropriately
type StoppableSource interface {
  Source
  StartStoppable(context.Context, handler.EventHandler,
  workqueue.RateLimitingInterface, ...predicate.Predicate) error
}

// StartStoppable blocks for start to finish and then calls RemoveEventHandler
// on the kind's informer.
func (ks *Kind) StartStoppable(ctx, handler, queue, prct) {
  informer := ks.cache.GetInformer(ctx, ks.Type)
  ks.Start(ctx, handler, queue, prct)
  <-ctx.Done()
  i.RemoveEventHandler()
}

```

A simpler alternative is to all of this is to bake the `EventHandler` tracking into the underlying
client-go `SharedIndexInformer`. This involves modifying client-go and is
discussed below in the alternatives section.

## Alternatives

A couple alternatives to what we propose, one that offloads all the
responsibility to the consumer requiring no changes to controller-runtime and
one that moves the event handler reference counting upstream into client-go.

### Cache of Caches

One alternative is to create a separate cache per watch (i.e. cache of caches)
as the end user consuming controller-runtime. The advantage is that it prevents
us from having to modify any code in controller-runtime. The main drawback is
that it's very clunky to maintain multiple caches (one for each informer) and
breaks the clean design of the cache.

### Stoppable Informers and EventHandler removal natively in client-go

*This proposal was discussed with sig-api-machinery on 11/5 and has been
tentatively accepted. See the [design
doc](https://docs.google.com/document/d/17QrTaxfIEUNOEAt61Za3Mu0M76x-iEkcmR51q0a5lis/edit) and [WIP implementation](https://github.com/kevindelgado/kubernetes/pull/1) for more detail.*

A few changes to the underlying SharedInformer interface in client-go could save
us from a lot of work in controller-runtime.

One is to add a second `Run` method on the `SharedInformer` interface such as
```
// RunWithStopOptions runs the informer and provides options to be checked that
// would indicate under what conditions the informer should stop
RunWithStopOptions(stopOptions StopOptions)

// StopOptions let the caller specifcy which conditions to stop the informer.
type StopOptions struct {
  // ExternalStop stops the Informer when it receives a close signal
  ExternalStop <-chan struct{}

  // OnListError inspects errors returned from the informer's underlying
  // reflector, and based on the error determines whether or not to stop the
  // informer.
  OnListError func(err) bool
}
```

This could be plumbed through controller-runtime allowing users to pass informer
`StopOptions` when they create a controller.

Another potential change to `SharedInformer` is to have it be responsible for
tracking the count of event handlers and allowing consumers to remove
EventHandlers from a `SharedInformer.

similar to the `EventHandler` reference counting interface we discussed above the
`SharedInformer` could add methods:

```
type SharedInformer interface {
...
  RemoveEventHandler(handler ResourceEventHandler) bool
  EventHandlerCount() int
}
```

This would remove the need to track it ourselves in controller-runtime.

## Open Questions

### Multiple Restarts

The ability to start a controller more than once was recently removed over
concerns around data races from workqueue recreation and starting too many
workers ([#1139](https://github.com/kubernetes-sigs/controller-runtime/pull/1139))

Previously there was never a valid reason to start a controller more than once,
now we want to support that. The simplest workaround is to expose a
`ResetStart()` method on the controller interface that simply resets the
'Started' on the controller. This allows restarts but doesn't address the
initial data race/excess worker concerns that drove the removal of this
functionality in the first place.

### Persisting controller watches

The list of start watches store don the controller currently gets cleared out
once the controller is started in order to prevent memory leak([#1163](https://github.com/kubernetes-sigs/controller-runtime/pull/1163))

We need to retain references to these watches in order to restart controllers.
The simplest workaround is maintain a boolean `SaveWatches` field on the
controller that determines whether or not the watches slice should me retained
after starting a controller. We can then expose a method on the controller for
setting this field and enabling watches to be saved. We will still have a memory
leak, but only on controllers that are expected to be restart. There might be a
better way clear out the watches list that fits with the restartable controllers
paradigm.

### Multi-Cluster Support

A multi-cluster environment where a single controller operates across multiple
clusters will likely be a heavy user of these new features because the
multi-cluster frequently results in a situation where the
cluster administrator is different from the controller administrator.

We lack a consistent story around multi-cluster support and introducing changes
such as these without fully thinking through the mult-cluster story might bind
us for future designs.


## Future Work / Use Cases

Were this to move forward, it unlocks a number of potential use-cases.

1. We can support a "metacontroller" or controller of controllers that could
   start and stop controllers based on the existence of their corresponding
   CRDs.

2. OPA/Gatekeeper can simplify it's dynamic watch management by having greater
   controller over the lifecycle of a controller. See [this
   doc](https://docs.google.com/document/d/1Wi3LM3sG6Qgfzm--bWb6R0SEKCkQCCt-ene6cO62FlM/edit)

## Timeline of Events

* 9/30/2020: Propose idea in design doc and proof-of-concept PR to
controller-runtime
* 10/7/2020: Design doc and proof-of-concept distilled to focus only on minimal
  hooks needed rather than proposing entire conditional controller solution.
  Alternatives added to discuss opportunities to push some of the implementation
  to the api-machinery level.
* 10/8/2020: Discuss idea in community meeting
* 10/19/2020: Proposal updated to add EventHandler count tracking and client-go
  based alternative.
* 11/4/2020: Propopsal to add RunWithStopOptions, RemoveEventHandler, and
  EventHandlerCount discussed at sig-api-machinery meeting and was tentatively
  accepted. See the [design doc](https://docs.google.com/document/d/17QrTaxfIEUNOEAt61Za3Mu0M76x-iEkcmR51q0a5lis/edit) and [WIP implementation](https://github.com/kevindelgado/kubernetes/pull/1) for more detail.

