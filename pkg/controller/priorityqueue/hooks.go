package priorityqueue

// Hooks represents a set of hooks that can be implemented to
// customize the behavior of the priority queue for elements
// of type T.
//
// NOTE: LOW LEVEL PRIMITIVE!
// Implementations must be goroutine-safe and considerate
// of the time spent in each hook, as they may be called
// in performance-sensitive paths. It's recommended to
// use non-blocking operations or offload heavy processing
// to separate goroutines through the use of channels or
// context-aware mechanisms.
type Hooks[T comparable] interface {
	// OnBecameReady is called when an item becomes ready to be processed.
	// For AddWithOpts() calls that result in the item being added with
	// a delay, this hook is called only when the item becomes ready
	// after the delay has elapsed.
	OnBecameReady(item T, priority int)
}

// hooks is a wrapper around Hooks to allow optional implementation.
type hooks[T comparable] struct {
	Hooks[T]
}

func (h hooks[T]) OnBecameReady(item T, priority int) {
	if h.Hooks != nil {
		h.Hooks.OnBecameReady(item, priority)
	}
}
