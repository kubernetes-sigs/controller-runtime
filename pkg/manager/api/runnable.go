package api

import "context"

// BaseContextFunc is a function used to provide a base Context to Runnables
// managed by a Manager.
type BaseContextFunc func() context.Context

// Runnable allows a component to be started.
// It's very important that Start blocks until
// it's done running.
type Runnable interface {
	// Start starts running the component.  The component will stop running
	// when the context is closed. Start blocks until the context is closed or
	// an error occurs.
	Start(context.Context) error
}

// RunnableFunc implements Runnable using a function.
// It's very important that the given function block
// until it's done running.
type RunnableFunc func(context.Context) error

// Start implements Runnable.
func (r RunnableFunc) Start(ctx context.Context) error {
	return r(ctx)
}

// LeaderElectionRunnable knows if a Runnable needs to be run in the leader election mode.
type LeaderElectionRunnable interface {
	// NeedLeaderElection returns true if the Runnable needs to be run in the leader election mode.
	// e.g. controllers need to be run in leader election mode, while webhook server doesn't.
	NeedLeaderElection() bool
}
