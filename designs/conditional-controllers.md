Informer Removal / Controller Lifecycle Management
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
has started.

The effect of this is that users have no clean way of restarting controllers
after they have stopped them. This would be useful for a number of use-cases
around when controllers have little control over the installation or
uninstallation on the of the resources that these controllers are responsible
for watching.

## Goals

An implementation of the minimally viable hooks needed in controller-runtime to
enable users to start, stop, and restart controllers and their caches.

## Non-Goals

A complete and ergonomic solution for automatically starting/stopping/restarting
controllers upon arbitrary conditions.

## Proposal

The following proposal offers a solution for controller/cache restarting by:
1. Enabling the removal of individual informers.
2. Publically exposing the informer removal and adding hooks into the internal
   controller implementation to allow for restarting controllers that have been
   stopped.

This proposal focuses on solutions that are entirely contained in
controller-runtime. In the alternatives section, we discuss potential ways that
changes can be added to api-machinery code in core kubernetes to enable a
possibly cleaner interface of accomplishing our goals.

### Informer Removal

The starting point for this proposal is Shomron’s proposed implementation of
individual informer removal.
[#936](https://github.com/kubernetes-sigs/controller-runtime/pull/936).

A discussion of risks/mitigations and alternatives are discussed in the linked PR as well as the
corresponding issue
[#935](https://github.com/kubernetes-sigs/controller-runtime/issues/935). A
summarization of these discussions are presented below.

#### Risks and Mitigations

* Controllers will silently degrade if the given informer for their watch is
  removed. Most likely this issue is mitigated by the fact that it's the
  controller responsible for removing the informer that will be the one impacted
  by the informer's removal and thus will be expected. If this is insufficient
  for all cases, a potnential mitigation is to implement reference counting in
  controller-runtime such that an informer is aware of any and all outstanding
  references when its removal is called.

* Safety of stopping individual informers. There is concern that stopping
  individual informers will leak go routines or memory. We should be able to use
  pprof tooling and exisiting leak tooling in controller-runtime to identify and
  mitigate any leaks

#### Alternatives

* Creating a cache per watch (i.e. cache of caches) as the end user. The advantage
  of this is that it prevents having to modify any code in controller-runtime.
  The main drawback is that it's very clunky to maintain multiple caches (one
  for each informer) and breaks the clean design of the cache.

* Adding support to de-register EventHandlers from informers in apimachinery.
  This along with ref counting would be cleanest way to free us of the concern
  of controllers silently failing when their watch is removed.
  The downside is that we are ultimately at the mercy of whether apimachinery
  wants to support these changes, and even if they were on board, it could take
  a long time to land these changes upstream.

* TODO: Bubbling up errors from apimachinery.


### Minimal hooks needed to use informer removal externally

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

* We lack a consistent story around multi-cluster support and introducing
  changes such as this without fully thinking through the multi-cluster story
  might bind us for future designs. We think that restarting
  controllers is a valid use-case even for single cluster regardless of the
  multi-cluster use case.

* [#1139](https://github.com/kubernetes-sigs/controller-runtime/pull/1139) discusses why
the ability to start a controller more than once was taken away. It's a little
unclear what effect explicitly enabling multiple starts in the case of
conditional controllers will have on the number of workers and workqueues
relative to expectations and metrics.

* [#1163](https://github.com/kubernetes-sigs/controller-runtime/pull/1163) discusses the
memory leak caused by no clearing out the watches internal slice. A possible
mitigation is to clear out the slices upon ConditionalController shutdown.

#### Alternatives

* A metacontroller or CRD controller could start and stop controllers based on
the existence of their corresponding CRDs. This puts the complexity of designing such a controller
onto the end user, but there are potentially ways to provide end users with
default, pluggable CRD controllers. More importantly, this probably is not even
be sufficient for enabling controller restarting, because informers are shared
between all controllers so restarting the controller will still try to use the
informer that is erroring out if the CRD it is watching goes away. Some hooks
into removing informers is sitll required in order to use a metacontroller.

* Instead of exposing ResetStart and SaveWatches on the internal controller struct
it might be better to expose it on the controller interface. This is more public
and creates more potential for abuse, but it prevents some hacky solutions
discussed below around needing to cast to the internal controller or create
extra interfaces.

## Future Work / Motivating Use-Cases

Were this to move forward, it unlocks a number of potential use-cases.

1. OPA/Gatekeeper can simplify it's dynamic watch management by having greater
   controller over the lifecycle of a controller. See [this
   doc](https://docs.google.com/document/d/1Wi3LM3sG6Qgfzm--bWb6R0SEKCkQCCt-ene6cO62FlM/edit)

2. We can support controllers that can conditionally start/stop/restart based on
   the installation/uninstallation of its CRD. See [this proof-of-concept branch](https://github.com/kevindelgado/controller-runtime/tree/experimental/conditional-runnables)

## Timeline of Events
* 9/30/2020: Propose idea in design doc and proof-of-concept PR to
controller-runtime
* 10/7/2020: Design doc and proof-of-concept distilled to focus only on minimal
  hooks needed rather than proposing entire conditional controller solution.
  Alternatives added to discuss opportunities to push some of the implementation
  to the api-machinery level.
* 10/8/2020: Discuss idea in community meeting

