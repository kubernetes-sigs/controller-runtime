package cluster

import "context"

// contextKey is how we find Loggers in a context.Context.
type contextKey struct{}

// FromContext returns a Logger from ctx or an error if no Logger is found.
func FromContext(ctx context.Context) string {
	if v, ok := ctx.Value(contextKey{}).(string); ok {
		return v
	}
	return ""
}

// IntoContext returns a new Context, derived from ctx, which carries the
// provided cluster identifier.
func IntoContext(ctx context.Context, cluster Cluster) context.Context {
	return context.WithValue(ctx, contextKey{}, cluster.ID())
}
