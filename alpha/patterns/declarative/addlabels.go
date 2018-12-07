package declarative

import (
	"context"

	"sigs.k8s.io/controller-runtime/alpha/patterns/declarative/pkg/manifest"
)

// AddLabels returns an ObjectTransform that adds labels to all the objects
func AddLabels(labels map[string]string) ObjectTransform {
	return func(ctx context.Context, o DeclarativeObject, manifest *manifest.Objects) error {
		// TODO: Add to selectors and labels in templates?
		for _, o := range manifest.Items {
			o.AddLabels(labels)
		}

		return nil
	}
}
