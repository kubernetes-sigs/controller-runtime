package declarative

import (
	"context"

	"sigs.k8s.io/controller-runtime/alpha/patterns/declarative/pkg/manifest"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

// This will likely become a topological sort in the future, for owner refs.
// For now, we implement a simple kind-based heuristic for the sort.

func DefaultObjectOrder(ctx context.Context) func(o *manifest.Object) int {
	log := log.Log

	return func(o *manifest.Object) int {
		gk := o.Group + "/" + o.Kind
		switch gk {
		// Create CRDs asap - both because they are slow and because we will likely create instances of them soon
		case "apiextensions.k8s.io/CustomResourceDefinition":
			return -1000

		// We need to create ServiceAccounts, Roles before we bind them with a RoleBinding
		case "/ServiceAccount", "rbac.authorization.k8s.io/ClusterRole":
			return 1
		case "rbac.authorization.k8s.io/ClusterRoleBinding":
			return 2

		// Pods might need configmap or secrets - avoid backoff by creating them first
		case "/ConfigMap", "/Secrets":
			return 100

		// Create the pods after we've created other things they might be waiting for
		case "extensions/Deployment", "app/Deployment":
			return 1000

		// Autoscalers typically act on a deployment
		case "autoscaling/HorizontalPodAutoscaler":
			return 1001

		// Create services late - after pods have been started
		case "/Service":
			return 10000

		default:
			// TODO: Downgrade to V(2) once we're more comfortable with the ordering
			log.WithValues("group", o.Group).WithValues("kind", o.Kind).Info("unknown group / kind")
			return 1000
		}
	}
}
