// Package internal provides building blocks not exported from the public
// statechart API.
package internal

// EventQueue is a simple FIFO of events of type T. Used by Actor as the
// internal raise queue. Not goroutine-safe — the Actor's mutex guards
// access.
type EventQueue[T any] struct {
	items []T
}

// Push appends an event to the back of the queue.
func (q *EventQueue[T]) Push(e T) { q.items = append(q.items, e) }

// Pop removes and returns the front event. The second return is false
// when the queue is empty.
func (q *EventQueue[T]) Pop() (T, bool) {
	var zero T
	if len(q.items) == 0 {
		return zero, false
	}
	head := q.items[0]
	// Drop the leading element; rely on slice header reuse — for bounded
	// queue depths this is fine. Switch to a ring buffer if profiling
	// shows churn.
	q.items = q.items[1:]
	return head, true
}

// Len reports the number of queued events.
func (q *EventQueue[T]) Len() int { return len(q.items) }

// Snapshot returns a copy of the current queue contents in order. Used by
// the actor's Persist (P6).
func (q *EventQueue[T]) Snapshot() []T {
	out := make([]T, len(q.items))
	copy(out, q.items)
	return out
}

// Restore replaces the queue contents with the given slice (in order).
// Used by Restore (P6).
func (q *EventQueue[T]) Restore(items []T) {
	q.items = append(q.items[:0], items...)
}
