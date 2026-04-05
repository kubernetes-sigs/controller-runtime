package priorityqueue

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"
	"k8s.io/utils/third_party/forked/golang/btree"

	"sigs.k8s.io/controller-runtime/pkg/internal/metrics"
)

// AddOpts describes the options for adding items to the queue.
type AddOpts struct {
	After       time.Duration
	RateLimited bool
	// Priority is the priority of the item. Higher values
	// indicate higher priority.
	// Defaults to zero if unset.
	Priority *int
}

// PriorityQueue is a priority queue for a controller. It
// internally de-duplicates all items that are added to
// it. It will use the max of the passed priorities and the
// min of possible durations.
//
// When an item that is already enqueued at a lower priority
// is re-enqueued with a higher priority, it will be placed at
// the end among items of the new priority, in order to
// preserve FIFO semantics within each priority level.
// The effective duration (i.e. the ready time) is still
// computed as the minimum across all enqueues.
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
	Log            logr.Logger
}

// Opt allows to configure a PriorityQueue.
type Opt[T comparable] func(*Opts[T])

type itemState int

const (
	stateNew             itemState = iota // item is new, it is not being processed, and it is not dirty
	stateProcessing                       // item is being processed, and it is not dirty
	stateProcessingDirty                  // item is being processed, and it is dirty, it will be requeued after processing is done
	stateDirty                            // item is not being processed, and it is dirty, it will be processed soon
)

type trackingItem[T comparable] struct {
	state itemState
	*item[T]
}

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

	freeList := btree.NewFreeList[*item[T]](btree.DefaultFreeListSize)
	pq := &priorityqueue[T]{
		log:                       opts.Log,
		addBuffer:                 make(map[T]*item[T]),
		addBufferOld:              make(map[T]*item[T]),
		items:                     map[T]trackingItem[T]{},
		ready:                     btree.NewWithFreeList(32, lessReady[T], freeList),
		waiting:                   btree.NewWithFreeList(32, lessWaiting[T], freeList),
		metrics:                   newQueueMetrics[T](opts.MetricProvider, name, clock.RealClock{}),
		waitingItemAddedOrUpdated: make(chan struct{}, 1),
		rateLimiter:               opts.RateLimiter,
		done:                      make(chan struct{}),

		now:  time.Now,
		tick: time.Tick,
	}
	pq.readyItemAdded = sync.NewCond(&pq.lock)

	pq.completed.Go(pq.handleWaitingItems)
	pq.completed.Go(pq.logState)
	if _, ok := pq.metrics.(noMetrics[T]); !ok {
		pq.completed.Go(pq.updateUnfinishedWorkLoop)
	}

	return pq
}

type priorityqueue[T comparable] struct {
	log logr.Logger

	addBufferLock sync.Mutex
	addBuffer     map[T]*item[T]
	addBufferOld  map[T]*item[T]

	// lock has to be acquired for any access to any of items, ready, waiting,
	// addedCounter or waiters.
	lock              sync.Mutex
	items             map[T]trackingItem[T]
	ready             bTree[*item[T]]
	waiting           bTree[*item[T]]
	nrProcessingDirty int

	// addedCounter is a counter of elements added, we need it
	// to provide FIFO semantics.
	addedCounter atomic.Uint64

	metrics queueMetrics[T]

	readyItemAdded            *sync.Cond
	waitingItemAddedOrUpdated chan struct{}

	rateLimiter workqueue.TypedRateLimiter[T]

	shutdown  atomic.Bool
	done      chan struct{}
	completed sync.WaitGroup

	// Configurable for testing
	now  func() time.Time
	tick func(time.Duration) <-chan time.Time
}

func (w *priorityqueue[T]) AddWithOpts(o AddOpts, items ...T) {
	if w.shutdown.Load() {
		return
	}

	if len(items) == 0 {
		return
	}

	w.addBufferLock.Lock()
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
			readyAt = new(w.now().Add(after))
			w.metrics.retry()
		}

		newItem := item[T]{
			Key:          key,
			AddedCounter: w.addedCounter.Add(1),
			Priority:     ptr.Deref(o.Priority, 0),
			ReadyAt:      readyAt,
		}
		existing, exists := w.addBuffer[key]
		if !exists {
			heapItem := new(item[T])
			*heapItem = newItem
			existing = heapItem
		} else {
			w.updateItem(existing, &newItem)
		}
		w.addBuffer[key] = existing

		if readyAt != nil {
			// Signal that a waiting item was added or updated, so the waiting item handler can re-evaluate when items become ready.
			select {
			case w.waitingItemAddedOrUpdated <- struct{}{}:
			default:
			}
		} else {
			w.readyItemAdded.Signal() // Signal that new intake items are available
		}
	}
	w.addBufferLock.Unlock()
}

type notifySetting int

const (
	notifyAll notifySetting = iota
	dontNotifyReady
	dontNotifyWaiting
)

func (w *priorityqueue[T]) lockedFlushAddBuffer(notify notifySetting) {
	w.addBufferLock.Lock()
	intakeItems := w.addBuffer
	w.addBuffer, w.addBufferOld = w.addBufferOld, w.addBuffer
	w.addBufferLock.Unlock()

	defer clear(intakeItems) // clear the intake items, making sure w.intakeBackup is empty for the next round

	for key, intake := range intakeItems {
		item := w.items[key]
		switch item.state {
		case stateNew:
			item.state = stateDirty
			item.item = intake
			w.items[key] = item
			if item.ReadyAt == nil {
				w.metrics.add(item.Key, item.Priority)
			}
			w.enqueue(item.item, notify)
		case stateProcessing:
			item.state = stateProcessingDirty
			item.item = intake // overwrite
			w.items[key] = item
			if item.ReadyAt == nil {
				w.metrics.add(item.Key, item.Priority)
				w.nrProcessingDirty++
			}
		case stateProcessingDirty:
			hasReadyAtBefore := item.ReadyAt != nil
			priorityBefore := item.Priority
			item.state = stateProcessingDirty
			w.updateItem(item.item, intake)
			w.items[key] = item
			if hasReadyAtBefore && item.ReadyAt == nil { // a non-ready item was replaced with a ready item, treat as add
				w.metrics.add(item.Key, item.Priority)
				w.nrProcessingDirty++
			} else if !hasReadyAtBefore && item.Priority != priorityBefore { // a ready item had its priority changed, update the priority metric
				w.metrics.updateDepthWithPriorityMetric(priorityBefore, item.Priority)
			}
		case stateDirty:
			hasReadyAtBefore := item.ReadyAt != nil
			priorityBefore := item.Priority
			w.dequeue(item.item)
			w.updateItem(item.item, intake)
			w.items[key] = item
			if hasReadyAtBefore && item.ReadyAt == nil { // a non-ready item was replaced with a ready item, treat as add
				w.metrics.add(item.Key, item.Priority)
			} else if !hasReadyAtBefore && item.Priority != priorityBefore { // a ready item had its priority changed, update the priority metric
				w.metrics.updateDepthWithPriorityMetric(priorityBefore, item.Priority)
			}
			w.enqueue(item.item, notify)
		}
	}
}

func (w *priorityqueue[T]) updateItem(existingItem *item[T], newItem *item[T]) {
	existingItem.Key = newItem.Key
	if newItem.Priority > existingItem.Priority {
		existingItem.Priority = newItem.Priority
		existingItem.AddedCounter = newItem.AddedCounter
	}
	if newItem.ReadyAt == nil || (existingItem.ReadyAt != nil && newItem.ReadyAt.Before(*existingItem.ReadyAt)) {
		existingItem.ReadyAt = newItem.ReadyAt
		existingItem.AddedCounter = newItem.AddedCounter
	}
}

func (w *priorityqueue[T]) dequeue(item *item[T]) {
	if item.ReadyAt != nil {
		w.waiting.Delete(item)
	} else {
		w.ready.Delete(item)
	}
}

func (w *priorityqueue[T]) enqueue(item *item[T], notify notifySetting) {
	if item.ReadyAt != nil {
		w.waiting.ReplaceOrInsert(item)
		if notify != dontNotifyWaiting {
			select {
			case w.waitingItemAddedOrUpdated <- struct{}{}:
			default:
			}
		}
	} else {
		w.ready.ReplaceOrInsert(item)
		if notify != dontNotifyReady {
			w.readyItemAdded.Signal()
		}
	}
}

func (w *priorityqueue[T]) handleWaitingItems() {
	blockForever := make(chan time.Time)
	var nextReady <-chan time.Time
	nextReady = blockForever

	for {
		select {
		case <-w.done:
			return
		case <-w.waitingItemAddedOrUpdated:
		case <-nextReady:
			nextReady = blockForever
		}

		func() {
			w.lock.Lock()
			defer w.lock.Unlock()
			w.lockedFlushAddBuffer(dontNotifyWaiting) // don't notify about waiting items, because we're going to re-evaluate them anyway

			var toMove []*item[T]
			w.waiting.Ascend(func(item *item[T]) bool {
				readyIn := item.ReadyAt.Sub(w.now()) // Store this to prevent TOCTOU issues
				if readyIn <= 0 {
					toMove = append(toMove, item)
					return true
				}

				nextReady = w.tick(readyIn)
				return false
			})

			// Don't manipulate the tree from within Ascend
			for _, toMove := range toMove {
				w.waiting.Delete(toMove)
				toMove.ReadyAt = nil

				// Bump added counter so items get sorted by when
				// they became ready, not when they were added.
				toMove.AddedCounter = w.addedCounter.Add(1)

				w.metrics.add(toMove.Key, toMove.Priority)
				w.ready.ReplaceOrInsert(toMove)
				w.readyItemAdded.Signal()
			}
		}()
	}
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
	w.lock.Lock()
	defer w.lock.Unlock()
	w.lockedFlushAddBuffer(dontNotifyReady) // don't notify about ready items, because we're going to check them anyway
	for w.ready.Len() == 0 && !w.shutdown.Load() {
		w.readyItemAdded.Wait()
		w.lockedFlushAddBuffer(dontNotifyReady) // don't notify about ready items, because we're going to check them anyway
	}
	if w.shutdown.Load() {
		return *new(T), 0, true
	}

	queueItem, _ := w.ready.DeleteMin()
	item := w.items[queueItem.Key]
	item.state = stateProcessing
	item.item = nil
	w.items[queueItem.Key] = item
	key, prio := queueItem.Key, queueItem.Priority
	w.metrics.get(key, prio)
	return key, prio, false
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

func (w *priorityqueue[T]) Done(key T) {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.lockedFlushAddBuffer(notifyAll)

	w.metrics.done(key)
	item := w.items[key]
	switch item.state {
	case stateProcessing:
		delete(w.items, key)
	case stateProcessingDirty:
		// if the item needs processing, we update the item and requeue it, so it will be processed again
		item.state = stateDirty
		w.items[key] = item
		if item.ReadyAt == nil {
			w.nrProcessingDirty-- // the item is now in the queue
		}
		w.enqueue(item.item, notifyAll)
	case stateNew, stateDirty:
		panic("Done called for an item that is not being processed")
	}
}

func (w *priorityqueue[T]) ShutDown() {
	w.shutdown.Store(true)
	close(w.done)
	w.readyItemAdded.Broadcast()
	w.completed.Wait()
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

	// Flush is performed before reading items to avoid errors caused by asynchronous behavior,
	// primarily for unit testing purposes.
	w.lockedFlushAddBuffer(notifyAll)

	return w.ready.Len() + w.nrProcessingDirty
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
		w.waiting.Ascend(func(item *item[T]) bool {
			items = append(items, item)
			return true
		})
		w.ready.Ascend(func(item *item[T]) bool {
			items = append(items, item)
			return true
		})
		w.lock.Unlock()

		w.log.V(5).Info("workqueue_items", "items", items)
	}
}

func lessWaiting[T comparable](a, b *item[T]) bool {
	if !a.ReadyAt.Equal(*b.ReadyAt) {
		return a.ReadyAt.Before(*b.ReadyAt)
	}
	return lessReady(a, b)
}

func lessReady[T comparable](a, b *item[T]) bool {
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
	ReplaceOrInsert(item T) (T, bool)
	DeleteMin() (T, bool)
	Delete(item T) (T, bool)
	Ascend(iterator btree.ItemIterator[T])
	Len() int
}
