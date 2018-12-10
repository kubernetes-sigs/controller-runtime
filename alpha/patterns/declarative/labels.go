package declarative

import (
	"context"
	"fmt"

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

// SourceLabel returns a fixed label based on the type and name of the DeclarativeObject
func SourceLabel(ctx context.Context, o DeclarativeObject) map[string]string {
	gvk := o.GetObjectKind().GroupVersionKind()

	return map[string]string{
		fmt.Sprintf("%s/%s", gvk.Group, gvk.Kind): o.GetName(),
	}
}
