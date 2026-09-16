package fate

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SnapshotVersion is the on-disk shape version.
const SnapshotVersion = 1

// ActorStatus is the lifecycle phase of an Actor.
type ActorStatus string

const (
	StatusRunning ActorStatus = "running"
	StatusStopped ActorStatus = "stopped"
	StatusDone    ActorStatus = "done"
	StatusError   ActorStatus = "error"
)

// Snapshot is an immutable view of an actor's state at one instant.
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

type persistedShape[Ctx any, Evt any] struct {
	Version     int                   `json:"version"`
	Status      ActorStatus           `json:"status"`
	Value       StateValue            `json:"value"`
	Context     Ctx                   `json:"context"`
	History     map[string]string     `json:"history,omitempty"`
	HistoryDeep map[string]StateValue `json:"history_deep,omitempty"`
	Queue       []Evt                 `json:"queue,omitempty"`
	// Timers and invocations are not stored: restore re-derives them (ADR-0004).
	Output json.RawMessage `json:"output,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Persist returns a JSON snapshot of the actor's state suitable for storage (e.g. ArangoDB) and later restoration via NewActorFromSnapshot.
func (a *Actor[Ctx, Evt]) Persist() ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return json.Marshal(a.persistedShapeLocked())
}

func (a *Actor[Ctx, Evt]) persistedShapeLocked() persistedShape[Ctx, Evt] {
	history := make(map[string]string, len(a.historyMemory))
	for node, child := range a.historyMemory {
		history[strings.Join(node.path, ".")] = child
	}
	deep := make(map[string]StateValue, len(a.historyDeepMemory))
	for node, sub := range a.historyDeepMemory {
		deep[strings.Join(node.path, ".")] = sub
	}
	return persistedShape[Ctx, Evt]{
		Version:     SnapshotVersion,
		Status:      a.status,
		Value:       a.value,
		Context:     a.ctx,
		History:     history,
		HistoryDeep: deep,
		Queue:       a.queue.Snapshot(),
		Output:      a.output,
		Error:       a.errText,
	}
}

// NewActorFromSnapshot constructs an actor seeded from a JSON snapshot.
func NewActorFromSnapshot[Ctx any, Evt any](m *Machine[Ctx, Evt], persisted []byte) (*Actor[Ctx, Evt], error) {
	var p persistedShape[Ctx, Evt]
	if err := json.Unmarshal(persisted, &p); err != nil {
		return nil, fmt.Errorf("statechart: unmarshal snapshot: %w", err)
	}
	if p.Version > SnapshotVersion {
		return nil, fmt.Errorf("statechart: snapshot version %d is newer than supported %d", p.Version, SnapshotVersion)
	}
	if p.Version < 1 {
		return nil, fmt.Errorf("statechart: snapshot version %d is too old (minimum 1)", p.Version)
	}
	a := &Actor[Ctx, Evt]{
		machine:           m,
		ctx:               p.Context,
		value:             p.Value,
		status:            p.Status,
		output:            p.Output,
		errText:           p.Error,
		armed:             map[TimerID]afterBinding[Ctx, Evt]{},
		pendingInvokes:    map[InvokeID]invokeBinding[Ctx, Evt]{},
		historyMemory:     map[*stateNode[Ctx, Evt]]string{},
		historyDeepMemory: map[*stateNode[Ctx, Evt]]StateValue{},
	}
	for path, child := range p.History {
		node := lookupByPath[Ctx, Evt](m.root, path)
		if node != nil {
			a.historyMemory[node] = child
		}
	}
	for path, sub := range p.HistoryDeep {
		node := lookupByPath[Ctx, Evt](m.root, path)
		if node != nil {
			a.historyDeepMemory[node] = sub
		}
	}
	if len(p.Queue) > 0 {
		a.queue.Restore(p.Queue)
	}
	if a.status == StatusRunning {
		for _, n := range activeConfigNodes[Ctx, Evt](m.root, a.value) {
			a.armAfterLocked(n)
			a.armInvokesLocked(n)
		}
	}
	return a, nil
}

func activeConfigNodes[Ctx any, Evt any](root *stateNode[Ctx, Evt], v StateValue) []*stateNode[Ctx, Evt] {
	leaves := resolveLeaves[Ctx, Evt](root, v)
	seen := map[*stateNode[Ctx, Evt]]bool{}
	var out []*stateNode[Ctx, Evt]
	for _, leaf := range leaves {
		for cursor := leaf; cursor != nil && cursor.name != ""; cursor = cursor.parent {
			if seen[cursor] {
				continue
			}
			seen[cursor] = true
			out = append(out, cursor)
		}
	}
	return out
}

func lookupByPath[Ctx any, Evt any](root *stateNode[Ctx, Evt], path string) *stateNode[Ctx, Evt] {
	if path == "" {
		return root
	}
	cursor := root
	for _, segment := range strings.Split(path, ".") {
		next, ok := cursor.children[segment]
		if !ok {
			return nil
		}
		cursor = next
	}
	return cursor
}
