package priorityqueue

import (
	"fmt"
	"sync"
	"testing"
	"time"

	fuzz "github.com/google/gofuzz"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/sets"
)

var _ = Describe("Controllerworkqueue", func() {
	It("returns an item", func() {
		q, metrics := newQueue()
		defer q.ShutDown()
		q.AddWithOpts(AddOpts{}, "foo")

		item, _, _ := q.GetWithPriority()
		Expect(item).To(Equal("foo"))

		Expect(metrics.depth["test"]).To(Equal(0))
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

		Expect(metrics.depth["test"]).To(Equal(0))
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

		Expect(metrics.depth["test"]).To(Equal(1))
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

		Expect(metrics.depth["test"]).To(Equal(1))
		Expect(metrics.adds["test"]).To(Equal(4))
	})

	It("de-duplicates items", func() {
		q, metrics := newQueue()
		defer q.ShutDown()

		q.AddWithOpts(AddOpts{}, "foo")
		q.AddWithOpts(AddOpts{}, "foo")

		Consistently(q.Len).Should(Equal(1))

		cwq := q.(*priorityqueue[string])
		cwq.lockedLock.Lock()
		Expect(cwq.locked.Len()).To(Equal(0))

		Expect(metrics.depth["test"]).To(Equal(1))
		Expect(metrics.adds["test"]).To(Equal(1))
	})

	It("retains the highest priority", func() {
		q, metrics := newQueue()
		defer q.ShutDown()

		q.AddWithOpts(AddOpts{Priority: 1}, "foo")
		q.AddWithOpts(AddOpts{Priority: 2}, "foo")

		item, priority, _ := q.GetWithPriority()
		Expect(item).To(Equal("foo"))
		Expect(priority).To(Equal(2))

		Expect(q.Len()).To(Equal(0))

		Expect(metrics.depth["test"]).To(Equal(0))
		Expect(metrics.adds["test"]).To(Equal(1))
	})

	It("gets pushed to the front if the priority increases", func() {
		q, metrics := newQueue()
		defer q.ShutDown()

		q.AddWithOpts(AddOpts{}, "foo")
		q.AddWithOpts(AddOpts{}, "bar")
		q.AddWithOpts(AddOpts{}, "baz")
		q.AddWithOpts(AddOpts{Priority: 1}, "baz")

		item, priority, _ := q.GetWithPriority()
		Expect(item).To(Equal("baz"))
		Expect(priority).To(Equal(1))

		Expect(q.Len()).To(Equal(2))

		Expect(metrics.depth["test"]).To(Equal(2))
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
		Expect(metrics.depth["test"]).To(Equal(0))
		Expect(metrics.adds["test"]).To(Equal(1))
	})

	It("returns an item only after after has passed", func() {
		q, metrics := newQueue()
		defer q.ShutDown()

		now := time.Now().Round(time.Second)
		nowLock := sync.Mutex{}
		tick := make(chan time.Time)

		cwq := q.(*priorityqueue[string])
		cwq.now = func() time.Time {
			nowLock.Lock()
			defer nowLock.Unlock()
			return now
		}
		cwq.tick = func(d time.Duration) <-chan time.Time {
			Expect(d).To(Equal(time.Second))
			return tick
		}

		retrievedItem := make(chan struct{})

		go func() {
			defer GinkgoRecover()
			q.GetWithPriority()
			close(retrievedItem)
		}()

		q.AddWithOpts(AddOpts{After: time.Second}, "foo")

		Consistently(retrievedItem).ShouldNot(BeClosed())

		nowLock.Lock()
		now = now.Add(time.Second)
		nowLock.Unlock()
		tick <- now
		Eventually(retrievedItem).Should(BeClosed())

		Expect(metrics.depth["test"]).To(Equal(0))
		Expect(metrics.adds["test"]).To(Equal(1))
		Expect(metrics.retries["test"]).To(Equal(1))
	})

	It("returns an item to a waiter as soon as it has one", func() {
		q, metrics := newQueue()
		defer q.ShutDown()

		retrieved := make(chan struct{})
		go func() {
			defer GinkgoRecover()
			item, _, _ := q.GetWithPriority()
			Expect(item).To(Equal("foo"))
			close(retrieved)
		}()

		// We are waiting for the GetWithPriority() call to be blocked
		// on retrieving an item. As golang doesn't provide a way to
		// check if something is listening on a channel without
		// sending them a message, I can't think of a way to do this
		// without sleeping.
		time.Sleep(time.Second)
		q.AddWithOpts(AddOpts{}, "foo")
		Eventually(retrieved).Should(BeClosed())

		Expect(metrics.depth["test"]).To(Equal(0))
		Expect(metrics.adds["test"]).To(Equal(1))
	})

	It("returns multiple items with after in correct order", func() {
		q, metrics := newQueue()
		defer q.ShutDown()

		now := time.Now().Round(time.Second)
		nowLock := sync.Mutex{}
		tick := make(chan time.Time)

		cwq := q.(*priorityqueue[string])
		cwq.now = func() time.Time {
			nowLock.Lock()
			defer nowLock.Unlock()
			return now
		}
		cwq.tick = func(d time.Duration) <-chan time.Time {
			// What a bunch of bs. Deferring in here causes
			// ginkgo to deadlock, presumably because it
			// never returns after the defer. Not deferring
			// hides the actual assertion result and makes
			// it complain that there should be a defer.
			// Move the assertion into a goroutine just to
			// get around that mess.
			done := make(chan struct{})
			go func() {
				defer GinkgoRecover()
				defer close(done)

				// This is not deterministic and depends on which of
				// Add() or Spin() gets the lock first.
				Expect(d).To(Or(Equal(200*time.Millisecond), Equal(time.Second)))
			}()
			<-done
			return tick
		}

		retrievedItem := make(chan struct{})
		retrievedSecondItem := make(chan struct{})

		go func() {
			defer GinkgoRecover()
			first, _, _ := q.GetWithPriority()
			Expect(first).To(Equal("bar"))
			close(retrievedItem)

			second, _, _ := q.GetWithPriority()
			Expect(second).To(Equal("foo"))
			close(retrievedSecondItem)
		}()

		q.AddWithOpts(AddOpts{After: time.Second}, "foo")
		q.AddWithOpts(AddOpts{After: 200 * time.Millisecond}, "bar")

		Consistently(retrievedItem).ShouldNot(BeClosed())

		nowLock.Lock()
		now = now.Add(time.Second)
		nowLock.Unlock()
		tick <- now
		Eventually(retrievedItem).Should(BeClosed())
		Eventually(retrievedSecondItem).Should(BeClosed())

		Expect(metrics.depth["test"]).To(Equal(0))
		Expect(metrics.adds["test"]).To(Equal(2))
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
func TestFuzzPrioriorityQueue(t *testing.T) {
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
	q, _ := newQueue()

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
					if existingPriority, exists := inQueue[item]; !exists || existingPriority < opts.Priority {
						inQueue[item] = opts.Priority
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

	if expected := len(inQueue); expected != q.Len() {
		t.Errorf("Expected queue length to be %d, was %d", expected, q.Len())
	}
}

func newQueue() (PriorityQueue[string], *fakeMetricsProvider) {
	metrics := newFakeMetricsProvider()
	q := New("test", func(o *Opts[string]) {
		o.MetricProvider = metrics
	})
	q.(*priorityqueue[string]).queue = &btreeInteractionValidator{
		bTree: q.(*priorityqueue[string]).queue,
	}

	upstreamTick := q.(*priorityqueue[string]).tick
	q.(*priorityqueue[string]).tick = func(d time.Duration) <-chan time.Time {
		if d <= 0 {
			panic(fmt.Sprintf("got non-positive tick: %v", d))
		}
		return upstreamTick(d)
	}
	return q, metrics
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
	// There is node codepath that deletes an item that doesn't exist
	old, existed := b.bTree.Delete(item)
	if !existed {
		panic(fmt.Sprintf("Delete: item %v not found", item))
	}
	return old, existed
}
