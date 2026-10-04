package effect

import "time"

// TimerID uniquely identifies one armed delayed ("after") transition for as long as it is pending.
type TimerID string

// PendingTimer describes one delayed ("after") transition the actor currently has armed.
type PendingTimer struct {
	ID    TimerID
	Delay time.Duration
	State string
}
