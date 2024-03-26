# Multi-Cluster Support
Author: @sttts
Initial implementation: @vincepri

Last Updated on: 03/26/2024

## Table of Contents

<!--ts-->
- [Multi-Cluster Support](#multi-cluster-support)
  - [Table of Contents](#table-of-contents)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Examples](#examples)
    - [Non-Goals/Future Work](#non-goalsfuture-work)
  - [Proposal](#proposal)
    - [Multi-Cluster-Compatible Reconcilers](#multi-cluster-compatible-reconcilers)
  - [User Stories](#user-stories)
    - [Controller Author with no interest in multi-cluster wanting to old behaviour.](#controller-author-with-no-interest-in-multi-cluster-wanting-to-old-behaviour)
    - [Multi-Cluster Integrator wanting to support cluster managers like Cluster-API or kind](#multi-cluster-integrator-wanting-to-support-cluster-managers-like-cluster-api-or-kind)
    - [Multi-Cluster Integrator wanting to support apiservers with logical cluster (like kcp)](#multi-cluster-integrator-wanting-to-support-apiservers-with-logical-cluster-like-kcp)
    - [Controller Author without self-interest in multi-cluster, but open for adoption in multi-cluster setups](#controller-author-without-self-interest-in-multi-cluster-but-open-for-adoption-in-multi-cluster-setups)
    - [Controller Author who wants to support certain multi-cluster setups](#controller-author-who-wants-to-support-certain-multi-cluster-setups)
  - [Risks and Mitigations](#risks-and-mitigations)
  - [Alternatives](#alternatives)
  - [Implementation History](#implementation-history)

<!--te-->

## Summary

Controller-runtime today allows to write controllers against one cluster only.
Multi-cluster use-cases require the creation of multiple managers and/or cluster
objects. This proposal is about adding native support for multi-cluster use-cases
to controller-runtime.

## Motivation

This change is important because:
- multi-cluster use-cases are becoming more and more common, compare projects
  like Kamarda, Crossplane or kcp. They all need to write (controller-runtime)
  controllers that operate on multiple clusters.
- writing controllers for upper systems in a **portable** way is hard today.
  Consequently, there is no multi-cluster controller ecosystem, but could and
  should be.
- kcp maintains a [controller-runtime fork with multi-cluster support](https://github.com/kcp-dev/controller-runtime)
  because adding support on-top leads to an inefficient controller design, and
  even more important leads of divergence in the ecosystem.

### Goals

- Provide a way to natively write controllers that
  1. (UNIFORM MULTI-CLUSTER CONTROLLER) operate on multiple clusters in a uniform way,
     i.e. reconciling the same resources on multiple clusters, **optionally**
     - sourcing information from one central hub cluster
     - sourcing information cross-cluster.

     Example: distributed `ReplicaSet` controller, reconciling `ReplicaSets` on multiple clusters.
  2. (AGGREGATING MULTI-CLUSTER CONTROLLER) operate on one central hub cluster aggregating information from multiple clusters.
     
     Example: distributed `Deployment` controller, aggregating `ReplicaSets` back into the `Deployment` object.
- Allow clusters to dynamically join and leave the set of clusters a controller operates on.
- Allow event sources to be cross-cluster:
  1. Multi-cluster events that trigger reconciliation in the one central hub cluster.
  2. Central hub cluster events to trigger reconciliation on multiple clusters.
- Allow (informer) indexes that span multiple clusters.
- Allow logical clusters where a set of clusters is actually backed by one physical informer store.
- Allow 3rd-parties to plug in their multi-cluster adapter (in source code) into
  an existing multi-cluster-compatible code-base.
- Minimize the amount of changes to make a controller-runtime controller 
  multi-cluster-compatible, in a way that 3rd-party projects have no reason to
  object these kind of changes.

Here we call a controller to be multi-cluster-compatible if the reconcilers get
reconcile requests in cluster `X` and do all reconciliation in cluster `X`. This 
is less than being multi-cluster-aware, where reconcilers implement cross-cluster
logic.

### Examples

- Run a controller-runtime controller against a kubeconfig with arbitrary many contexts, all being reconciled.
- Run a controller-runtime controller against cluster-managers like kind, Cluster-API, Open-Cluster-Manager or Hypershift.
- Run a controller-runtime controller against a kcp shard with a wildcard watch.

### Non-Goals/Future Work

- Ship integration for different multi-cluster setups. This should become 
  out-of-tree subprojects that can individually evolve and vendor'ed by controller authors.
- Make controller-runtime controllers "binary pluggable".
- Manage one manager per cluster.
- Manage one controller per cluster with dedicated workqueues.

## Proposal

The `ctrl.Manager` _SHOULD_ be extended to get an optional `cluster.Provider` via
`ctrl.Options` implementing

```golang
// pkg/cluster
type Provider interface {
   Get(ctx context.Context, clusterName string, opts ...Option) (Cluster, error)
   List(ctx context.Context) ([]string, error)
   Watch(ctx context.Context) (Watcher, error)
}
```
The `cluster.Cluster` _SHOULD_ be extended with a unique name identifier:
```golang
// pkg/cluster:
type Cluster interface {
    Name() string
	...
}
```

The `ctrl.Manager` will use the provider to watch clusters coming and going, and
will inform runnables implementing the `cluster.AwareRunnable` interface:

```golang
// pkg/cluster
type AwareRunnable interface {
	Engage(context.Context, Cluster) error
	Disengage(context.Context, Cluster) error
}
```
In particular, controllers implement the `AwareRunnable` interface. They react
to engaged clusters by duplicating and starting their registered `source.Source`s
and `handler.EventHandler`s for each cluster through implementation of
```golang
// pkg/source
type DeepCopyableSyncingSource interface {
	SyncingSource
	DeepCopyFor(cluster cluster.Cluster) DeepCopyableSyncingSource
}

// pkg/handler
type DeepCopyableEventHandler interface {
    EventHandler
    DeepCopyFor(c cluster.Cluster) DeepCopyableEventHandler
}
```
The standard implementing types, in particular `internal.Kind` will adhere to
these interfaces.

The `ctrl.Manager` _SHOULD_ be extended by a `cluster.Cluster` getter:
```golang
// pkg/manager
type Manager interface {
    // ...
    GetCluster(ctx context.Context, clusterName string) (cluster.Cluster, error)
}
```
The embedded `cluster.Cluster` corresponds to `GetCluster(ctx, "")`. We call the
clusters with non-empty name "provider clusters" or "enganged clusters", while
the embedded cluster of the manager is called the "default cluster" or "hub 
cluster".

The `reconcile.Request` _SHOULD_ be extended by an optional `ClusterName` field:
```golang
// pkg/reconile
type Request struct {
    ClusterName string
    types.NamespacedName
}
```

With these changes, the behaviour of controller-runtime without a set cluster
provider will be unchanged.

### Multi-Cluster-Compatible Reconcilers

Reconcilers can be made multi-cluster-compatible by changing client and cache
accessing code from directly accessing `mgr.GetClient()` and `mgr.GetCache()` to
going through `mgr.GetCluster(req.ClusterName).GetClient()` and
`mgr.GetCluster(req.ClusterName).GetCache()`.

When building a controller like
```golang
builder.NewControllerManagedBy(mgr).
    For(&appsv1.ReplicaSet{}).
	Owns(&v1.Pod{}).
    Complete(reconciler)
```
with the described change to use `GetCluster(ctx, req.ClusterName)` will automatically 
act as *uniform multi-cluster controller*. It will reconcile resources from cluster `X`
in cluster `X`.

For a manager with `cluster.Provider`, the builder _SHOULD_ create a controller
that sources events **ONLY** from the provider clusters that got engaged with
the controller.

Controllers that should be triggered by events on the hub cluster will have to
opt-in like in this example:

```golang
builder.NewControllerManagedBy(mgr).
    For(&appsv1.Deployment{}, builder.InDefaultCluster).
	Owns(&v1.ReplicaSet{}).
    Complete(reconciler)
```
A mixed set of sources is possible as shown here in the example.

## User Stories

### Controller Author with no interest in multi-cluster wanting to old behaviour.

- Do nothing. Controller-runtime behaviour is unchanged.

### Multi-Cluster Integrator wanting to support cluster managers like Cluster-API or kind

- Implement the `cluster.Provider` interface, either via polling of the cluster registry
  or by watching objects in the hub cluster.
- For every new cluster create an instance of `cluster.Cluster`.

### Multi-Cluster Integrator wanting to support apiservers with logical cluster (like kcp)

- Implement the `cluster.Provider` interface by watching the apiserver for logical cluster objects
  (`LogicalCluster` CRD in kcp).
- Return a facade `cluster.Cluster` that scopes all operations (client, cache, indexers) 
  to the logical cluster, but backed by one physical `cluster.Cluster` resource.
- Add cross-cluster indexers to the physical `cluster.Cluster` object.

### Controller Author without self-interest in multi-cluster, but open for adoption in multi-cluster setups

- Replace `mgr.GetClient()` and `mgr.GetCache` with `mgr.GetCluster(req.ClusterName).GetClient()` and `mgr.GetCluster(req.ClusterName).GetCache()`.
- Make manager and controller plumbing vendor'able to allow plugging in multi-cluster provider.

### Controller Author who wants to support certain multi-cluster setups

- Do the `GetCluster` plumbing as described above.
- Vendor 3rd-party multi-cluster providers and wire them up in `main.go`

## Risks and Mitigations

- The standard behaviour of controller-runtime is unchanged for single-cluster controllers.
- The activation of the multi-cluster mode is through attaching the `cluster.Provider` to the manager. 
  To make it clear that the semantics are experimental, we make the `Options.provider` field private
  and adds `Options.WithExperimentalClusterProvider` method.
- We only extend these interfaces and structs:
  - `ctrl.Manager` with `GetCluster(ctx, clusterName string) (cluster.Cluster, error)`
  - `cluster.Cluster` with `Name() string`
  - `reconcile.Request` with `ClusterName string`
  We think that the behaviour of these extensions is well understood and hence low risk.
  Everything else behind the scenes is an implementation detail that can be changed
  at any time.

## Alternatives

- Multi-cluster support could be built outside of core controller-runtime. This would
  lead likely to a design with one manager per cluster. This has a number of problems:
  - only one manager can serve webhooks or metrics
  - cluster management must be custom built
  - logical cluster support would still require a fork of controller-runtime and
    with that a divergence in the ecosystem. The reason is that logical clusters
    require a shared workqueue because they share the same apiserver. So for
    fair queueing, this needs deep integration into one manager.
  - informers facades are not supported in today's cluster/cache implementation.
- We could deepcopy the builder instead of the sources and handlers. This would
  lead to one controller and one workqueue per cluster. For the reason outlined
  in the previous alternative, this is not desireable.
- We could skip adding `ClusterName` to `reconcile.Request` and instead pass the
  cluster through in the context. On the one hand, this looks attractive as it
  would avoid having to touch reconcilers at all to make them multi-cluster-compatible.
  On the other hand, with `cluster.Cluster` embedded into `manager.Manager`, not
  every method of `cluster.Cluster` carries a context. So virtualizing the cluster
  in the manager leads to contradictions in the semantics.

  For example, it can well be that every cluster has different REST mapping because
  installed CRDs are different. Without a context, we cannot return the right
  REST mapper.

  An alternative would be to add a context to every method of `cluster.Cluster`,
  which is a much bigger and uglier change than what is proposed here.


## Implementation History

- [PR #2207 by @vincepri : WIP: ✨ Cluster Provider and cluster-aware controllers](https://github.com/kubernetes-sigs/controller-runtime/pull/2207) – with extensive review
- [PR #2208 by @sttts replace #2207: WIP: ✨ Cluster Provider and cluster-aware controllers](https://github.com/kubernetes-sigs/controller-runtime/pull/2726) – 
  picking up #2207, addressing lots of comments and extending the approach to what kcp needs, with a `fleet-namespace` example that demonstrates a similar setup as kcp with real logical clusters.
- [github.com/kcp-dev/controller-runtime](https://github.com/kcp-dev/controller-runtime) – the kcp controller-runtime fork
