package fate

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
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
	Version int         `json:"version"`
	Status  ActorStatus `json:"status"`
	Value   StateValue  `json:"value"`
	Context Ctx         `json:"context"`
	// History stores shallow-history memory: compound state's dot-path →
	// remembered immediate child name.
	History map[string]string `json:"history,omitempty"`
	// HistoryDeep stores deep-history memory: compound state's dot-path →
	// saved value-inside subtree. Added 2026-05-27; older snapshots that
	// lack this field unmarshal it as an empty map, which is a safe
	// fallback — the next compound exit re-populates it.
	HistoryDeep map[string]StateValue `json:"history_deep,omitempty"`
	Queue       []Evt                 `json:"queue,omitempty"`
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
//
// Restoration sequence:
//   - Validates the snapshot version is supported.
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
	// Re-derive pending effects (timers, invocations) from the active
	// configuration rather than storing them. Entry actions are NOT re-run;
	// arming only records intent for the adapter to pull. See ADR-0004.
	if a.status == StatusRunning {
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

// PersistDeterministic returns a JSON snapshot with sorted map keys, matching
// the ADR-007 determinism requirement: identical actor state must produce
// byte-identical Persist output across runs.
//
// The standard library's json.Marshal already sorts struct fields; map keys
// in StateValue.MarshalJSON are sorted; the history map iteration uses sorted
// keys here. So Persist() and PersistDeterministic() currently produce the
// same bytes — but PersistDeterministic is the explicit guarantee surface
// that downstream code should call when byte-equality matters.
func (a *Actor[Ctx, Evt]) PersistDeterministic() ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	p := a.persistedShapeLocked()
	// Sort history keys for stable iteration via json's internal map sort.
	// (Go's json.Marshal sorts map[string]X keys lexicographically.)
	if len(p.History) > 0 {
		_ = sortedKeys(p.History) // touched here to make the intent visible.
	}
	return json.Marshal(p)
}

// sortedKeys returns the keys of m in sorted order — a defensive helper
// referenced by PersistDeterministic as a documentation hook.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
