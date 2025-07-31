package priorityqueue

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/btree"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/internal/metrics"
)

// AddOpts describes the options for adding items to the queue.
type AddOpts struct {
	After       time.Duration
	RateLimited bool
	Priority    int
}

// PriorityQueue is a priority queue for a controller. It
// internally de-duplicates all items that are added to
// it. It will use the max of the passed priorities and the
// min of possible durations.
type PriorityQueue[T comparable] interface {
	workqueue.TypedRateLimitingInterface[T]
	AddWithOpts(o AddOpts, Items ...T)
	GetWithPriority() (item T, priority int, shutdown bool)
}

// Opts contains the options for a PriorityQueue.
type Opts[T comparable] struct {
	// Ratelimiter is being used when AddRateLimited is called. Defaults to a per-item exponential backoff
	// limiter with an initial delay of five milliseconds and a max delay of 1000 seconds.
	RateLimiter    workqueue.TypedRateLimiter[T]
	MetricProvider workqueue.MetricsProvider
	// FairnessDimensionsExtractor can be configured to make the PriorityQueue return items
	// with the same priority fairly based on the returned dimensions. This could for
	// example be the items Namespace. Doing this ensures that one Namespace with a lot
	// of events can not starve other Namespaces.
	//
	// If more than one dimension is returned, fairness is first ensured within the first
	// dimension, then within the second dimension and so on.
	//
	// If you want the opposite, i.E. explicitly priorize specific events, call AddWithOpts with a
	// higher priority.
	FairnessDimensionsExtractor func(T) []string
	Log                         logr.Logger
}

// Opt allows to configure a PriorityQueue.
type Opt[T comparable] func(*Opts[T])

// New constructs a new PriorityQueue.
func New[T comparable](name string, o ...Opt[T]) PriorityQueue[T] {
	opts := &Opts[T]{}
	for _, f := range o {
		f(opts)
	}

	if opts.RateLimiter == nil {
		opts.RateLimiter = workqueue.NewTypedItemExponentialFailureRateLimiter[T](5*time.Millisecond, 1000*time.Second)
	}

	if opts.MetricProvider == nil {
		opts.MetricProvider = metrics.WorkqueueMetricsProvider{}
	}

	if opts.FairnessDimensionsExtractor == nil {
		opts.FairnessDimensionsExtractor = func(T) []string { return nil }
	}

	pq := &priorityqueue[T]{
		log:                       opts.Log,
		items:                     map[T]*item[T]{},
		queue:                     btree.NewG(32, less[T]),
		fairQueue:                 &fairq[T]{},
		extractFairnessDimensions: opts.FairnessDimensionsExtractor,
		becameReady:               sets.Set[T]{},
		metrics:                   newQueueMetrics[T](opts.MetricProvider, name, clock.RealClock{}),
		// itemOrWaiterAdded indicates that an item or
		// waiter was added. It must be buffered, because
		// if we currently process items we can't tell
		// if that included the new item/waiter.
		itemOrWaiterAdded: make(chan struct{}, 1),
		rateLimiter:       opts.RateLimiter,
		locked:            sets.Set[T]{},
		done:              make(chan struct{}),
		get:               make(chan item[T]),
		now:               time.Now,
		tick:              time.Tick,
	}

	go pq.spin()
	go pq.logState()
	if _, ok := pq.metrics.(noMetrics[T]); !ok {
		go pq.updateUnfinishedWorkLoop()
	}

	return pq
}

type priorityqueue[T comparable] struct {
	log logr.Logger
	// lock has to be acquired before accessing any of items,
	// queue, fairQueue, fairQueue, addedCounter, becameReady or
	// locked.
	lock  sync.Mutex
	items map[T]*item[T]
	queue bTree[*item[T]]

	// fairQueue ensures that the items we hand out are far across
	// configurable dimensions, for example the namespace.
	// It holds all items that are ready to be handed out and in the
	// highest priority bracket that has ready items, as well as state
	// to determine the fairness.
	fairQueue fairQueueInterface[T]

	// fairQueuePriority is the priority of the items in the fairQueue.
	// it may be nil, indicating the fairQueuePriority is currently
	// unknown.
	fairQueuePriority *int

	extractFairnessDimensions func(item T) []string

	// addedCounter is a counter of elements added, we need it
	// because unixNano is not guaranteed to be unique.
	addedCounter uint64

	// becameReady holds items that are in the queue, were added
	// with non-zero after and became ready. We need it to call
	// metrics.add and fairQueue.add exactly once for them.
	becameReady sets.Set[T]
	metrics     queueMetrics[T]

	itemOrWaiterAdded chan struct{}

	rateLimiter workqueue.TypedRateLimiter[T]

	// locked contains the keys we handed out through Get() and that haven't
	// yet been returned through Done().
	locked sets.Set[T]

	shutdown atomic.Bool
	done     chan struct{}

	get chan item[T]

	// waiters is the number of routines blocked in Get, we use it to determine
	// if we can push items.
	waiters atomic.Int64

	// Configurable for testing
	now  func() time.Time
	tick func(time.Duration) <-chan time.Time
}

func (w *priorityqueue[T]) AddWithOpts(o AddOpts, items ...T) {
	if w.shutdown.Load() {
		return
	}

	w.lock.Lock()
	defer w.lock.Unlock()

	for _, key := range items {
		after := o.After
		if o.RateLimited {
			rlAfter := w.rateLimiter.When(key)
			if after == 0 || rlAfter < after {
				after = rlAfter
			}
		}

		var readyAt *time.Time
		if after > 0 {
			readyAt = ptr.To(w.now().Add(after))
			w.metrics.retry()
		}

		if readyAt == nil && !w.locked.Has(key) && w.fairQueuePriority != nil && *w.fairQueuePriority < o.Priority {
			w.fairQueue.drain()
			w.fairQueuePriority = nil // Leave nil here in case something at that priority becomes ready in parallel
		}

		if _, ok := w.items[key]; !ok {
			item := &item[T]{
				Key:          key,
				AddedCounter: w.addedCounter,
				Priority:     o.Priority,
				ReadyAt:      readyAt,
			}
			w.items[key] = item
			w.queue.ReplaceOrInsert(item)
			if item.ReadyAt == nil {
				w.metrics.add(key, item.Priority)
				w.maybeAddToFairQueueLocked(item)
			}
			w.addedCounter++
			continue
		}

		// The b-tree de-duplicates based on ordering and any change here
		// will affect the order - Just delete and re-add.
		item, _ := w.queue.Delete(w.items[key])
		if o.Priority > item.Priority {
			// Update depth metric only if the item in the queue was already added to the depth metric.
			if item.ReadyAt == nil || w.becameReady.Has(key) {
				w.metrics.updateDepthWithPriorityMetric(item.Priority, o.Priority)
			}
			item.Priority = o.Priority

			w.maybeAddToFairQueueLocked(item)
		}

		if item.ReadyAt != nil && (readyAt == nil || readyAt.Before(*item.ReadyAt)) {
			if readyAt == nil && !w.becameReady.Has(key) {
				w.metrics.add(key, item.Priority)
				w.maybeAddToFairQueueLocked(item)
			}
			item.ReadyAt = readyAt
		}

		w.queue.ReplaceOrInsert(item)
	}

	if len(items) > 0 {
		w.notifyItemOrWaiterAdded()
	}
}

func (w *priorityqueue[T]) notifyItemOrWaiterAdded() {
	select {
	case w.itemOrWaiterAdded <- struct{}{}:
	default:
	}
}

func (w *priorityqueue[T]) spin() {
	blockForever := make(chan time.Time)
	var nextReady <-chan time.Time
	nextReady = blockForever

	for {
		select {
		case <-w.done:
			return
		case <-w.itemOrWaiterAdded:
		case <-nextReady:
		}

		nextReady = blockForever

		func() {
			w.lock.Lock()
			defer w.lock.Unlock()

		fairQueuePopulating:
			fairQueueNeedsPopulating := w.fairQueuePriority == nil
			w.queue.Ascend(func(item *item[T]) bool {
				if item.ReadyAt != nil {
					if readyAt := item.ReadyAt.Sub(w.now()); readyAt > 0 {
						nextReady = w.tick(readyAt)
						return false
					}
					if !w.becameReady.Has(item.Key) {
						w.metrics.add(item.Key, item.Priority)
						w.becameReady.Insert(item.Key)
						if !fairQueueNeedsPopulating {
							w.maybeAddToFairQueueLocked(item)
						}
					}
				}

				// Item is locked, we can not hand it out
				if w.locked.Has(item.Key) {
					return true
				}

				if w.fairQueuePriority == nil {
					w.fairQueuePriority = ptr.To(item.Priority)
				}

				if fairQueueNeedsPopulating {
					w.maybeAddToFairQueueLocked(item)
				}

				return true
			})

			if w.fairQueue.isEmpty() {
				return
			}

			for w.waiters.Load() > 0 {
				item, hasItem := w.fairQueue.dequeue()
				if !hasItem {
					w.fairQueuePriority = nil
					if w.waiters.Load() > 0 {
						// There could be ready items that have a lower priority
						goto fairQueuePopulating
					}
					break
				}

				w.metrics.get(item.Key, item.Priority)
				w.locked.Insert(item.Key)
				w.waiters.Add(-1)
				delete(w.items, item.Key)
				w.becameReady.Delete(item.Key)
				w.get <- *item
				w.queue.Delete(item)

				if w.fairQueue.isEmpty() {
					w.fairQueuePriority = nil
				}
			}
		}()
	}
}

func (w *priorityqueue[T]) maybeAddToFairQueueLocked(item *item[T]) {
	if w.fairQueuePriority == nil ||
		*w.fairQueuePriority != item.Priority ||
		(item.ReadyAt != nil && item.ReadyAt.Sub(w.now()) > 0) ||
		w.locked.Has(item.Key) {
		return
	}

	w.fairQueue.add(item, w.extractFairnessDimensions(item.Key)...)
}

func (w *priorityqueue[T]) Add(item T) {
	w.AddWithOpts(AddOpts{}, item)
}

func (w *priorityqueue[T]) AddAfter(item T, after time.Duration) {
	w.AddWithOpts(AddOpts{After: after}, item)
}

func (w *priorityqueue[T]) AddRateLimited(item T) {
	w.AddWithOpts(AddOpts{RateLimited: true}, item)
}

func (w *priorityqueue[T]) GetWithPriority() (_ T, priority int, shutdown bool) {
	if w.shutdown.Load() {
		var zero T
		return zero, 0, true
	}

	w.waiters.Add(1)

	w.notifyItemOrWaiterAdded()

	item := <-w.get

	return item.Key, item.Priority, w.shutdown.Load()
}

func (w *priorityqueue[T]) Get() (item T, shutdown bool) {
	key, _, shutdown := w.GetWithPriority()
	return key, shutdown
}

func (w *priorityqueue[T]) Forget(item T) {
	w.rateLimiter.Forget(item)
}

func (w *priorityqueue[T]) NumRequeues(item T) int {
	return w.rateLimiter.NumRequeues(item)
}

func (w *priorityqueue[T]) ShuttingDown() bool {
	return w.shutdown.Load()
}

func (w *priorityqueue[T]) Done(item T) {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.locked.Delete(item)
	w.metrics.done(item)

	// Update the fairqueue if the item is waiting
	if w.fairQueuePriority != nil && w.items[item] != nil && (w.items[item].ReadyAt == nil || w.items[item].ReadyAt.Sub(w.now()) <= 0) {
		if *w.fairQueuePriority == w.items[item].Priority {
			// We can just insert as the fairQueue sorts by addedCounter
			w.fairQueue.add(w.items[item], w.extractFairnessDimensions(item)...)
		} else if *w.fairQueuePriority < w.items[item].Priority {
			w.fairQueue.drain()
			w.fairQueuePriority = ptr.To(w.items[item].Priority)
			w.fairQueue.add(w.items[item], w.extractFairnessDimensions(item)...)
		}
	}
	w.notifyItemOrWaiterAdded()
}

func (w *priorityqueue[T]) ShutDown() {
	w.shutdown.Store(true)
	close(w.done)
}

// ShutDownWithDrain just calls ShutDown, as the draining
// functionality is not used by controller-runtime.
func (w *priorityqueue[T]) ShutDownWithDrain() {
	w.ShutDown()
}

// Len returns the number of items that are ready to be
// picked up. It does not include items that are not yet
// ready.
func (w *priorityqueue[T]) Len() int {
	w.lock.Lock()
	defer w.lock.Unlock()

	var result int
	w.queue.Ascend(func(item *item[T]) bool {
		if item.ReadyAt == nil || item.ReadyAt.Compare(w.now()) <= 0 {
			result++
			return true
		}
		return false
	})

	return result
}

func (w *priorityqueue[T]) logState() {
	t := time.Tick(10 * time.Second)
	for {
		select {
		case <-w.done:
			return
		case <-t:
		}

		// Log level may change at runtime, so keep the
		// loop going even if a given level is currently
		// not enabled.
		if !w.log.V(5).Enabled() {
			continue
		}
		w.lock.Lock()
		items := make([]*item[T], 0, len(w.items))
		w.queue.Ascend(func(item *item[T]) bool {
			items = append(items, item)
			return true
		})
		w.lock.Unlock()

		w.log.V(5).Info("workqueue_items", "items", items)
	}
}

func less[T comparable](a, b *item[T]) bool {
	if a.ReadyAt == nil && b.ReadyAt != nil {
		return true
	}
	if b.ReadyAt == nil && a.ReadyAt != nil {
		return false
	}
	if a.ReadyAt != nil && b.ReadyAt != nil && !a.ReadyAt.Equal(*b.ReadyAt) {
		return a.ReadyAt.Before(*b.ReadyAt)
	}
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}

	return a.AddedCounter < b.AddedCounter
}

type item[T comparable] struct {
	Key          T          `json:"key"`
	AddedCounter uint64     `json:"addedCounter"`
	Priority     int        `json:"priority"`
	ReadyAt      *time.Time `json:"readyAt,omitempty"`
}

func (w *priorityqueue[T]) updateUnfinishedWorkLoop() {
	t := time.Tick(500 * time.Millisecond) // borrowed from workqueue: https://github.com/kubernetes/kubernetes/blob/67a807bf142c7a2a5ecfdb2a5d24b4cdea4cc79c/staging/src/k8s.io/client-go/util/workqueue/queue.go#L182
	for {
		select {
		case <-w.done:
			return
		case <-t:
		}
		w.metrics.updateUnfinishedWork()
	}
}

type bTree[T any] interface {
	ReplaceOrInsert(item T) (_ T, _ bool)
	Delete(item T) (T, bool)
	Ascend(iterator btree.ItemIteratorG[T])
}

type fairQueueInterface[T comparable] interface {
	add(i *item[T], dimensions ...string)
	dequeue() (*item[T], bool)
	drain()
	isEmpty() bool
}
