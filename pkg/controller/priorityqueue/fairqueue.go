package priorityqueue

import (
	"slices"

	"k8s.io/utils/ptr"
)

type fairq[T comparable] struct {
	children map[string]*fairq[T]
	items    []*item[T]
	// order is the order in which to dequeue. Nil is used
	// as a magic value to indicate we should dequeue from
	// items instead of a child next.
	order []*string
}

func (q *fairq[T]) add(i *item[T], dimensions ...string) {
	if len(dimensions) == 0 {
		if len(q.order) == len(q.children) {
			q.order = append([]*string{nil}, q.order...)
		}
		q.items = append(q.items, i)

		// Sort here so items that get unlocked can be added, rather
		// than having to completely drain and rebuild the fairQueue.
		slices.SortFunc(q.items, func(a, b *item[T]) int {
			if less(a, b) {
				return -1
			}
			return 1
		})

		return
	}

	dimension := dimensions[0]
	dimensions = dimensions[1:]

	if q.children == nil {
		q.children = make(map[string]*fairq[T], 1)
	}

	_, exists := q.children[dimension]
	if !exists {
		q.children[dimension] = &fairq[T]{}
		q.order = append([]*string{ptr.To(dimension)}, q.order...)
	}

	q.children[dimension].add(i, dimensions...)
}

func (q *fairq[T]) dequeue() (*item[T], bool) {
	var item *item[T]
	var hasItem bool

	for idx, dimension := range q.order {
		switch {
		case dimension != nil: // child element
			item, hasItem = q.children[*dimension].dequeue()
			if !hasItem {
				continue
			}
		case len(q.items) > 0: // leaf element
			item = q.items[0]
			q.items = q.items[1:]
		default: // no items for current dimension, check next
			continue
		}

		q.order = append(q.order[:idx], q.order[idx+1:]...)
		q.order = append(q.order, dimension)

		return item, true
	}

	return item, false
}

func (q *fairq[T]) drain() {
	for _, child := range q.children {
		child.drain()
	}
	q.items = nil
}

func (q *fairq[T]) isEmpty() bool {
	if len(q.items) > 0 {
		return false
	}
	for _, child := range q.children {
		if !child.isEmpty() {
			return false
		}
	}
	return true
}
