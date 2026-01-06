package fate

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// NodeType discriminates state node kinds.
type NodeType uint8

const (
	NodeAtomic NodeType = iota
	NodeCompound
	NodeParallel
	NodeFinal
	NodeHistory
)

// History selects the depth of memory for a NodeHistory pseudo-state.
type History uint8

const (
	HistoryShallow History = iota
	HistoryDeep
)

// String returns the textual name of the node type.
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
type StateValue struct {
	Leaf     string
	Children map[string]StateValue
}

// AtomicValue constructs a StateValue for a leaf node.
func AtomicValue(name string) StateValue {
	return StateValue{Leaf: name}
}

// CompoundValue constructs a StateValue for a compound or parallel node.
func CompoundValue(children map[string]StateValue) StateValue {
	return StateValue{Children: children}
}

// IsAtomic reports whether the value represents a leaf state.
func (v StateValue) IsAtomic() bool {
	return len(v.Children) == 0
}

// Path returns the state value flattened to a dot-separated path.
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

// Matches reports whether the state value matches the given dot-separated target path.
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
func (v StateValue) MarshalJSON() ([]byte, error) {
	if v.IsAtomic() {
		return json.Marshal(v.Leaf)
	}
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

// UnmarshalJSON implements json.Unmarshaler.
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
