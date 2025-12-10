Add Support for Warm Replicas
=============================

## Summary

When controllers manage huge caches, failover takes minutes because follower replicas wait to win leader election before starting informers. “Warm replicas” allow controller-runtime to start sources while a manager instance is still on standby, so the new leader can immediately schedule workers with already-populated queues. This design documents the feature implemented in [PR #3192](https://github.com/kubernetes-sigs/controller-runtime/pull/3192) and answers the outstanding review questions.

## Motivation

Controllers reconcile every object from their sources at startup and after leader failover. For sources with millions of objects (e.g., Secrets, ConfigMaps, custom resources across all namespaces) the initial List+Watch can take tens of minutes, delaying recovery. Today a controller only starts its sources inside `Start`, which manager runs **after** acquiring the leader lock. That guarantees downtime equal to the cache warmup time whenever the leader rotates.

## Goals

- Allow controller authors to opt a controller (or all controllers) into warmup behavior with a single option (`EnableWarmup`).
- Ensure warmup never changes behavior when disabled.
- Keep the API surface minimal (no exported warmup interface yet).

## Implemented Changes

### Manager Warmup Phase

Manager already buckets runnables (HTTP servers, caches, others, leader election). We added an internal `warmupRunnable` interface:

```go
type warmupRunnable interface {
	Warmup(context.Context) error
}
```

During `Start`, the manager now runs:

1. HTTP servers
2. Webhooks
3. Caches
4. `Others`
5. **Warmup runnables (new)**
6. Leader election runnables once the lock is acquired

Warmup runnables are also stopped in parallel with non-leader runnables during shutdown to avoid deadlocks.

### Controller Opt-in

Controllers expose the option via:

- `ctrl.Options{Controller: config.Controller{EnableWarmup: ptr.To(true)}}`

If both `EnableWarmup` and `NeedLeaderElection` are true, controller-runtime registers the controller as a warmup runnable. Calling `Warmup` launches the same event sources and cache sync logic as `Start`, but it does **not** start worker goroutines. Once the manager becomes leader, the controller’s normal `Start` simply spins up workers against the already-initialized queue. Enabling warmup on a controller that does not use leader election is a no-op, as the worker threads do not block on leader election being won.

### Usage Example

```go
mgr, err := ctrl.NewManager(cfg, ctrl.Options{
	Controller: config.Controller{
		EnableWarmup: ptr.To(true), // make every controller warm up
	},
})
if err != nil {
    panic(err)
}

builder.ControllerManagedBy(mgr).
	Named("slow-source").
	WithOptions(controller.Options{
		EnableWarmup: ptr.To(true), // optional per-controller override
	}).
	For(&examplev1.Example{}).
	Complete(reconciler)
```

### Operational Considerations

- **API server load** – Warm replicas temporarily duplicate List/Watch traffic: each standby replica performs the initial List and opens watches even though the current leader is already doing so. The additional load exists only while a replica is warming its caches, but on huge clusters this can still be expensive depending on the number of warm replicas.
- **Queue depth metrics** – Because warm replicas start their sources before workers run, the `workqueue_depth` metric spikes during warmup even though reconcilers have not begun processing. Alerting or SLOs based on that metric should either ignore the warmup window or switch to per-controller gauges that reset when workers start.

### References

- Implementation: [#3192](https://github.com/kubernetes-sigs/controller-runtime/pull/3192)
- Earlier context: [#2005](https://github.com/kubernetes-sigs/controller-runtime/pull/2005), [#2600](https://github.com/kubernetes-sigs/controller-runtime/issues/2600)
