package controllerworkqueue

import (
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

		cwq := q.(*metricWrappedQueue[string])
		cwq.lockedLock.Lock()
		Expect(cwq.locked.Len()).To(Equal(0))

		Expect(metrics.depth["test"]).To(Equal(1))
		Expect(metrics.adds["test"]).To(Equal(2))
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
		Expect(metrics.adds["test"]).To(Equal(2))
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
		Expect(metrics.adds["test"]).To(Equal(4))
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
		Expect(metrics.adds["test"]).To(Equal(2))
	})

	It("returns an item only after after has passed", func() {
		q, metrics := newQueue()
		defer q.ShutDown()

		now := time.Now().Round(time.Second)
		nowLock := sync.Mutex{}
		tick := make(chan time.Time)

		cwq := q.(*metricWrappedQueue[string])
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
	})

	It("returns multiple items with after in correct order", func() {
		q, metrics := newQueue()
		defer q.ShutDown()

		now := time.Now().Round(time.Second)
		nowLock := sync.Mutex{}
		tick := make(chan time.Time)

		cwq := q.(*metricWrappedQueue[string])
		cwq.now = func() time.Time {
			nowLock.Lock()
			defer nowLock.Unlock()
			return now
		}
		cwq.tick = func(d time.Duration) <-chan time.Time {
			Expect(d).To(Equal(200 * time.Millisecond))
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
	return q, metrics
}
