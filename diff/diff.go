// Package diff computes structural differences between two [fate.Snapshot] values of the same context type.
package diff

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/arisros/fate"
)

// Kind enumerates the categories of difference [Snapshots] may surface.
type Kind string

const (
	// KindStateValue: the active state configuration changed.
	KindStateValue Kind = "state_value"

	// KindContextField: a field in the marshaled context JSON changed.
	KindContextField Kind = "context_field"

	// KindStatus: actor status differs (running / done / stopped).
	KindStatus Kind = "status"

	// KindContextShape: the contexts have structurally different JSON shapes (e.g. different field sets at some nesting level).
	KindContextShape Kind = "context_shape"
)

// Entry is a single line of difference between two snapshots.
type Entry struct {
	Kind  Kind   `json:"kind"`
	Field string `json:"field,omitempty"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// String returns a one-line human-readable form.
func (d Entry) String() string {
	if d.Field != "" {
		return fmt.Sprintf("%s %s: %s → %s", d.Kind, d.Field, d.From, d.To)
	}
	return fmt.Sprintf("%s: %s → %s", d.Kind, d.From, d.To)
}

// Result is the outcome of comparing two snapshots.
type Result struct {
	Entries []Entry `json:"entries"`
}

// Empty reports whether the snapshots are equivalent.
func (d Result) Empty() bool { return len(d.Entries) == 0 }

// Strings returns each entry's String form, sorted for stable output.
func (d Result) Strings() []string {
	out := make([]string, 0, len(d.Entries))
	for _, e := range d.Entries {
		out = append(out, e.String())
	}
	sort.Strings(out)
	return out
}

// Snapshots computes the structural diff between two Snapshots of the same context type.
func Snapshots[Ctx any](left, right fate.Snapshot[Ctx]) Result {
	var out Result

	if leftPath, rightPath := left.Value.Path(), right.Value.Path(); leftPath != rightPath {
		out.Entries = append(out.Entries, Entry{
			Kind: KindStateValue,
			From: leftPath,
			To:   rightPath,
		})
	}

	if string(left.Status) != string(right.Status) {
		out.Entries = append(out.Entries, Entry{
			Kind: KindStatus,
			From: string(left.Status),
			To:   string(right.Status),
		})
	}

	leftBytes, lerr := json.Marshal(left.Context)
	rightBytes, rerr := json.Marshal(right.Context)
	if lerr != nil || rerr != nil {
		out.Entries = append(out.Entries, Entry{
			Kind: KindContextShape,
			From: errString(lerr),
			To:   errString(rerr),
		})
		return out
	}
	var leftAny, rightAny any
	if err := json.Unmarshal(leftBytes, &leftAny); err != nil {
		leftAny = string(leftBytes)
	}
	if err := json.Unmarshal(rightBytes, &rightAny); err != nil {
		rightAny = string(rightBytes)
	}
	walkContextDiff("", leftAny, rightAny, &out)
	return out
}

func walkContextDiff(field string, left, right any, out *Result) {
	switch l := left.(type) {
	case map[string]any:
		r, ok := right.(map[string]any)
		if !ok {
			out.Entries = append(out.Entries, Entry{
				Kind:  KindContextShape,
				Field: field,
				From:  encodeJSON(left),
				To:    encodeJSON(right),
			})
			return
		}
		keys := unionStringSet(mapKeys(l), mapKeys(r))
		sort.Strings(keys)
		for _, k := range keys {
			lv, lok := l[k]
			rv, rok := r[k]
			child := joinField(field, k)
			if !lok || !rok {
				out.Entries = append(out.Entries, Entry{
					Kind:  KindContextField,
					Field: child,
					From:  presence(lok, lv),
					To:    presence(rok, rv),
				})
				continue
			}
			walkContextDiff(child, lv, rv, out)
		}
	case []any:
		r, ok := right.([]any)
		if !ok {
			out.Entries = append(out.Entries, Entry{
				Kind:  KindContextShape,
				Field: field,
				From:  encodeJSON(left),
				To:    encodeJSON(right),
			})
			return
		}
		if len(l) != len(r) {
			out.Entries = append(out.Entries, Entry{
				Kind:  KindContextField,
				Field: field + ".length",
				From:  fmt.Sprintf("%d", len(l)),
				To:    fmt.Sprintf("%d", len(r)),
			})
		}
		n := len(l)
		if len(r) < n {
			n = len(r)
		}
		for i := 0; i < n; i++ {
			walkContextDiff(fmt.Sprintf("%s[%d]", field, i), l[i], r[i], out)
		}
	default:
		le := encodeJSON(left)
		re := encodeJSON(right)
		if le != re {
			out.Entries = append(out.Entries, Entry{
				Kind:  KindContextField,
				Field: field,
				From:  le,
				To:    re,
			})
		}
	}
}

func encodeJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("<encode error: %v>", err)
	}
	return string(b)
}

func presence(ok bool, v any) string {
	if !ok {
		return "<missing>"
	}
	return encodeJSON(v)
}

func errString(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}

func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func unionStringSet(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	for _, s := range a {
		seen[s] = struct{}{}
	}
	for _, s := range b {
		seen[s] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	return out
}

func joinField(prefix, segment string) string {
	if prefix == "" {
		return segment
	}
	return prefix + "." + segment
}
