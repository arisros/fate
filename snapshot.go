package fate

import "encoding/json"

// SnapshotVersion is the on-disk shape version. Incremented for
// backward-incompatible changes per ADR-003.
const SnapshotVersion = 1

// ActorStatus is the lifecycle phase of an Actor.
type ActorStatus string

const (
	StatusRunning ActorStatus = "running"
	StatusStopped ActorStatus = "stopped"
	StatusDone    ActorStatus = "done"
	StatusError   ActorStatus = "error"
)

// Snapshot is an immutable view of an actor's state at one instant. Safe to
// marshal to JSON and persist (see ADR-003).
//
// The P3 skeleton only populates Version, Value, Context, and Status. Output,
// Error, Children, Queue, History, and Timers land in later phases.
type Snapshot[Ctx any] struct {
	Version int             `json:"version"`
	Value   StateValue      `json:"value"`
	Context Ctx             `json:"context"`
	Status  ActorStatus     `json:"status"`
	Output  json.RawMessage `json:"output,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// Matches is a convenience wrapper around Value.Matches.
func (s Snapshot[Ctx]) Matches(target string) bool { return s.Value.Matches(target) }
