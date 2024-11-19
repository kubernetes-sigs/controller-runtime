package controllerworkqueue

import (
	"sync"
	"sync/atomic"
	"time"

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

	cwq := &controllerworkqueue[T]{
		items:       map[T]*item[T]{},
		queue:       btree.NewG(32, less[T]),
		tryPush:     make(chan struct{}, 1),
		rateLimiter: opts.RateLimiter,
		locked:      sets.Set[T]{},
		done:        make(chan struct{}),
		get:         make(chan item[T]),
		now:         time.Now,
		tick:        time.Tick,
	}

	go cwq.spin()

	return wrapWithMetrics(cwq, name, opts.MetricProvider)
}

type controllerworkqueue[T comparable] struct {
	// lock has to be acquired for any access to either items or queue
	lock  sync.Mutex
	items map[T]*item[T]
	queue *btree.BTreeG[*item[T]]

	tryPush chan struct{}

	rateLimiter workqueue.TypedRateLimiter[T]

	// addedCounter is a counter of elements added, we need it
	// because unixNano is not guaranteed to be unique.
	addedCounter uint64

	// locked contains the keys we handed out through Get() and that haven't
	// yet been returned through Done().
	locked     sets.Set[T]
	lockedLock sync.RWMutex

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

func (w *controllerworkqueue[T]) AddWithOpts(o AddOpts, items ...T) {
	w.lock.Lock()
	defer w.lock.Unlock()

	for _, key := range items {
		if o.RateLimited {
			after := w.rateLimiter.When(key)
			if o.After == 0 || after < o.After {
				o.After = after
			}
		}

		var readyAt *time.Time
		if o.After != 0 {
			readyAt = ptr.To(w.now().Add(o.After))
		}
		if _, ok := w.items[key]; !ok {
			item := &item[T]{
				key:             key,
				addedAtUnixNano: w.now().UnixNano(),
				addedCounter:    w.addedCounter,
				priority:        o.Priority,
				readyAt:         readyAt,
			}
			w.items[key] = item
			w.queue.ReplaceOrInsert(item)
			w.addedCounter++
			continue
		}

		// The b-tree de-duplicates based on ordering and any change here
		// will affect the order - Just delete and re-add.
		item, _ := w.queue.Delete(w.items[key])
		if o.Priority > item.priority {
			item.priority = o.Priority
		}

		if item.readyAt != nil && (readyAt == nil || readyAt.Before(*item.readyAt)) {
			item.readyAt = readyAt
		}

		w.queue.ReplaceOrInsert(item)
	}
}

func (w *controllerworkqueue[T]) doTryPush() {
	select {
	case w.tryPush <- struct{}{}:
	default:
	}
}

func (w *controllerworkqueue[T]) spin() {
	blockForever := make(chan time.Time)
	var nextReady <-chan time.Time
	nextReady = blockForever
	for {
		select {
		case <-w.done:
			return
		case <-w.tryPush:
		case <-nextReady:
		}

		nextReady = blockForever

		func() {
			w.lock.Lock()
			defer w.lock.Unlock()

			w.lockedLock.Lock()
			defer w.lockedLock.Unlock()

			w.queue.Ascend(func(item *item[T]) bool {
				if w.waiters.Load() == 0 { // no waiters, return as we can not hand anything out anyways
					return false
				}

				// No next element we can process
				if item.readyAt != nil && item.readyAt.After(w.now()) {
					nextReady = w.tick(item.readyAt.Sub(w.now()))
					return false
				}

				// Item is locked, we can not hand it out
				if w.locked.Has(item.key) {
					return true
				}

				w.get <- *item
				w.locked.Insert(item.key)
				w.waiters.Add(-1)
				delete(w.items, item.key)
				w.queue.Delete(item)

				return true
			})
		}()
	}
}

func (w *controllerworkqueue[T]) Add(item T) {
	w.AddWithOpts(AddOpts{}, item)
}

func (w *controllerworkqueue[T]) AddAfter(item T, after time.Duration) {
	w.AddWithOpts(AddOpts{After: after}, item)
}

func (w *controllerworkqueue[T]) AddRateLimited(item T) {
	w.AddWithOpts(AddOpts{RateLimited: true}, item)
}

func (w *controllerworkqueue[T]) GetWithPriority() (_ T, priority int, shutdown bool) {
	w.waiters.Add(1)

	w.doTryPush()
	item := <-w.get

	return item.key, item.priority, w.shutdown.Load()
}

func (w *controllerworkqueue[T]) Get() (item T, shutdown bool) {
	key, _, shutdown := w.GetWithPriority()
	return key, shutdown
}

func (w *controllerworkqueue[T]) Forget(item T) {
	w.rateLimiter.Forget(item)
}

func (w *controllerworkqueue[T]) NumRequeues(item T) int {
	return w.rateLimiter.NumRequeues(item)
}

func (w *controllerworkqueue[T]) ShuttingDown() bool {
	return w.shutdown.Load()
}

func (w *controllerworkqueue[T]) Done(item T) {
	w.lockedLock.Lock()
	defer w.lockedLock.Unlock()
	w.locked.Delete(item)
	w.doTryPush()
}

func (w *controllerworkqueue[T]) ShutDown() {
	w.shutdown.Store(true)
	close(w.done)
}

func (w *controllerworkqueue[T]) ShutDownWithDrain() {
	w.ShutDown()
}

func (w *controllerworkqueue[T]) Len() int {
	w.lock.Lock()
	defer w.lock.Unlock()

	return w.queue.Len()
}

func less[T comparable](a, b *item[T]) bool {
	if a.readyAt == nil && b.readyAt != nil {
		return true
	}
	if a.readyAt != nil && b.readyAt == nil {
		return false
	}
	if a.readyAt != nil && b.readyAt != nil && !a.readyAt.Equal(*b.readyAt) {
		return a.readyAt.Before(*b.readyAt)
	}
	if a.priority != b.priority {
		return a.priority > b.priority
	}

	if a.addedAtUnixNano != b.addedAtUnixNano {
		return a.addedAtUnixNano < b.addedAtUnixNano
	}

	return a.addedCounter < b.addedCounter
}

type item[T comparable] struct {
	key             T
	addedAtUnixNano int64
	addedCounter    uint64
	priority        int
	readyAt         *time.Time
}

func wrapWithMetrics[T comparable](q *controllerworkqueue[T], name string, provider workqueue.MetricsProvider) PriorityQueue[T] {
	mwq := &metricWrappedQueue[T]{
		controllerworkqueue: q,
		metrics:             newQueueMetrics[T](provider, name, clock.RealClock{}),
	}

	go mwq.updateUnfinishedWorkLoop()

	return mwq
}

type metricWrappedQueue[T comparable] struct {
	*controllerworkqueue[T]
	metrics queueMetrics[T]
}

func (m *metricWrappedQueue[T]) AddWithOpts(o AddOpts, items ...T) {
	for _, item := range items {
		m.metrics.add(item)
	}
	m.controllerworkqueue.AddWithOpts(o, items...)
}

func (m *metricWrappedQueue[T]) GetWithPriority() (T, int, bool) {
	item, priority, shutdown := m.controllerworkqueue.GetWithPriority()
	m.metrics.get(item)
	return item, priority, shutdown
}

func (m *metricWrappedQueue[T]) Get() (T, bool) {
	item, _, shutdown := m.GetWithPriority()
	return item, shutdown
}

func (m *metricWrappedQueue[T]) Done(item T) {
	m.metrics.done(item)
	m.controllerworkqueue.Done(item)
}

func (m *metricWrappedQueue[T]) updateUnfinishedWorkLoop() {
	t := time.NewTicker(time.Millisecond)
	defer t.Stop()
	for range t.C {
		if m.controllerworkqueue.ShuttingDown() {
			return
		}
		m.metrics.updateUnfinishedWork()
	}
}
