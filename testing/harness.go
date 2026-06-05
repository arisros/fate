// Package testing provides helpers for asserting on Actor behavior in unit
// tests. Not for use outside test code.
package testing

import (
	"context"
	"fmt"
	"time"

	"github.com/arisros/fate"
)

// WaitFor blocks until the actor's snapshot satisfies pred or the deadline
// expires. The default deadline is 1 second; override with a non-zero
// timeout argument.
func WaitFor[Ctx any, Evt any](
	a *fate.Actor[Ctx, Evt],
	pred func(fate.Snapshot[Ctx]) bool,
	timeout time.Duration,
) (fate.Snapshot[Ctx], error) {
	if timeout == 0 {
		timeout = time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap := a.Snapshot()
		if pred(snap) {
			return snap, nil
		}
		time.Sleep(time.Millisecond)
	}
	return a.Snapshot(), fmt.Errorf("statechart/testing: timed out waiting %s for snapshot predicate", timeout)
}

// Trace returns a recorder that captures every snapshot emitted by the
// actor's Subscribe channel, in order. Useful for asserting on the sequence
// of states a test drives the actor through.
type Trace[Ctx any] struct {
	Snapshots []fate.Snapshot[Ctx]
	unsub     func()
}

// NewTrace starts capturing snapshots from the actor immediately.
func NewTrace[Ctx any, Evt any](a *fate.Actor[Ctx, Evt]) *Trace[Ctx] {
	t := &Trace[Ctx]{}
	t.unsub = a.Subscribe(func(s fate.Snapshot[Ctx]) {
		t.Snapshots = append(t.Snapshots, s)
	})
	return t
}

// Stop unsubscribes the trace from the actor. Safe to call multiple times.
func (t *Trace[Ctx]) Stop() {
	if t.unsub != nil {
		t.unsub()
		t.unsub = nil
	}
}

// Paths returns the sequence of state-value paths observed by the trace.
// Convenient for one-line equality assertions in tests.
func (t *Trace[Ctx]) Paths() []string {
	out := make([]string, len(t.Snapshots))
	for i, s := range t.Snapshots {
		out[i] = s.Value.Path()
	}
	return out
}

// DefaultContext returns the standard test context. Centralized so future
// tweaks (e.g. attaching a logger) apply to every test that uses the harness.
func DefaultContext() context.Context { return context.Background() }
