package declarative

import (
	"context"

	"sigs.k8s.io/controller-runtime/alpha/patterns/declarative/pkg/manifest"
)

// Status provides health and readiness information for a given DeclarativeObject
type Status interface {
	Reconciled
	Preflight
}

type Reconciled interface {
	// Reconciled is triggered when Reconciliation has occured.
	// The caller is encouraged to determine and surface the health of the reconcilation
	// on the DeclarativeObject.
	Reconciled(context.Context, DeclarativeObject, *manifest.Objects) error
}

type Preflight interface {
	// Preflight validates if the current state of the world is ready for reconciling.
	// Returning a non-nil error on this object will prevent Reconcile from running.
	// The caller is encouraged to surface the error status on the DeclarativeObject.
	Preflight(context.Context, DeclarativeObject) error
}

// StatusBuilder provides a pluggable implementation of Status
type StatusBuilder struct {
	ReconciledImpl Reconciled
	PreflightImpl  Preflight
}

func (s *StatusBuilder) Reconciled(ctx context.Context, src DeclarativeObject, objs *manifest.Objects) error {
	if s.ReconciledImpl != nil {
		return s.ReconciledImpl.Reconciled(ctx, src, objs)
	}
	return nil
}

func (s *StatusBuilder) Preflight(ctx context.Context, src DeclarativeObject) error {
	if s.PreflightImpl != nil {
		return s.PreflightImpl.Preflight(ctx, src)
	}
	return nil
}

var _ Status = &StatusBuilder{}
