Controller Lifecycle Management
==========================

## Summary

Enable fine-grained control over the lifecycle of a controller, including the
ability to start/stop/restart controllers and their caches by exposing a way to
detect when the resource a controller is watching is uninstalled/reinstalled on
the cluster.

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
their caches based on whether or not a CRD is installed in the cluster.

## Non-Goals

* controller-runtime does not need to support starting/stopping/restarting controllers and
their caches on arbitrary conditions.

* No further guarantees of multi-cluster support beyond what is already provided
  by controller-runtime.

## Proposal

The following proposal offers a solution for controller/cache restarting by:

1. Implementing a stop channel on the informers map `MapEntry` that can be
   retrieved from the cache's `Informers` interface that fires when the informer has
   stopped.
2. A new Source is created called `ConditionalSource` that is implemented by a
   `ConditionalKind` that has a `Ready()` method that blocks until the kind's
   type is installed on the cluster and ready to be controlled, and a
   `StartNotifyDone()` that starts the underlying source with a channel that
   fires when the source (and its underlying informer have stopped).
3. Controllers maintain a list of `ConditonalSource`s known as
   `conditionalWatches` that can be added like regular watches via the
   controllers `Watch` method. For any `ConditionalSource` that `Watch()` is
   called with, it uses `Ready()` to determine when to `StartNotifyDone()` it
   and uses the done channel returned by `StartNotifyDone()` to determine when
   the watch has started and that it should wait on `Ready()` again.
4. The manager recognize a special type or `Runnable` known as a
   `ConditionalRunnable` that has a `Ready()` method to indicate when the
   underlying `Runnable` can be ran.
5. The controller builder enables users supply a boolean `conditional` option to
   the builder's `For`, `Owns`, and `Watches` inputs that creates
   `ConditionalRunnables` to be ran by the manager.

### Informer Stop Channel

The cache's `Informers` interface adds a new method that gives a stop channel
that fires when an informer is stopped:

```
type Informers interface {
	// GetInformerStop fetches the stop channel for the informer for the given object (constructing
	// the informer if necessary). This stop channel fires when the controller has stopped.
	GetInformerStop(ctx context.Context, obj client.Object) (<-chan struct{}, error)

        // ... other existing methods
}
```

This is implemented by adding a stop channel to the informer cache's `MapEntry`
which gets populated when the informer is constructed and started in
`specificInformersMap#addInformerToMap`:

```
type MapEntry struct {
	// StopCh is a channel that is closed after
	// the informer stops
	StopCh <-chan struct{}

        // ... other existing methods
}
```

### ConditionalSource

A special type of Source called a `ConditionalSource` provides mechanisms for
determing when the Source can be started (`Ready()`) and when it is done
(`StartNotifyDone()`)

```
// ConditionalSource is a Source of events that additionally offer the ability to see when the Source is ready to be
// started as well as offering a wrapper around the Source's Start method that returns a channel the fires when an
// already started Source has stopped.
type ConditionalSource interface {
	Source

	// StartNotifyDone runs the underlying Source's Start method and provides a channel that fires when
	// the Source has stopped.
	StartNotifyDone(context.Context, handler.EventHandler, workqueue.RateLimitingInterface, ...predicate.Predicate) (<-chan struct{}, error)

	// Ready blocks until it is safe to call StartNotifyDone, meaning the Source's Kind's type has been
	// successfully installed on the cluster  and is ready to have its events watched and handled.
	Ready(ctx context.Context, wg *sync.WaitGroup)
}
```

### ConditionalWatches (Controller)

Controllers maintain a list of `ConditonalSource`s known as `conditionalWatches`.
```

type Controller struct {
	// startWatches maintains a list of sources, handlers, and predicates to start when the controller is started.
	startWatches       []watchDescription

	// conditionalWatches maintains a list of conditional sources that provide information on when the source is ready to be started/restarted and when the source has stopped.
	conditionalWatches []conditionalWatchDescription

        // ... existing fields
}
```

This is list is populated just like the existing startWatches method by passing
a `ConditionalSource` to the controller's `Watch()` method

```
func (c *Controller) Watch(src source.Source, evthdler handler.EventHandler, prct ...predicate.Predicate) error {
        // ... existing code

	if conditionalSource, ok := src.(source.ConditionalSource); ok && !c.Started {
		c.conditionalWatches = append(c.conditionalWatches, conditionalWatchDescription{src: conditionalSource, handler: evthdler, predicates: prct})
		return nil
	}

        // ... existing code
```

Controllers now expose a `Ready` method that is called by the controller manager
to determine when the controller can be started.
```
func (c *Controller) Ready(ctx context.Context) <-chan struct{} {
	ready := make(chan struct{})
	if len(c.conditionalWatches) == 0 {
		close(ready)
		return ready
	}

	var wg sync.WaitGroup
	for _, w := range c.conditionalWatches {
		wg.Add(1)
		go w.src.Ready(ctx, &wg)
	}

	go func() {
		defer close(ready)
		wg.Wait()
	}()
	return ready
}
```

The controller's `Start()` method now runs the conditionalWatches with
`StartNotifyDone` and cancels the controller with the watch stops.

### ConditionalRunnable (Manager)

The manager recognizes a special type or `Runnable` known as a
`ConditionalRunnable` that has a `Ready()` method to indicate when the
underlying `Runnable` can be ran.

```
// ConditionalRunnable wraps Runnable with an additonal Ready method
// that returns a channel that fires when the underlying Runnable can
// be started.
type ConditionalRunnable interface {
	Runnable
	Ready(ctx context.Context) <-chan struct{}
}
```

Now when the controller starts all the conditional runnables, it runs in a loop
starting the runnable only when it is ready, looping and waiting for ready again
whenever it is stopped.

```
// startConditionalRunnable fires off a goroutine that
// blocks on the runnable's Ready (or the shutdown context).
//
// Once ready, call a version of start runnable that blocks
// until the runnable is terminated.
//
// Once the runnable stops, loop back and wait for ready again.
func (cm *controllerManager) startConditionalRunnable(cr ConditionalRunnable) {
	go func() {
		for {
			select {
			case <-cm.internalCtx.Done():
				return
			case <-cr.Ready(cm.internalCtx):
				cm.waitForRunnable.Add(1)
				defer cm.waitForRunnable.Done()
				if err := cr.Start(cm.internalCtx); err != nil {
					cm.errChan <- err
				}
				return
			}
		}
	}()
}
```

### Conditional Builder Option

The controller builder enables users supply a boolean `conditional` option to
the builder's `For`, `Owns`, and `Watches` inputs that creates
`ConditionalRunnables` to be ran by the manager.

```
func (blder *Builder) doWatch() error {
	if blder.forInput.conditional {
		gvk, err := getGvk(blder.forInput.object, blder.mgr.GetScheme())
		if err != nil {
			return err
		}
		existsInDiscovery := // function to determine whether the gvk
                exists in discovery
		src = &source.ConditionalKind{Kind: source.Kind{Type: typeForSrc}, DiscoveryCheck: existsInDiscovery}
	} else {
		src = &source.Kind{Type: typeForSrc}
	}
```

## Alternatives

### Cache of Caches

One alternative is to create a separate cache per watch (i.e. cache of caches)
as the end user consuming controller-runtime. The advantage is that it prevents
us from having to modify any code in controller-runtime. The main drawback is
that it's very clunky to maintain multiple caches (one for each informer) and
breaks the clean design of the cache.

### Stoppable Informers and EventHandler removal natively in client-go

Currently, there is no way to stop a client-go `SharedInformer`. Therefore, the
current proposal suggests that we use the shared informer's `SetErrorHandler()`
method to determine when a CRD has been uninstalled (by inspecting listAndWatch
errors).

A cleaner approach is to offload this work to client-go. Then we can run the
informer such that it knows to stop itself when the CRD has been uninstalled.
This has been proposed to the client-go maintainers and a WIP implementation
exists [here](https://github.com/kubernetes/kubernetes/pull/97214).

## Open Questions

### Discovery Client usage in DiscoveryCheck

The current proposal stores a user provided function on the `ConditionalKind`
called a `DiscoveryCheck` that determines whether a kind exists in discovery or
not.

This is a little clunky and calls out to the discovery client are slow.
Alternatively, the RESTMapper could theoretically be used to determine whether a
kind exists in discovery, but the current RESTMapper implementation doesn't
remove a type from the map when its resource is uninstalled from the cluster.

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
* 5/14/2021: Proposal updated to be fully self-contained in controller-runtime
  and modifies the proposed mechanism using Ready/StartNotifyDone mechanics.

