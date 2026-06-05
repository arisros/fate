package fate

import "time"

// TimerID uniquely identifies one armed delayed ("after") transition for as
// long as it is pending. It is derived deterministically from the owning
// state's path, the delay, and the delay's index within that state, so the same
// logical timer keeps the same ID across runs and across persistence — a
// prerequisite for replay-safe driving by an adapter.
type TimerID string

// PendingTimer describes one delayed ("after") transition the actor currently
// has armed. It is what an adapter reads from [Actor.PendingTimers] to learn
// which timers to drive; when the adapter decides a delay has elapsed it calls
// [Actor.FireTimer] with the ID.
//
// The fate core is clock-agnostic: it never sleeps, reads the wall clock, or
// starts a goroutine for a timer. It only records that a state wants to fire
// "after Delay" and exposes that intent. How and when the timer actually fires
// is entirely the adapter's responsibility (a Temporal adapter maps it to
// workflow.NewTimer; an in-memory adapter maps it to the OS clock; a test
// drives it by hand).
type PendingTimer struct {
	// ID is the timer's stable identifier, passed back to [Actor.FireTimer].
	ID TimerID
	// Delay is the configured delay of the underlying after-transition. An
	// adapter that resumes a persisted actor is responsible for tracking how
	// much of the delay has already elapsed.
	Delay time.Duration
}
