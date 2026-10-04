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

// persistedShape is the JSON layout of a persisted actor snapshot. Versioned
// per ADR-003; backward-compat is the responsibility of restoreV1, restoreV2,
// etc. — never break old shapes silently.
//
// Note on generics + JSON: Ctx and Evt are user types. They must be JSON-
// marshalable for Persist to succeed. For sealed-interface Evt types where
// the concrete type isn't recoverable from JSON alone, callers can layer a
// codec on top of Persist — see ADR-003 follow-up notes.
type persistedShape[Ctx any, Evt any] struct {
	Version int                 `json:"version"`
	Status  persist.ActorStatus `json:"status"`
	Value   persist.StateValue  `json:"value"`
	Context Ctx                 `json:"context"`
	// History stores shallow-history memory: compound state's dot-path →
	// remembered immediate child name.
	History map[string]string `json:"history,omitempty"`
	// HistoryDeep stores deep-history memory: compound state's dot-path →
	// saved value-inside subtree. Added 2026-05-27; older snapshots that
	// lack this field unmarshal it as an empty map, which is a safe
	// fallback — the next compound exit re-populates it.
	HistoryDeep map[string]persist.StateValue `json:"history_deep,omitempty"`
	Queue       []Evt                         `json:"queue,omitempty"`
	// Seq is the number of steps taken, so Step.Seq keeps counting after a
	// restore. Snapshots written before it existed read as zero.
	Seq uint64 `json:"seq,omitempty"`
	// Output and Error capture a completed/failed actor's result. Pending
	// timers and invocations are intentionally NOT stored: they are re-derived
	// from the active configuration on restore (see ADR-0004).
	Output json.RawMessage `json:"output,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Persist returns a JSON snapshot of the actor's state suitable for storage
// (e.g. ArangoDB) and later restoration via NewActorFromSnapshot.
//
// Round-trip guarantee: NewActorFromSnapshot(m, actor.Persist()) produces an
// actor that, given the same future events, yields byte-identical Persist
// output to the original.
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
		Seq:         a.seq,
		Output:      a.output,
		Error:       a.errText,
	}
}

// NewActorFromSnapshot constructs an actor seeded from a JSON snapshot.
//
// Restoration sequence:
//   - Validates the snapshot version is supported.
//   - Validates the state value against the machine, returning
//     ErrSnapshotMismatch when it names a state the machine does not have,
//     gives a compound state more than one active child, or leaves a parallel
//     region out.
//   - Rebuilds the history memory by resolving stored path strings to
//     stateNode pointers within the supplied machine.
//   - Restores any queued internal events.
//
// The restored actor has the same status as when persisted; if it was
// running, it is running after restoration (no Start needed).
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
		seq:               p.Seq,
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
	// Re-derive pending effects (timers, invocations) from the active
	// configuration rather than storing them. Entry actions are NOT re-run;
	// arming only records intent for the adapter to pull. See ADR-0004.
	if a.status == persist.StatusRunning {
		for _, n := range activeConfigNodes[Ctx, Evt](m.root, a.value) {
			a.armAfterLocked(n)
			a.armInvokesLocked(n)
		}
	}
	return a, nil
}

// activeConfigNodes returns every state node in the active configuration for
// value v — each active leaf and all of its ancestors (excluding the synthetic
// root) — so on-entry effects can be re-derived after restore.
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

// lookupByPath resolves a dot-separated descendant path to its stateNode.
// Returns nil if any segment is missing. An empty path returns the root.
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

// validateValue reports why v, the value inside parent, is not a configuration
// of the machine.
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
