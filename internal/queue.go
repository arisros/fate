// Package internal provides building blocks not exported from the public statechart API.
package internal

// EventQueue is a simple FIFO of events of type T.
type EventQueue[T any] struct {
	items []T
}

// Push appends an event to the back of the queue.
func (q *EventQueue[T]) Push(e T) { q.items = append(q.items, e) }

// Pop removes and returns the front event.
func (q *EventQueue[T]) Pop() (T, bool) {
	var zero T
	if len(q.items) == 0 {
		return zero, false
	}
	head := q.items[0]
	q.items = q.items[1:]
	return head, true
}

// Len reports the number of queued events.
func (q *EventQueue[T]) Len() int { return len(q.items) }

// Snapshot returns a copy of the current queue contents in order.
func (q *EventQueue[T]) Snapshot() []T {
	out := make([]T, len(q.items))
	copy(out, q.items)
	return out
}

// Restore replaces the queue contents with the given slice (in order).
func (q *EventQueue[T]) Restore(items []T) {
	q.items = append(q.items[:0], items...)
}
