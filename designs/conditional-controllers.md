Conditionally Runnable Controllers
==========================

## Summary

Enable controller managers to successfully operate when the CRD the controller
is configured to watch does not exist.

Successful operation of a controller manager includes:
* Starts and runs without error when a CRD the controller watches does not exist.
* Begins watching the CRD once it is installed.
* Unregisters (stops watching) once a CRD is uninstalled.

## Background/Motivation

Usually there is a 1:1 relationship between controller and resource and a 1:1
relationship between controller and k8s cluster. When this is the case it is
fine to assume that for a controller to run successfully, the resource it
controls must be installed on the cluster that the controller is watching.

We are now seeing use cases where a single controller is responsible for
multiple resources across multiple clusters. This creates situations where
controllers need to successfully run even when the resource is not installed,
and need to proceed successfully as it begins/terminates its watch on a resource
when the resource is installed/uninstalled.

The current approach is to check the discovery for a CRDs existence prior to
adding the controller to the manager. This has its limitations, as complexity
increases greatly for users who need to manage multiple controllers for a
mixture of CRDs that might not always be installed on the cluster or are
installed in different order.

While there is a lot of groundwork needed to fully support multi-cluster
controllers, this proposal offers incremental steps to supporting one specific use case
(conditionally runnable controllers), of which hopefully some minimal form can be agreed
upon before needing a comple multi-cluster story.

## Goals

(In order from most necessary to least)
1. A mechanism for starting/stopping/restarting controllers and their caches.
2. A solution for running a controller conditionally, such that it automatically
starts/stops/restarts upon installation/uninstallation/reinstallation of its
respective CRD in the cluster.
3. An easy-to-use mechanism for solving goal #2 without the end user needing to
understand too much.

## Non-Goals

TODO

## Proposal

The following proposal attempts to address the three goals in four separate
steps.

The first goal of enabling the possibility of starting/stopping/restarting
controllers is addressed in two parts, first where a mechanism is built to
enable the removal of informers, and second where we expose this mechanism and
enable the restarting of controllers

The second goal of a solution for running controllers conditionally is addressed
by creating a wrapper around a controller called a ConditionalController that
within its Start() function, it polls the discovery doc for the existence of the
watched object and starts/stops/restarts the underlying controller as necessary 

The third goal of an ergonomic mechanism to use ConditionalControllers is a
small modification to the controller builder to enable running a controller as a
ConditionalController.

###### Proof of concept
A proof of concept PR exists at
[#1180](https://github.com/kubernetes-sigs/controller-runtime/pull/1180)

Each commit maps to one step in the proposal and can loosely be considered going
from most necessary to least.

### Informer Removal [Pre-req]

This proposal assumes a mechanism exists for removing individual informers. We
are agnostic to how this is done, but the current proposal is built on top of
Shomron’s proposed implementation of individual informer removal
[#936](https://github.com/kubernetes-sigs/controller-runtime/pull/936).


Risks/mitigations and alternatives are discussed in the linked PR as well as the
corresponding issue
[#935](https://github.com/kubernetes-sigs/controller-runtime/issues/935). 

We are currently looking into whether that PR can be distilled further into a more
minimal solution.

### Minimal hooks

Given that a mechanism exists to remove individual informers, the next step is
to expose this removal functionality and enable safely
starting/stopping/restarting controllers and their caches.

The proposal to do this is:
1. Expose the `informerCache.Remove()` functionality on the cache interface.
2. Expose the ability to reset the internal controller’s `Started` field.
3. Expose a field on the internal controller to determine whether to `saveWatches`
or not and use this field to not empty the controller’s `startWatches` when the
controller is stopped.

#### Risks and Mitigations

* [#1139](https://github.com/kubernetes-sigs/controller-runtime/pull/1139) discusses why
the ability to start a controller more than once was taken away. It's a little
unclear what effect explicitly enabling multiple starts in the case of
conditional controllers will hae on the number of workers and workqueues
relative to expectations and metrics.
* [#1163](https://github.com/kubernetes-sigs/controller-runtime/pull/1163) discusses the
memory leak caused by no clearing out the watches internal slice. A possible
mitigation is to clear out the slices upon ConditionalController shutdown.

#### Alternatives

* Instead of exposing ResetStart and SaveWatches on the internal controller struct
it might be better to expose it on the controller interface. This is more public
and creates more potential for abuse, but it prevents some hacky solutions
discussed below around needing to cast to the internal controller or create
extra interfaces.

### Conditional Controllers

With the minimal hooks needed to start/stop/restart controllers and their caches
in place, the next step is to provide a wrapper controller around a traditional
controller that starts/stops the underlying controller based on the existence of
the CRD under watch.

The proposal to do this:
1. `ConditionalController` that takes implements controller and with in Start()
method:
2. Polls the discovery doc every configurable amount of time and recognizes when
the CRD has been installed/uninstalled.
3. Upon installation it merges the caller’s stop channel with a local start channel
and runs the underlying controller with the merged stop channel such that both
the caller and this conditional controller can stop it.
4. Upon uninstallation it sets the controllers `Started` field to false so that it
can be restarted, indicates that the controller should save its watches upon
stopping, and then stops the controller via the local stop channel and removes
the cache for the object under watch.

#### Risks and Mitigations

* With the above minimal hooks exposing `ResetStart()` and `SaveWatches()` on the
internal controller this creates the need to create a hacky intermediate
interface (StoppableController) for testing and to avoid casting to the internal
controller. The simplest solution is just to expose these on the controller
interface (see above alternative), but there might be a better way.

#### Alternatives

* A more general solution could allow for conditional runnables of more than just
controllers, where the user could supply the conditional check on their own,
such that it’s not just limited to looking at the existence of a CRD. (but we
believe that the common case will be a controller watching CRD install and thus
there should still be a simple way to do this)
* Nit: it's unclear if the controller package is the best place for this to live.
Originally it was thought that controllerutil might be best, but that creates an
import cycle.

### Builder Ergonomics

Since we provide simple ergonomics for creating controllers (builder), it would
make sense to do the same for any conditional controller utility we create.

The current proposal (link) is to:
1. Provide a forInput boolean option that the user can set to enable conditionally
running and an option to configure the wait time that the ConditionalController
will wait between attempts to poll the discovery doc.
2. In the builder’s doController function, it will first create an unmanaged
controller.
3. If the conditionally run option is set, it will wrap the unmanaged controller in
a conditional controller.
4. Add the resulting controller to the manager.

#### Risks and Mitigations

TODO

#### Alternatives
* A separate builder specifically for the conditional controller would prevent us
from having to make changes to the current builder.
* As noted above a more general conditional runnable will probably require its own
builder.

## Acceptance Criteria

Ultimately, the end user must have some successful solution that involves
starting with a controller for a CRD that can
1. Run the mgr without CRD installed yet and mgr should start successfully
2. After CRD installation the controller should start and run successfully
3. Upon uninstalling the CRD, the controller should stop but the manager should
continue running successfully
4. Upon CRD reinstallation, the controller should once again start and run
successfully with no disruption to the manager and its other runnables.

## Timeline of Events
* 9/30/2020: Propose idea in design doc and proof-of-concept PR to
controller-runtime
* 10/8/2020: Discuss idea in community meeting

