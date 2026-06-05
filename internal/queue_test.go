package internal

import "testing"

func TestEventQueueFIFOAndPersist(t *testing.T) {
	var q EventQueue[int]
	if q.Len() != 0 {
		t.Fatalf("new queue len = %d", q.Len())
	}
	if _, ok := q.Pop(); ok {
		t.Fatal("Pop on empty queue should report !ok")
	}

	q.Push(1)
	q.Push(2)
	q.Push(3)
	if q.Len() != 3 {
		t.Fatalf("len = %d, want 3", q.Len())
	}

	// Snapshot/Restore round-trip preserves order.
	snap := q.Snapshot()
	var q2 EventQueue[int]
	q2.Restore(snap)

	for _, want := range []int{1, 2, 3} {
		got, ok := q2.Pop()
		if !ok || got != want {
			t.Fatalf("Pop = (%d, %v), want (%d, true)", got, ok, want)
		}
	}
	if _, ok := q2.Pop(); ok {
		t.Fatal("queue should be drained")
	}
}
