Add support for warm replicas
===================

## Motivation
Controllers reconcile all objects during startup / leader election failover to account for changes
in the reconciliation logic. For certain sources, the time to serve the initial list can be
significant in the order of minutes. This is problematic because by default controllers (and by
extension watches) do not start until they have won leader election. This implies guaranteed
downtime as even after leader election, the controller has to wait for the initial list to be served
before it can start reconciling.

## Proposal
A warm replica is a replica with a queue pre-filled by sources started even when leader election is
not won so that it is ready to start processing items as soon as the leader election is won. This
proposal aims to add support for warm replicas in controller-runtime.

### Context
Mostly written to confirm my understanding, but also to provide context for the proposal.

Controllers are a monolithic runnable with a `Start(ctx)` that
1. Starts the watches [here](https://github.com/kubernetes-sigs/controller-runtime/blob/v0.20.2/pkg/internal/controller/controller.go#L196-L213)
2. Starts the workers [here](https://github.com/kubernetes-sigs/controller-runtime/blob/v0.20.2/pkg/internal/controller/controller.go#L244-L252)
There needs to be a way to decouple the two so that the watches can be started before the workers
even as part of the same Runnable.

If a runnable implements the `LeaderElectionRunnable` interface, the return value of the
`NeedLeaderElection` function dictates whether or not it gets binned into the leader election
runnables group [code](https://github.com/kubernetes-sigs/controller-runtime/blob/v0.20.2/pkg/manager/runnable_group.go).

Runnables in the leader election group are started only after the manager has won leader election,
and all controllers are leader election runnables by default.

### Design
1. Add a new interface `WarmupRunnable` that allows for leader election runnables to specify
behavior when manager is not in leader mode. This interface should be as follows:
```go
type WarmupRunnable interface {
    NeedWarmup() bool
    GetWarmupRunnable() Runnable
}
```

2. Controllers will implement this interface to specify behavior when the manager is not the leader.
Add a new controller option `ShouldWarmupWithoutLeadership`. If set to true, then the main
controller runnable will not start sources, and instead rely on the warmup runnable to start sources
The option will be used as follows:
```go
type Controller struct {
    // ...

    // ShouldWarmupWithoutLeadership specifies whether the controller should start its sources
    // when the manager is not the leader.
    // Defaults to false, which means that the controller will wait for leader election to start
    // before starting sources.
    ShouldWarmupWithoutLeadership *bool

    // ...
}

type runnableWrapper struct {
    startFunc func (ctx context.Context) error
}

func(rw runnableWrapper) Start(ctx context.Context) error {
    return rw.startFunc(ctx)
}

// NeedWarmup implements WarmupRunnable
func (c *Controller[request]) NeedWarmup() bool {
    if c.ShouldWarmupWithoutLeadership == nil {
        return false
    }
    return c.ShouldWarmupWithoutLeadership
}

// GetWarmupRunnable implements WarmupRunnable
func (c *Controller[request]) GetWarmupRunnable() Runnable {
    return runnableWrapper{
        startFunc: func (ctx context.Context) error {
            if !c.ShouldWarmupWithoutLeadership {
                return nil
            }

            // pseudocode
            for _, watch := c.startWatches {
                watch.Start()
                // handle syncing sources
            }
        }
    }
}
```

3. Add a separate runnable category for warmup runnables to specify behavior when the
manager is not the leader. [ref](https://github.com/kubernetes-sigs/controller-runtime/blob/v0.20.2/pkg/manager/runnable_group.go#L55-L76)
```go
type runnables struct {
    // ...

	LeaderElection *runnableGroup
    Warmup *runnableGroup

    // ...
}

func (r *runnables) Add(fn Runnable) error {
	switch runnable := fn.(type) {
    // ...
    case WarmupRunnable:
        if runnable.NeedWarmup() {
            r.Warmup.Add(runnable.GetWarmupRunnable(), nil)
        }

        // fallthrough to ensure that a runnable that implements both LeaderElection and
        // NonLeaderElection interfaces is added to both groups
        fallthrough
	case LeaderElectionRunnable:
		if !runnable.NeedLeaderElection() {
			return r.Others.Add(fn, nil)
		}
		return r.LeaderElection.Add(fn, nil)
    // ...
	}
}
```

4. Start the non-leader runnables during manager startup.
```go
func (cm *controllerManager) Start(ctx context.Context) (err error) {
    // ...

    // Start the warmup runnables
	if err := cm.runnables.Warmup.Start(cm.internalCtx); err != nil {
		return fmt.Errorf("failed to start other runnables: %w", err)
	}

    // ...
}
```

## Concerns/Questions
1. Controllers opted into this feature will break the workqueue.depth metric as the controller will
   have a pre filled queue before it starts processing items.
2. Ideally, non-leader runnables should block readyz and healthz checks until they are in sync. I am
   not sure what the best way of implementing this is, because we would have to add a healthz check
   that blocks on WaitForSync for all the sources started as part of the non-leader runnables.
3. An alternative way of implementing the above is to moving the source starting / management code
   out into their own runnables instead of having them as part of the controller runnable and
   exposing a method to fetch the sources. I am not convinced that that is the right change as it
   would introduce the problem of leader election runnables potentially blocking each other as they
   wait for the sources to be in sync.
