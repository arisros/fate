package engine

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/arisros/fate/effect"
	"github.com/arisros/fate/persist"
)

type persistedShape[Ctx any, Evt any] struct {
	Version     int                           `json:"version"`
	Status      persist.ActorStatus           `json:"status"`
	Value       persist.StateValue            `json:"value"`
	Context     Ctx                           `json:"context"`
	History     map[string]string             `json:"history,omitempty"`
	HistoryDeep map[string]persist.StateValue `json:"history_deep,omitempty"`
	Queue       []Evt                         `json:"queue,omitempty"`
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
	deep := make(map[string]persist.StateValue, len(a.historyDeepMemory))
	for node, sub := range a.historyDeepMemory {
		deep[strings.Join(node.path, ".")] = sub
	}
	return persistedShape[Ctx, Evt]{
		Version:     persist.SnapshotVersion,
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
	if p.Version > persist.SnapshotVersion {
		return nil, fmt.Errorf("statechart: snapshot version %d is newer than supported %d", p.Version, persist.SnapshotVersion)
	}
	if p.Version < 1 {
		return nil, fmt.Errorf("statechart: snapshot version %d is too old (minimum 1)", p.Version)
	}
	if err := validateValue[Ctx, Evt](m.root, p.Value); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSnapshotMismatch, err)
	}
	a := &Actor[Ctx, Evt]{
		machine:           m,
		ctx:               p.Context,
		value:             p.Value,
		status:            p.Status,
		output:            p.Output,
		errText:           p.Error,
		armed:             map[effect.TimerID]afterBinding[Ctx, Evt]{},
		pendingInvokes:    map[effect.InvokeID]invokeBinding[Ctx, Evt]{},
		historyMemory:     map[*stateNode[Ctx, Evt]]string{},
		historyDeepMemory: map[*stateNode[Ctx, Evt]]persist.StateValue{},
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
	if a.status == persist.StatusRunning {
		for _, n := range activeConfigNodes[Ctx, Evt](m.root, a.value) {
			a.armAfterLocked(n)
			a.armInvokesLocked(n)
		}
	}
	return a, nil
}

func activeConfigNodes[Ctx any, Evt any](root *stateNode[Ctx, Evt], v persist.StateValue) []*stateNode[Ctx, Evt] {
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

func validateValue[Ctx any, Evt any](parent *stateNode[Ctx, Evt], v persist.StateValue) error {
	switch parent.typ {
	case NodeCompound:
		if v.IsAtomic() {
			if _, ok := parent.children[v.Leaf]; !ok {
				return fmt.Errorf("state %q has no child %q", statePath(parent), v.Leaf)
			}
			return nil
		}
		if len(v.Children) != 1 {
			return fmt.Errorf("compound state %q has %d active children", statePath(parent), len(v.Children))
		}
	case NodeParallel:
		if v.IsAtomic() {
			return fmt.Errorf("parallel state %q has no regions in the snapshot", statePath(parent))
		}
		for _, name := range slices.Sorted(maps.Keys(parent.children)) {
			if _, ok := v.Children[name]; !ok {
				return fmt.Errorf("parallel state %q is missing region %q", statePath(parent), name)
			}
		}
	default:
		return nil
	}
	for _, name := range slices.Sorted(maps.Keys(v.Children)) {
		child, ok := parent.children[name]
		if !ok {
			return fmt.Errorf("state %q has no child %q", statePath(parent), name)
		}
		if err := validateValue[Ctx, Evt](child, v.Children[name]); err != nil {
			return err
		}
	}
	return nil
}

func statePath[Ctx any, Evt any](n *stateNode[Ctx, Evt]) string {
	if n.name == "" {
		return "(root)"
	}
	return strings.Join(n.path, ".")
}
