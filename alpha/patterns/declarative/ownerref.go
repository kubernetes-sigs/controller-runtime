package declarative

import (
	"context"

	"sigs.k8s.io/controller-runtime/alpha/patterns/declarative/pkg/manifest"
)

// SourceAsOwner is a OwnerSelector that selects the source DeclarativeObject as the owner
func SourceAsOwner(ctx context.Context, src DeclarativeObject, obj manifest.Object, objs manifest.Objects) (DeclarativeObject, error) {
	return src, nil
}

var _ OwnerSelector = SourceAsOwner
