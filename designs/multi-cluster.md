# Multi-Cluster Support

Author: @sttts @embik

Initial implementation: @vincepri

Last Updated on: 2025-01-06

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

With this change, it will be possible to implement pluggable cluster providers
that automatically start and stop sources (and thus, cluster-aware reconcilers) when
the cluster provider adds ("engages") or removes ("disengages") a cluster.

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

- Allow 3rd-parties to implement an (optional) multi-cluster provider Go interface that controller-runtime will use (if configured on the manager) to dynamically attach and detach registered controllers to clusters that come and go.
- With that, provide a way to natively write controllers for these patterns:
  1. (UNIFORM MULTI-CLUSTER CONTROLLERS) operate on multiple clusters in a uniform way,
     i.e. reconciling the same resources on multiple clusters, **optionally**
     - sourcing information from one central hub cluster
     - sourcing information cross-cluster.

      Example: distributed `ReplicaSet` controller, reconciling `ReplicaSets` on multiple clusters.
  2. (AGGREGATING MULTI-CLUSTER CONTROLLERS) operate on one central hub cluster aggregating information from multiple clusters.
     
     Example: distributed `Deployment` controller, aggregating `ReplicaSets` across multiple clusters back into a central `Deployment` object.

#### Low-Level Requirements

- Allow event sources to be cross-cluster such that:
  1. Multi-cluster events can trigger reconciliation in the one central hub cluster.
  2. Central hub cluster events can trigger reconciliation on multiple clusters.
- Allow reconcilers to look up objects through (informer) indexes from specific other clusters.
- Minimize the amount of changes to make a controller-runtime controller 
  multi-cluster-compatible, in a way that 3rd-party projects have no reason to
  object to these kind of changes.

Here we call a controller to be multi-cluster-compatible if the reconcilers get
reconcile requests in cluster `X` and do all reconciliation in cluster `X`. This 
is less than being multi-cluster-aware, where reconcilers implement cross-cluster
logic.

### Examples

- Run a controller-runtime controller against a kubeconfig with arbitrary many contexts, all being reconciled.
- Run a controller-runtime controller against cluster managers like kind, Cluster API, Open-Cluster-Manager or Hypershift.
- Run a controller-runtime controller against a kcp shard with a wildcard watch.

### Non-Goals/Future Work

- Ship integration for different multi-cluster setups. This should become 
  out-of-tree subprojects that can individually evolve and vendor'ed by controller authors.
- Make controller-runtime controllers "binary pluggable".
- Manage one manager per cluster.
- Manage one controller per cluster with dedicated workqueues.

## Proposal

The `ctrl.Manager` _SHOULD_ be extended to get an optional `cluster.Provider` via
`ctrl.Options`, implementing:

```golang
// pkg/cluster

// Provider defines methods to retrieve clusters by name. The provider is
// responsible for discovering and managing the lifecycle of each cluster.
//
// Example: A Cluster API provider would be responsible for discovering and
// managing clusters that are backed by Cluster API resources, which can live
// in multiple namespaces in a single management cluster.
type Provider interface {
	// Get returns a cluster for the given identifying cluster name. Get
	// returns an existing cluster if it has been created before.
	Get(ctx context.Context, clusterName string) (Cluster, error)
}
```

A cluster provider is responsible for constructing a `cluster.Cluster` instance
upon calls to `Get(ctx, clusterName)` and returning it. Providers should keep track
of created clusters and return them again if the same name is requested. Since
providers are responsible for constructing the `cluster.Cluster` instance, they
can make decisions about e.g. reusing existing informers.

The `cluster.Cluster` _SHOULD_ be extended with a unique name identifier:

```golang
// pkg/cluster:
type Cluster interface {
    Name() string
	...
}
```

A new interface for cluster-aware runnables will be provided:

```golang
// pkg/cluster
type Aware interface {
	// Engage gets called when the component should start operations for the given Cluster.
	// The given context is tied to the Cluster's lifecycle and will be cancelled when the
	// Cluster is removed or an error occurs.
	//
	// Implementers should return an error if they cannot start operations for the given Cluster,
	// and should ensure this operation is re-entrant and non-blocking.
	//
	//	\_________________|)____.---'--`---.____
	//              ||    \----.________.----/
	//              ||     / /    `--'
	//            __||____/ /_
	//           |___         \
	//               `--------'
	Engage(context.Context, Cluster) error

	// Disengage gets called when the component should stop operations for the given Cluster.
	Disengage(context.Context, Cluster) error
}
```

`ctrl.Manager` will implement `cluster.Aware`. It is the cluster provider's responsibility
to call `Engage` and `Disengage` on a `ctrl.Manager` instance when clusters join or leave
the set of target clusters that should be reconciled.

The internal `ctrl.Manager` implementation in turn will call `Engage` and `Disengage` on all 
its runnables that are cluster-aware (i.e. that implement the `cluster.Aware` interface).

In particular, cluster-aware controllers implement the `cluster.Aware` interface and are
responsible for starting watches on clusters when they are engaged. This is expressed through
the interface below:

```golang
// pkg/controller
type TypedMultiClusterController[request comparable] interface {
	cluster.Aware
	TypedController[request]
}
```

The multi-cluster controller implementation reacts to engaged clusters by starting
a new `TypedSyncingSource` that also wraps the context passed down from the call to `Engage`,
which _MUST_ be canceled by the cluster provider at the end of a cluster's lifecycle.

Instead of extending `reconcile.Request`, implementations _SHOULD_ bring their
own request type through the generics support in `Typed*` types (`request comparable`).

Optionally, a passed `TypedEventHandler` will be duplicated per engaged cluster if they
fullfil the following interface:

```golang
// pkg/handler
type TypedDeepCopyableEventHandler[object any, request comparable] interface {
	TypedEventHandler[object, request]
	DeepCopyFor(c cluster.Cluster) TypedDeepCopyableEventHandler[object, request]
}
```

This might be necessary if a BYO `TypedEventHandler` needs to store information about
the engaged cluster (e.g. because the events do not supply information about the cluster in
object annotations) that it has been started for.

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


### Multi-Cluster-Compatible Reconcilers

Reconcilers can be made multi-cluster-compatible by changing client and cache
accessing code from directly accessing `mgr.GetClient()` and `mgr.GetCache()` to
going through `mgr.GetCluster(clusterName).GetClient()` and
`mgr.GetCluster(clusterName).GetCache()`. `clusterName` needs to be extracted from
the BYO `request` type (e.g. a `clusterName` field in the type itself).

A typical snippet at the beginning of a reconciler to fetch the client could look like this:

```golang
cl, err := mgr.GetCluster(ctx, req.ClusterName)
if err != nil {
	return reconcile.Result{}, err
}
client := cl.GetClient()
```

Due to the BYO `request` type, controllers using the `For` builder function need to be built/changed like this:

```golang
// previous
builder.TypedControllerManagedBy[reconcile.Request](mgr).
	Named("single-cluster-controller").
	For(&corev1.Pod{}).
	Complete(reconciler)

// new
builder.TypedControllerManagedBy[ClusterRequest](mgr).
	Named("multi-cluster-controller").
	Watches(&corev1.Pod{}, &ClusterRequestEventHandler{}).
	Complete(reconciler)
```

With `ClusterRequest` and `ClusterRequestEventHandler` being BYO types. `reconciler`
can be e.g. of type `reconcile.TypedFunc[ClusterRequest]`.

`ClusterRequest` will likely often look like this (but since it is a BYO type, it could store other information as well):

```golang
type ClusterRequest struct {
	reconcile.Request
	ClusterName string
}
```

Controllers that use `Owns` cannot be converted to multi-cluster controllers
without a BYO type re-implementation of `handler.EnqueueRequestForOwner` matching
the BYO type, which is considered out of scope for now.

With the described changes (use `GetCluster(ctx, clusterName)`, making `reconciler`
a `TypedFunc[ClusterRequest]` and migrating to `Watches`) an existing controller will automatically act as
*uniform multi-cluster controller*. It will reconcile resources from cluster `X`
in cluster `X`.

For a manager with `cluster.Provider`, the builder _SHOULD_ create a controller
that sources events **ONLY** from the provider clusters that got engaged with
the controller.

Controllers that should be triggered by events on the hub cluster can continue
to use `For` and `Owns` and explicitly pass the intention to engage only with the
"default" cluster (this is only necessary if a cluster provider is plugged in):

```golang
builder.NewControllerManagedBy(mgr).
    WithOptions(controller.TypedOptions{
        EngageWithDefaultCluster: ptr.To(true),
        EngageWithProviderClusters: ptr.To(false),
    }).
    For(&appsv1.Deployment{}).
	Owns(&v1.ReplicaSet{}).
    Complete(reconciler)
```

## User Stories

### Controller Author with no interest in multi-cluster wanting to old behaviour.

- Do nothing. Controller-runtime behaviour is unchanged.

### Multi-Cluster Integrator wanting to support cluster managers like Cluster API or kind

- Implement the `cluster.Provider` interface, either via polling of the cluster registry
  or by watching objects in the hub cluster.
- For every new cluster create an instance of `cluster.Cluster` and call `mgr.Engage`.

### Multi-Cluster Integrator wanting to support apiservers with logical cluster (like kcp)

- Implement the `cluster.Provider` interface by watching the apiserver for logical cluster objects
  (`LogicalCluster` CRD in kcp).
- Return a facade `cluster.Cluster` that scopes all operations (client, cache, indexers) 
  to the logical cluster, but backed by one physical `cluster.Cluster` resource.
- Add cross-cluster indexers to the physical `cluster.Cluster` object.

### Controller Author without self-interest in multi-cluster, but open for adoption in multi-cluster setups

- Replace `mgr.GetClient()` and `mgr.GetCache` with `mgr.GetCluster(req.ClusterName).GetClient()` and `mgr.GetCluster(req.ClusterName).GetCache()`.
- Switch from `For` and `Owns` builder calls to `watches`
- Make manager and controller plumbing vendor'able to allow plugging in multi-cluster provider and BYO request type.

### Controller Author who wants to support certain multi-cluster setups

- Do the `GetCluster` plumbing as described above.
- Vendor 3rd-party multi-cluster providers and wire them up in `main.go`

## Risks and Mitigations

- The standard behaviour of controller-runtime is unchanged for single-cluster controllers.
- The activation of the multi-cluster mode is through attaching the `cluster.Provider` to the manager. 
  To make it clear that the semantics are experimental, we name the `manager.Options` field
  `ExperimentalClusterProvider`.
- We only extend these interfaces and structs:
  - `ctrl.Manager` with `GetCluster(ctx, clusterName string) (cluster.Cluster, error)` and `cluster.Aware`.
  - `cluster.Cluster` with `Name() string`.
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

## Implementation History

- [PR #2207 by @vincepri : WIP: ✨ Cluster Provider and cluster-aware controllers](https://github.com/kubernetes-sigs/controller-runtime/pull/2207) – with extensive review
- [PR #2726 by @sttts replacing #2207: WIP: ✨ Cluster Provider and cluster-aware controllers](https://github.com/kubernetes-sigs/controller-runtime/pull/2726) – 
  picking up #2207, addressing lots of comments and extending the approach to what kcp needs, with a `fleet-namespace` example that demonstrates a similar setup as kcp with real logical clusters.
- [PR #3019 by @embik, replacing #2726: ✨ WIP: Cluster provider and cluster-aware controllers](https://github.com/kubernetes-sigs/controller-runtime/pull/3019) - 
  picking up #2726, reworking existing code to support the recent `Typed*` generic changes of the codebase.
- [github.com/kcp-dev/controller-runtime](https://github.com/kcp-dev/controller-runtime) – the kcp controller-runtime fork
