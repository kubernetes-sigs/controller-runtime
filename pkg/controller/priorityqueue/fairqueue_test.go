package priorityqueue

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("FairQueue", func() {
	DescribeTable("Ordering tests",
		Entry("simple",
			[]string{"foo", "foo", "bar"},
			[]string{"bar", "foo", "foo"},
		),
		Entry("balanced",
			[]string{"foo", "foo", "bar", "bar"},
			[]string{"bar", "foo", "bar", "foo"},
		),
		Entry("mixed-dimensional, multi-dimensional comes first",
			[]string{"foo-foo", "foo-foo", "foo"},
			[]string{"foo", "foo-foo", "foo-foo"},
		),
		Entry("mixed-dimensional, single dimension comes first",
			[]string{"foo", "foo-foo", "foo-foo"},
			[]string{"foo-foo", "foo", "foo-foo"},
		),
		Entry("complex mixed-dimensional",
			[]string{"bar", "foo", "foo-foo", "foo-foo", "foo-bar"},
			// First the foo with the latest-added second dimension, then the bar,
			// then the remaining foos based on the second dimension.
			[]string{"foo-bar", "bar", "foo-foo", "foo", "foo-foo"},
		),

		func(adds []string, expected []string) {
			q := &fairq[string]{}
			for idx, add := range adds {
				q.add(&item[string]{Key: add, AddedCounter: uint64(idx)}, strings.Split(add, "-")...)
			}

			actual := make([]string, 0, len(expected))
			for range len(expected) {
				item, didGetItem := q.dequeue()
				Expect(didGetItem).To(BeTrue())
				actual = append(actual, item.Key)
			}

			_, didGetItem := q.dequeue()
			Expect(didGetItem).To(BeFalse())
			Expect(actual).To(Equal(expected))
		},
	)

	It("retains fairness across queue operations", func() {
		q := &fairq[string]{}
		q.add(&item[string]{Key: "foo"}, "foo")
		_, _ = q.dequeue()
		q.add(&item[string]{Key: "bar", AddedCounter: 1}, "bar")
		q.add(&item[string]{Key: "foo", AddedCounter: 2}, "foo")

		item, _ := q.dequeue()
		Expect(item.Key).To(Equal("bar"))
		Expect(q.isEmpty()).To(BeFalse())
	})

	It("sorts by added counter", func() {
		q := &fairq[string]{}
		q.add(&item[string]{Key: "foo", AddedCounter: 2})
		q.add(&item[string]{Key: "bar", AddedCounter: 1})

		item, _ := q.dequeue()
		Expect(item.Key).To(Equal("bar"))
		Expect(q.isEmpty()).To(BeFalse())
	})

	It("drains all items", func() {
		q := &fairq[string]{}
		q.add(&item[string]{Key: "foo"}, "foo")
		q.add(&item[string]{Key: "bar"}, "bar")
		q.add(&item[string]{Key: "baz"}, "baz")

		q.drain()
		_, gotItem := q.dequeue()
		Expect(gotItem).To(BeFalse())
		Expect(q.isEmpty()).To(BeTrue())
	})
})
