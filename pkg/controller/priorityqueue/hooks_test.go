package priorityqueue

import (
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
)

type mockHooks struct {
	mu                 sync.Mutex
	onBecameReadyCalls []onBecameReadyCall
}

type onBecameReadyCall struct {
	item     int
	priority int
}

func (m *mockHooks) OnBecameReady(item int, priority int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onBecameReadyCalls = append(m.onBecameReadyCalls, onBecameReadyCall{
		item:     item,
		priority: priority,
	})
}

func (m *mockHooks) getCalls() []onBecameReadyCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]onBecameReadyCall, len(m.onBecameReadyCalls))
	copy(result, m.onBecameReadyCalls)
	return result
}

var _ = Describe("Hooks", func() {
	It("works with nil hooks", func() {
		q := New[int]("test")
		defer q.ShutDown()

		q.Add(10)
		item, shutdown := q.Get()
		Expect(shutdown).To(BeFalse())
		Expect(item).To(Equal(10))
	})

	It("calls OnBecameReady when item is added without delay", func() {
		hooks := &mockHooks{}
		q := New("test", func(o *Opts[int]) {
			o.Hooks = hooks
		})
		defer q.ShutDown()

		q.AddWithOpts(AddOpts{Priority: ptr.To(5)}, 10)

		item, priority, shutdown := q.GetWithPriority()
		Expect(shutdown).To(BeFalse())
		Expect(item).To(Equal(10))
		Expect(priority).To(Equal(5))

		calls := hooks.getCalls()
		Expect(calls).To(HaveLen(1))
		Expect(calls[0]).To(Equal(onBecameReadyCall{item: 10, priority: 5}))
	})

	It("calls OnBecameReady only once for duplicate items", func() {
		hooks := &mockHooks{}
		q := New("test", func(o *Opts[int]) {
			o.Hooks = hooks
		})
		defer q.ShutDown()

		q.Add(10)
		q.Add(10)
		q.Add(10)

		item, shutdown := q.Get()
		Expect(shutdown).To(BeFalse())
		Expect(item).To(Equal(10))

		calls := hooks.getCalls()
		Expect(calls).To(HaveLen(1))
		Expect(calls[0].item).To(Equal(10))
	})

	It("calls OnBecameReady when priority is increased for existing item", func() {
		hooks := &mockHooks{}
		q := New("test", func(o *Opts[int]) {
			o.Hooks = hooks
		})
		defer q.ShutDown()

		q.AddWithOpts(AddOpts{Priority: ptr.To(1)}, 10)
		q.AddWithOpts(AddOpts{Priority: ptr.To(5)}, 10)

		item, priority, shutdown := q.GetWithPriority()
		Expect(shutdown).To(BeFalse())
		Expect(item).To(Equal(10))
		Expect(priority).To(Equal(5))

		calls := hooks.getCalls()
		Expect(calls).To(HaveLen(1))
		Expect(calls[0]).To(Equal(onBecameReadyCall{item: 10, priority: 1}))
	})

	It("does not call OnBecameReady when item is added with delay", func() {
		hooks := &mockHooks{}
		q := New("test", func(o *Opts[int]) {
			o.Hooks = hooks
		})
		defer q.ShutDown()

		q.AddWithOpts(AddOpts{After: time.Hour}, 10)

		Consistently(func() []onBecameReadyCall {
			return hooks.getCalls()
		}, "100ms").Should(BeEmpty())
	})

	It("calls OnBecameReady when delayed item becomes ready", func() {
		hooks := &mockHooks{}
		q := New("test", func(o *Opts[int]) {
			o.Hooks = hooks
		})
		defer q.ShutDown()

		pq := q.(*priorityqueue[int])
		now := time.Now().Round(time.Second)
		nowLock := sync.Mutex{}
		tick := make(chan time.Time)

		pq.now = func() time.Time {
			nowLock.Lock()
			defer nowLock.Unlock()
			return now
		}
		pq.tick = func(d time.Duration) <-chan time.Time {
			return tick
		}

		q.AddWithOpts(AddOpts{After: time.Second, Priority: ptr.To(3)}, 10)

		Consistently(func() []onBecameReadyCall {
			return hooks.getCalls()
		}, "100ms").Should(BeEmpty())

		// Forward time
		nowLock.Lock()
		now = now.Add(time.Second)
		nowLock.Unlock()
		tick <- now

		Eventually(func() []onBecameReadyCall {
			return hooks.getCalls()
		}).Should(HaveLen(1))

		calls := hooks.getCalls()
		Expect(calls[0]).To(Equal(onBecameReadyCall{item: 10, priority: 3}))
	})

	It("calls OnBecameReady when delayed item is re-added without delay", func() {
		hooks := &mockHooks{}
		q := New("test", func(o *Opts[int]) {
			o.Hooks = hooks
		})
		defer q.ShutDown()

		q.AddWithOpts(AddOpts{After: time.Hour}, 10)
		Expect(hooks.getCalls()).To(BeEmpty())

		// Re-add without delay
		q.AddWithOpts(AddOpts{Priority: ptr.To(2)}, 10)

		Eventually(func() []onBecameReadyCall {
			return hooks.getCalls()
		}).Should(HaveLen(1))

		calls := hooks.getCalls()
		Expect(calls[0]).To(Equal(onBecameReadyCall{item: 10, priority: 2}))
	})

	It("calls OnBecameReady for each unique item", func() {
		hooks := &mockHooks{}
		q := New("test", func(o *Opts[int]) {
			o.Hooks = hooks
		})
		defer q.ShutDown()

		q.Add(10)
		q.Add(20)
		q.Add(30)

		Eventually(func() int {
			return len(hooks.getCalls())
		}).Should(Equal(3))

		calls := hooks.getCalls()
		items := make([]int, len(calls))
		for i, call := range calls {
			items[i] = call.item
		}
		Expect(items).To(ConsistOf(10, 20, 30))
	})
})
