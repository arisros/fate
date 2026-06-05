package fate

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// NodeType discriminates state node kinds. As of P5, Atomic, Compound,
// Final, and History are supported; Parallel is the remaining P5 piece.
type NodeType uint8

const (
	NodeAtomic NodeType = iota
	NodeCompound
	NodeParallel // P5 follow-up
	NodeFinal
	NodeHistory
)

// History selects the depth of memory for a NodeHistory pseudo-state.
//
//   - HistoryShallow remembers only the immediate child of the parent compound.
//     On re-entry, the parent restarts that child via the child's initial chain.
//   - HistoryDeep remembers the full descendant configuration. On re-entry,
//     the entire active sub-tree at exit time is restored.
type History uint8

const (
	HistoryShallow History = iota
	HistoryDeep
)

// String returns the textual name of the node type. Used in error messages
// and snapshot debugging output.
func (t NodeType) String() string {
	switch t {
	case NodeAtomic:
		return "atomic"
	case NodeCompound:
		return "compound"
	case NodeParallel:
		return "parallel"
	case NodeFinal:
		return "final"
	case NodeHistory:
		return "history"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

// StateValue represents the current configuration of a running statechart.
//
// For an atomic or final state, Leaf is the state's local name and Children
// is nil. For a compound state, Leaf is the empty string and Children has
// exactly one entry: the active child's name → its own StateValue. For a
// parallel state, Children may have multiple entries — one per region.
//
// JSON marshaling collapses atomic states to a bare string and compound /
// parallel states to a {"name": child} object, matching the XState v5
// snapshot shape (see ADR-003).
type StateValue struct {
	Leaf     string
	Children map[string]StateValue
}

// AtomicValue constructs a StateValue for a leaf node.
func AtomicValue(name string) StateValue {
	return StateValue{Leaf: name}
}

// CompoundValue constructs a StateValue for a compound or parallel node.
// The children map keys are immediate child state names; values are their
// (possibly nested) StateValues.
func CompoundValue(children map[string]StateValue) StateValue {
	return StateValue{Children: children}
}

// IsAtomic reports whether the value represents a leaf state.
func (v StateValue) IsAtomic() bool {
	return len(v.Children) == 0
}

// Path returns the state value flattened to a dot-separated path. For a
// compound state {"a": {"b": "c"}}, it returns "a.b.c". For a parallel state
// with multiple regions, the regions are joined alphabetically with " | ":
// {"a": "x", "b": "y"} → "a.x | b.y". Used for human-friendly logging and
// inspection.
func (v StateValue) Path() string {
	if v.IsAtomic() {
		return v.Leaf
	}
	keys := make([]string, 0, len(v.Children))
	for k := range v.Children {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		child := v.Children[k].Path()
		if child == "" {
			parts = append(parts, k)
		} else {
			parts = append(parts, k+"."+child)
		}
	}
	return strings.Join(parts, " | ")
}

// Matches reports whether the state value matches the given dot-separated
// target path. A target "a.b" matches a value that is in state "a" with
// active descendant "b". A target "a" matches any value where region "a"
// is active (used for parallel-region queries).
func (v StateValue) Matches(target string) bool {
	if target == "" {
		return true
	}
	if v.IsAtomic() {
		return v.Leaf == target
	}
	head, tail, _ := strings.Cut(target, ".")
	child, ok := v.Children[head]
	if !ok {
		return false
	}
	return child.Matches(tail)
}

// MarshalJSON implements json.Marshaler.
//
//   - Atomic state →   "name"
//   - Compound/parallel → {"name": <child JSON>, ...}
func (v StateValue) MarshalJSON() ([]byte, error) {
	if v.IsAtomic() {
		return json.Marshal(v.Leaf)
	}
	// Sort keys for deterministic output (per ADR-002 / ADR-007).
	keys := make([]string, 0, len(v.Children))
	for k := range v.Children {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		b.Write(kb)
		b.WriteByte(':')
		cb, err := v.Children[k].MarshalJSON()
		if err != nil {
			return nil, err
		}
		b.Write(cb)
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}

// UnmarshalJSON implements json.Unmarshaler. Accepts both the atomic form
// (a JSON string) and the compound/parallel form (a JSON object).
func (v *StateValue) UnmarshalJSON(data []byte) error {
	data = bytesTrimSpace(data)
	if len(data) == 0 {
		return fmt.Errorf("statechart: empty state value JSON")
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		v.Leaf = s
		v.Children = nil
		return nil
	}
	if data[0] != '{' {
		return fmt.Errorf("statechart: state value must be string or object, got %s", string(data))
	}
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	v.Leaf = ""
	v.Children = make(map[string]StateValue, len(raw))
	for k, r := range raw {
		var child StateValue
		if err := child.UnmarshalJSON(r); err != nil {
			return fmt.Errorf("statechart: child %q: %w", k, err)
		}
		v.Children[k] = child
	}
	return nil
}

func bytesTrimSpace(b []byte) []byte {
	i, j := 0, len(b)
	for i < j && isSpace(b[i]) {
		i++
	}
	for j > i && isSpace(b[j-1]) {
		j--
	}
	return b[i:j]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
