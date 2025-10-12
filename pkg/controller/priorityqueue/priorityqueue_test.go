package priorityqueue

import (
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	fuzz "github.com/google/gofuzz"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
)

var _ = Describe("Controllerworkqueue", func() {
	It("returns an item", func() {
		q, metrics := newQueue()
		defer q.ShutDown()
		q.AddWithOpts(AddOpts{}, "foo")

		item, _, _ := q.GetWithPriority()
		Expect(item).To(Equal("foo"))

		Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 0}))
		Expect(metrics.adds["test"]).To(Equal(1))
		Expect(metrics.retries["test"]).To(Equal(0))
	})

	It("returns items in order", func() {
		q, metrics := newQueue()
		defer q.ShutDown()

		q.AddWithOpts(AddOpts{}, "foo")
		q.AddWithOpts(AddOpts{}, "bar")

		item, _, _ := q.GetWithPriority()
		Expect(item).To(Equal("foo"))
		item, _, _ = q.GetWithPriority()
		Expect(item).To(Equal("bar"))

		Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 0}))
		Expect(metrics.adds["test"]).To(Equal(2))
	})

	It("doesn't return an item that is currently locked", func() {
		q, metrics := newQueue()
		defer q.ShutDown()

		q.AddWithOpts(AddOpts{}, "foo")

		item, _, _ := q.GetWithPriority()
		Expect(item).To(Equal("foo"))

		q.AddWithOpts(AddOpts{}, "foo")
		q.AddWithOpts(AddOpts{}, "bar")
		item, _, _ = q.GetWithPriority()
		Expect(item).To(Equal("bar"))

		Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 1}))
		Expect(metrics.adds["test"]).To(Equal(3))
	})

	It("returns an item as soon as its unlocked", func() {
		q, metrics := newQueue()
		defer q.ShutDown()

		q.AddWithOpts(AddOpts{}, "foo")

		item, _, _ := q.GetWithPriority()
		Expect(item).To(Equal("foo"))

		q.AddWithOpts(AddOpts{}, "foo")
		q.AddWithOpts(AddOpts{}, "bar")
		item, _, _ = q.GetWithPriority()
		Expect(item).To(Equal("bar"))

		q.AddWithOpts(AddOpts{}, "baz")
		q.Done("foo")
		item, _, _ = q.GetWithPriority()
		Expect(item).To(Equal("foo"))

		Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 1}))
		Expect(metrics.adds["test"]).To(Equal(4))
	})

	It("de-duplicates items", func() {
		q, metrics := newQueue()
		defer q.ShutDown()

		q.AddWithOpts(AddOpts{}, "foo")
		q.AddWithOpts(AddOpts{}, "foo")

		Expect(q.Len()).To(Equal(1))

		q.lockedLock.Lock()
		Expect(q.locked.Len()).To(Equal(0))

		Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 1}))
		Expect(metrics.adds["test"]).To(Equal(1))
	})

	It("retains the highest priority", func() {
		q, metrics := newQueue()
		defer q.ShutDown()

		q.AddWithOpts(AddOpts{Priority: ptr.To(1)}, "foo")
		q.AddWithOpts(AddOpts{Priority: ptr.To(2)}, "foo")

		item, priority, _ := q.GetWithPriority()
		Expect(item).To(Equal("foo"))
		Expect(priority).To(Equal(2))

		Expect(q.Len()).To(Equal(0))

		Expect(metrics.depth["test"]).To(Equal(map[int]int{1: 0, 2: 0}))
		Expect(metrics.adds["test"]).To(Equal(1))
	})

	It("gets pushed to the front if the priority increases", func() {
		q, metrics := newQueue()
		defer q.ShutDown()

		q.AddWithOpts(AddOpts{}, "foo")
		q.AddWithOpts(AddOpts{}, "bar")
		q.AddWithOpts(AddOpts{}, "baz")
		q.AddWithOpts(AddOpts{Priority: ptr.To(1)}, "baz")

		item, priority, _ := q.GetWithPriority()
		Expect(item).To(Equal("baz"))
		Expect(priority).To(Equal(1))

		Expect(q.Len()).To(Equal(2))

		Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 2, 1: 0}))
		Expect(metrics.adds["test"]).To(Equal(3))
	})

	It("retains the lowest after duration", func() {
		q, metrics := newQueue()
		defer q.ShutDown()

		q.AddWithOpts(AddOpts{After: 0}, "foo")
		q.AddWithOpts(AddOpts{After: time.Hour}, "foo")

		item, priority, _ := q.GetWithPriority()
		Expect(item).To(Equal("foo"))
		Expect(priority).To(Equal(0))

		Expect(q.Len()).To(Equal(0))
		Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 0}))
		Expect(metrics.adds["test"]).To(Equal(1))
	})

	It("doesn't include non-ready items in Len()", func() {
		q, metrics := newQueue()
		defer q.ShutDown()

		q.AddWithOpts(AddOpts{After: time.Minute}, "foo")
		q.AddWithOpts(AddOpts{}, "baz")
		q.AddWithOpts(AddOpts{After: time.Minute}, "bar")
		q.AddWithOpts(AddOpts{}, "bal")

		Expect(q.Len()).To(Equal(2))
		Expect(metrics.depth).To(HaveLen(1))
		Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 2}))
	})

	// ref: https://github.com/kubernetes-sigs/controller-runtime/issues/3239
	It("Get from priority queue might get stuck when the priority queue is shut down", func() {
		q, _ := newQueue()

		q.Add("baz")
		// shut down
		q.ShutDown()
		q.AddWithOpts(AddOpts{After: time.Second}, "foo")

		item, priority, isShutDown := q.GetWithPriority()
		Expect(item).To(Equal(""))
		Expect(priority).To(Equal(0))
		Expect(isShutDown).To(BeTrue())

		item1, priority1, isShutDown := q.GetWithPriority()
		Expect(item1).To(Equal(""))
		Expect(priority1).To(Equal(0))
		Expect(isShutDown).To(BeTrue())
	})

	It("returns many items", func() {
		// This test ensures the queue is able to drain a large queue without panic'ing.
		// In a previous version of the code we were calling queue.Delete within q.Ascend
		// which led to a panic in queue.Ascend > iterate:
		// "panic: runtime error: index out of range [0] with length 0"
		q, _ := newQueue()
		defer q.ShutDown()

		for range 20 {
			for i := range 1000 {
				rn := rand.N(100)
				if rn < 10 {
					q.AddWithOpts(AddOpts{After: time.Duration(rn) * time.Millisecond}, fmt.Sprintf("foo%d", i))
				} else {
					q.AddWithOpts(AddOpts{Priority: &rn}, fmt.Sprintf("foo%d", i))
				}
			}

			wg := sync.WaitGroup{}
			for range 100 { // The panic only occurred relatively frequently with a high number of go routines.
				wg.Add(1)
				go func() {
					defer wg.Done()
					for range 10 {
						obj, _, _ := q.GetWithPriority()
						q.Done(obj)
					}
				}()
			}

			wg.Wait()
		}
	})

	It("updates metrics correctly for an item that gets initially added with after and then without", func() {
		q, metrics := newQueue()
		defer q.ShutDown()

		q.AddWithOpts(AddOpts{After: time.Hour}, "foo")
		Expect(q.Len()).To(Equal(0))
		metrics.mu.Lock()
		Expect(metrics.depth["test"]).To(Equal(map[int]int{}))
		metrics.mu.Unlock()

		q.AddWithOpts(AddOpts{}, "foo")

		Expect(q.Len()).To(Equal(1))
		metrics.mu.Lock()
		Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 1}))
		metrics.mu.Unlock()

		// Get the item to ensure the codepath in
		// `spin` for the metrics is passed by so
		// that this starts failing if it incorrectly
		// calls `metrics.add` again.
		item, _ := q.Get()
		Expect(item).To(Equal("foo"))
		Expect(q.Len()).To(Equal(0))
		metrics.mu.Lock()
		Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 0}))
		metrics.mu.Unlock()
	})
})

func BenchmarkAddGetDone(b *testing.B) {
	q := New[int]("")
	defer q.ShutDown()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for i := 0; i < 1000; i++ {
			q.Add(i)
		}
		for range 1000 {
			item, _ := q.Get()
			q.Done(item)
		}
	}
}

func BenchmarkAddOnly(b *testing.B) {
	q := New[int]("")
	defer q.ShutDown()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for i := 0; i < 1000; i++ {
			q.Add(i)
		}
	}
}

func BenchmarkAddLockContended(b *testing.B) {
	q := New[int]("")
	defer q.ShutDown()
	go func() {
		for range 1000 {
			item, _ := q.Get()
			q.Done(item)
		}
	}()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for i := 0; i < 1000; i++ {
			q.Add(i)
		}
	}
}

// TestFuzzPrioriorityQueue validates a set of basic
// invariants that should always be true:
//
//   - The queue is threadsafe when multiple producers and consumers
//     are involved
//   - There are no deadlocks
//   - An item is never handed out again before it is returned
//   - Items in the queue are de-duplicated
//   - max(existing priority, new priority) is used
func TestFuzzPriorityQueue(t *testing.T) {
	t.Parallel()

	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	f := fuzz.NewWithSeed(seed)
	fuzzLock := sync.Mutex{}
	fuzz := func(in any) {
		fuzzLock.Lock()
		defer fuzzLock.Unlock()

		f.Fuzz(in)
	}

	inQueue := map[string]int{}
	inQueueLock := sync.Mutex{}

	handedOut := sets.Set[string]{}
	handedOutLock := sync.Mutex{}

	wg := sync.WaitGroup{}
	q, metrics := newQueue()

	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for range 1000 {
				opts, item := AddOpts{}, ""

				fuzz(&opts)
				fuzz(&item)

				if opts.After > 100*time.Millisecond {
					opts.After = 10 * time.Millisecond
				}
				opts.RateLimited = false

				func() {
					inQueueLock.Lock()
					defer inQueueLock.Unlock()

					q.AddWithOpts(opts, item)
					if existingPriority, exists := inQueue[item]; !exists || existingPriority < ptr.Deref(opts.Priority, 0) {
						inQueue[item] = ptr.Deref(opts.Priority, 0)
					}
				}()
			}
		}()
	}

	for range 100 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for {
				item, cont := func() (string, bool) {
					inQueueLock.Lock()
					defer inQueueLock.Unlock()

					if len(inQueue) == 0 {
						return "", false
					}

					item, priority, _ := q.GetWithPriority()
					if expected := inQueue[item]; expected != priority {
						t.Errorf("got priority %d, expected %d", priority, expected)
					}
					delete(inQueue, item)
					return item, true
				}()

				if !cont {
					return
				}

				func() {
					handedOutLock.Lock()
					defer handedOutLock.Unlock()

					if handedOut.Has(item) {
						t.Errorf("item %s got handed out more than once", item)
					}

					metrics.mu.Lock()
					for priority, depth := range metrics.depth["test"] {
						if depth < 0 {
							t.Errorf("negative depth of %d for priority %d:", depth, priority)
						}
					}

					metrics.mu.Unlock()
					handedOut.Insert(item)
				}()

				func() {
					handedOutLock.Lock()
					defer handedOutLock.Unlock()

					handedOut.Delete(item)
					q.Done(item)
				}()
			}
		}()
	}

	wg.Wait()
}

func newQueueWithTimeForwarder() (_ *priorityqueue[string], _ *fakeMetricsProvider, forwardQueueTime func(time.Duration)) {
	q, m := newQueue()

	now := time.Now().Round(time.Second)
	nowLock := sync.Mutex{}
	tick := make(chan time.Time)

	q.now = func() time.Time {
		nowLock.Lock()
		defer nowLock.Unlock()
		return now
	}
	q.tick = func(d time.Duration) <-chan time.Time {
		return tick
	}

	return q, m, func(d time.Duration) {
		nowLock.Lock()
		now = now.Add(d)
		nowLock.Unlock()
		tick <- now
	}
}

func newQueue() (*priorityqueue[string], *fakeMetricsProvider) {
	metrics := newFakeMetricsProvider()
	q := New("test", func(o *Opts[string]) {
		o.MetricProvider = metrics
	})
	q.(*priorityqueue[string]).queue = &btreeInteractionValidator{
		bTree: q.(*priorityqueue[string]).queue,
	}

	// validate that tick always gets a positive value as it will just return
	// nil otherwise, which results in blocking forever.
	upstreamTick := q.(*priorityqueue[string]).tick
	q.(*priorityqueue[string]).tick = func(d time.Duration) <-chan time.Time {
		if d <= 0 {
			panic(fmt.Sprintf("got non-positive tick: %v", d))
		}
		return upstreamTick(d)
	}
	return q.(*priorityqueue[string]), metrics
}

type btreeInteractionValidator struct {
	bTree[*item[string]]
}

func (b *btreeInteractionValidator) ReplaceOrInsert(item *item[string]) (*item[string], bool) {
	// There is no codepath that updates an item
	item, alreadyExist := b.bTree.ReplaceOrInsert(item)
	if alreadyExist {
		panic(fmt.Sprintf("ReplaceOrInsert: item %v already existed", item))
	}
	return item, alreadyExist
}

func (b *btreeInteractionValidator) Delete(item *item[string]) (*item[string], bool) {
	// There is no codepath that deletes an item that doesn't exist
	old, existed := b.bTree.Delete(item)
	if !existed {
		panic(fmt.Sprintf("Delete: item %v not found", item))
	}
	return old, existed
}

func TestItemIsOnlyReturnedAfterAfterHasPassed(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := NewWithT(t)
		q, metrics, forwardQueueTimeBy := newQueueWithTimeForwarder()
		defer q.ShutDown()

		originalTick := q.tick
		q.tick = func(d time.Duration) <-chan time.Time {
			g.Expect(d).To(Equal(time.Second))
			return originalTick(d)
		}

		retrievedItem := make(chan struct{})
		go func() {
			q.GetWithPriority()
			close(retrievedItem)
		}()

		q.AddWithOpts(AddOpts{After: time.Second}, "foo")
		synctest.Wait()

		g.Expect(retrievedItem).ShouldNot(BeClosed())

		forwardQueueTimeBy(time.Second)
		synctest.Wait()
		g.Expect(retrievedItem).Should(BeClosed())

		g.Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 0}))
		g.Expect(metrics.adds["test"]).To(Equal(1))
		g.Expect(metrics.retries["test"]).To(Equal(1))
	})
}

func TestHighPriorityItemThatBecameReadyIsReturnedBeforeLowPriorityItem(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := NewWithT(t)

		q, metrics, forwardQueueTimeBy := newQueueWithTimeForwarder()
		defer q.ShutDown()

		tickSetup := make(chan any)
		originalTick := q.tick
		q.tick = func(d time.Duration) <-chan time.Time {
			g.Expect(d).To(Equal(time.Second))
			close(tickSetup)
			return originalTick(d)
		}

		lowPriority := -100
		highPriority := 0
		q.AddWithOpts(AddOpts{After: 0, Priority: &lowPriority}, "foo")
		q.AddWithOpts(AddOpts{After: time.Second, Priority: &highPriority}, "prio")
		synctest.Wait()

		g.Expect(tickSetup).To(BeClosed())

		forwardQueueTimeBy(1 * time.Second)
		key, prio, _ := q.GetWithPriority()

		g.Expect(key).To(Equal("prio"))
		g.Expect(prio).To(Equal(0))
		g.Expect(metrics.depth["test"]).To(Equal(map[int]int{-100: 1, 0: 0}))
		g.Expect(metrics.adds["test"]).To(Equal(2))
		g.Expect(metrics.retries["test"]).To(Equal(1))
	})
}

func TestItemIsReturnedAsSoonAsPossible(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := NewWithT(t)

		q, metrics := newQueue()
		defer q.ShutDown()

		retrieved := make(chan struct{})
		go func() {
			item, _, _ := q.GetWithPriority()
			g.Expect(item).To(Equal("foo"))
			close(retrieved)
		}()
		synctest.Wait() // Wait for the above goroutine to be blocked

		q.AddWithOpts(AddOpts{}, "foo")
		synctest.Wait() // Wait until the priorityqueue and the above goroutine finish running

		g.Expect(retrieved).Should(BeClosed())

		g.Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 0}))
		g.Expect(metrics.adds["test"]).To(Equal(1))
	})
}

func TestMultipleItemsWithAfterAreReturnedInCorrectOrder(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := NewWithT(t)

		q, metrics, forwardQueueTimeBy := newQueueWithTimeForwarder()
		defer q.ShutDown()

		originalTick := q.tick
		q.tick = func(d time.Duration) <-chan time.Time {
			// This is not deterministic and depends on which of
			// Add() or Spin() gets the lock first.
			g.Expect(d).To(Or(Equal(200*time.Millisecond), Equal(time.Second)))
			return originalTick(d)
		}

		retrievedItem := make(chan struct{})
		retrievedSecondItem := make(chan struct{})

		go func() {
			first, _, _ := q.GetWithPriority()
			g.Expect(first).To(Equal("bar"))
			close(retrievedItem)

			second, _, _ := q.GetWithPriority()
			g.Expect(second).To(Equal("foo"))
			close(retrievedSecondItem)
		}()

		q.AddWithOpts(AddOpts{After: time.Second}, "foo")
		q.AddWithOpts(AddOpts{After: 200 * time.Millisecond}, "bar")
		synctest.Wait() // Block until the adds are processed

		g.Expect(retrievedItem).NotTo(BeClosed())

		forwardQueueTimeBy(time.Second)
		synctest.Wait() // Block until the priorityqueue finished processing

		g.Expect(retrievedItem).Should(BeClosed())
		g.Expect(retrievedSecondItem).Should(BeClosed())

		g.Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 0}))
		g.Expect(metrics.adds["test"]).To(Equal(2))
	})
}

func TestGetFromPriorityQueueIsUnblockedOnShutdown(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := NewWithT(t)

		q, _ := newQueue()

		getUnblocked := make(chan struct{})

		go func() {
			defer close(getUnblocked)

			item, priority, isShutDown := q.GetWithPriority()
			g.Expect(item).To(Equal(""))
			g.Expect(priority).To(Equal(0))
			g.Expect(isShutDown).To(BeTrue())
		}()
		synctest.Wait() // Wait for the above goroutine to be blocked

		g.Expect(getUnblocked).NotTo(BeClosed())

		// shut down
		q.ShutDown()
		synctest.Wait()

		// Verify the shutdown unblocked the go routine.
		g.Expect(getUnblocked).To(BeClosed())
	})
}

func TestItemsAreInludedInLenAndMetricsOnceTheyAreReady(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := NewWithT(t)

		q, metrics, forwardQueueTimeBy := newQueueWithTimeForwarder()
		defer q.ShutDown()

		q.AddWithOpts(AddOpts{After: 500 * time.Millisecond}, "foo")
		q.AddWithOpts(AddOpts{}, "baz")
		q.AddWithOpts(AddOpts{After: 500 * time.Millisecond}, "bar")
		q.AddWithOpts(AddOpts{}, "bal")
		// Block here until spin finished, otherwise it is possible it
		// checks now() after forwardQueueTimeBy updated it, does then
		// not listen on tick and causes the write to tick from forwardQueueTimeBy
		// to lock up the test.
		synctest.Wait()

		g.Expect(q.Len()).To(Equal(2))
		metrics.mu.Lock()
		g.Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 2}))
		metrics.mu.Unlock()

		forwardQueueTimeBy(time.Second)
		synctest.Wait()

		g.Expect(q.Len()).To(Equal(4))
		metrics.mu.Lock()
		g.Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 4}))
		metrics.mu.Unlock()

		// Drain queue
		for range 4 {
			item, _ := q.Get()
			q.Done(item)
		}
		g.Expect(q.Len()).To(Equal(0))
		metrics.mu.Lock()
		g.Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 0}))
		metrics.mu.Unlock()

		// Validate that doing it again still works to notice bugs with removing
		// it from the queues becameReady tracking.
		q.AddWithOpts(AddOpts{After: 500 * time.Millisecond}, "foo")
		q.AddWithOpts(AddOpts{}, "baz")
		q.AddWithOpts(AddOpts{After: 500 * time.Millisecond}, "bar")
		q.AddWithOpts(AddOpts{}, "bal")
		// Block here until spin finished, otherwise it is possible it
		// checks now() after forwardQueueTimeBy updated it, does then
		// not listen on tick and causes the write to tick from forwardQueueTimeBy
		// to lock up the test.
		synctest.Wait()

		g.Expect(q.Len()).To(Equal(2))
		metrics.mu.Lock()
		g.Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 2}))
		metrics.mu.Unlock()

		forwardQueueTimeBy(time.Second)
		synctest.Wait()

		g.Expect(q.Len()).To(Equal(4))
		metrics.mu.Lock()
		g.Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 4}))
		metrics.mu.Unlock()
	})
}

func TestMetricsAreUpdatedForItemWhoseRequeueAfterExpiredThatGetsAddedAgainWithoutRequeueAfter(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := NewWithT(t)

		q, metrics, forwardQueueTimeBy := newQueueWithTimeForwarder()
		defer q.ShutDown()

		q.AddWithOpts(AddOpts{After: 50 * time.Millisecond}, "foo")
		synctest.Wait()
		forwardQueueTimeBy(50 * time.Millisecond)
		synctest.Wait()

		g.Expect(q.Len()).To(Equal(1))
		metrics.mu.Lock()
		g.Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 1}))
		metrics.mu.Unlock()

		q.AddWithOpts(AddOpts{}, "foo")
		g.Expect(q.Len()).To(Equal(1))
		metrics.mu.Lock()
		g.Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 1}))
		metrics.mu.Unlock()

		// Get the item to ensure the codepath in
		// `spin` for the metrics is passed by so
		// that this starts failing if it incorrectly
		// calls `metrics.add` again.
		item, _ := q.Get()
		g.Expect(item).To(Equal("foo"))
		g.Expect(q.Len()).To(Equal(0))
		metrics.mu.Lock()
		g.Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 0}))
		metrics.mu.Unlock()
	})
}

func TesWhenAddingMultipleItemsWithRatelimitTrueTheyDontAffectEachOther(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		g := NewWithT(t)

		q, metrics, forwardQueueTimeBy := newQueueWithTimeForwarder()
		defer q.ShutDown()

		q.rateLimiter = workqueue.NewTypedItemExponentialFailureRateLimiter[string](5*time.Millisecond, 1000*time.Second)
		originalTick := q.tick
		q.tick = func(d time.Duration) <-chan time.Time {
			g.Expect(d).To(Or(Equal(5*time.Millisecond), Equal(635*time.Millisecond)))
			return originalTick(d)
		}

		retrievedItem := make(chan struct{})
		retrievedSecondItem := make(chan struct{})

		go func() {
			first, _, _ := q.GetWithPriority()
			g.Expect(first).To(Equal("foo"))
			close(retrievedItem)

			second, _, _ := q.GetWithPriority()
			g.Expect(second).To(Equal("bar"))
			close(retrievedSecondItem)
		}()

		// after 7 calls, the next When("bar") call will return 640ms.
		for range 7 {
			q.rateLimiter.When("bar")
		}
		q.AddWithOpts(AddOpts{RateLimited: true}, "foo", "bar")
		synctest.Wait() // Block until the adds are processed
		g.Expect(retrievedItem).NotTo(BeClosed())

		forwardQueueTimeBy(5 * time.Millisecond)
		synctest.Wait()
		g.Expect(retrievedItem).NotTo(BeClosed())
		g.Expect(retrievedSecondItem).NotTo(BeClosed())

		forwardQueueTimeBy(635 * time.Millisecond)
		synctest.Wait()
		g.Expect(retrievedSecondItem).To(BeClosed())

		g.Expect(metrics.depth["test"]).To(Equal(map[int]int{0: 0}))
		g.Expect(metrics.adds["test"]).To(Equal(2))
		g.Expect(metrics.retries["test"]).To(Equal(2))
	})
}
