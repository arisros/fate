package fate

// Snapshot diff for the P7 studio's diff view and for shadow-mode
// divergence triage in P8–P12. Pure-Go, no TUI deps.
//
// The diff is structural: it walks both state values and contexts in
// parallel and reports differences as a list of typed entries. Renderers
// (the studio's diff_view, log formatters, JSON serializers) consume this
// list — they don't compute it themselves.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// DiffKind enumerates the categories of difference DiffSnapshots may
// surface. Kept narrow on purpose; richer typing (e.g. "field added with
// value X") belongs to the renderer.
type DiffKind string

const (
	// DiffKindStateValue: the active state configuration changed. Examples:
	//   - left "active.main.verif", right "active.main.asset_doc"
	//   - left "running", right "done"
	DiffKindStateValue DiffKind = "state_value"

	// DiffKindContextField: a field in the marshaled context JSON changed.
	// Field is the dot-path of the leaf, From/To are the JSON-encoded
	// values (so the renderer can pretty-print or color them).
	DiffKindContextField DiffKind = "context_field"

	// DiffKindStatus: actor status differs (running / done / stopped).
	DiffKindStatus DiffKind = "status"

	// DiffKindContextShape: the contexts have structurally different
	// JSON shapes (e.g. different field sets at some nesting level).
	// Surfaced when a sub-tree on one side is an object and the other is
	// a primitive or null. Field is the dot-path.
	DiffKindContextShape DiffKind = "context_shape"
)

// DiffEntry is a single line of difference between two snapshots.
type DiffEntry struct {
	Kind  DiffKind `json:"kind"`
	Field string   `json:"field,omitempty"` // dot-path; empty for top-level kinds
	From  string   `json:"from"`            // left-hand JSON-encoded value
	To    string   `json:"to"`              // right-hand JSON-encoded value
}

// String returns a one-line human-readable form. Useful for log output and
// for the divergence-log formatter; the studio diff view uses the
// structured fields directly.
func (d DiffEntry) String() string {
	if d.Field != "" {
		return fmt.Sprintf("%s %s: %s → %s", d.Kind, d.Field, d.From, d.To)
	}
	return fmt.Sprintf("%s: %s → %s", d.Kind, d.From, d.To)
}

// SnapshotDiff is the result of comparing two snapshots.
type SnapshotDiff struct {
	Entries []DiffEntry `json:"entries"`
}

// Empty reports whether the snapshots are equivalent — no entries means
// the renderer should show "no differences" rather than an empty list.
func (d SnapshotDiff) Empty() bool { return len(d.Entries) == 0 }

// Strings returns each entry's String form, sorted for stable output. The
// studio's golden-file tests rely on this ordering.
func (d SnapshotDiff) Strings() []string {
	out := make([]string, 0, len(d.Entries))
	for _, e := range d.Entries {
		out = append(out, e.String())
	}
	sort.Strings(out)
	return out
}

// DiffSnapshots computes the structural diff between two Snapshots of the
// same context type. Order of arguments is left → right; produced entries
// are listed in a deterministic order suitable for golden-file testing.
//
// Context comparison serializes both sides to JSON and walks the resulting
// trees. Non-marshalable contexts surface a single DiffKindContextShape
// entry naming the cause.
func DiffSnapshots[Ctx any](left, right Snapshot[Ctx]) SnapshotDiff {
	var out SnapshotDiff

	if leftPath, rightPath := left.Value.Path(), right.Value.Path(); leftPath != rightPath {
		out.Entries = append(out.Entries, DiffEntry{
			Kind: DiffKindStateValue,
			From: leftPath,
			To:   rightPath,
		})
	}

	if string(left.Status) != string(right.Status) {
		out.Entries = append(out.Entries, DiffEntry{
			Kind: DiffKindStatus,
			From: string(left.Status),
			To:   string(right.Status),
		})
	}

	leftBytes, lerr := json.Marshal(left.Context)
	rightBytes, rerr := json.Marshal(right.Context)
	if lerr != nil || rerr != nil {
		out.Entries = append(out.Entries, DiffEntry{
			Kind: DiffKindContextShape,
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

// walkContextDiff recursively walks two unmarshaled JSON values. Field is
// the dot-path of the current cursor; "" at root.
func walkContextDiff(field string, left, right any, out *SnapshotDiff) {
	switch l := left.(type) {
	case map[string]any:
		r, ok := right.(map[string]any)
		if !ok {
			out.Entries = append(out.Entries, DiffEntry{
				Kind:  DiffKindContextShape,
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
				// One side missing the key: surface as a field-level
				// context diff with explicit "null"/"missing".
				out.Entries = append(out.Entries, DiffEntry{
					Kind:  DiffKindContextField,
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
			out.Entries = append(out.Entries, DiffEntry{
				Kind:  DiffKindContextShape,
				Field: field,
				From:  encodeJSON(left),
				To:    encodeJSON(right),
			})
			return
		}
		if len(l) != len(r) {
			out.Entries = append(out.Entries, DiffEntry{
				Kind:  DiffKindContextField,
				Field: field + ".length",
				From:  fmt.Sprintf("%d", len(l)),
				To:    fmt.Sprintf("%d", len(r)),
			})
		}
		// Compare prefix overlap element-by-element so a single inserted
		// value doesn't pollute every following index. For now keep
		// linear: position-wise. Renderer can collapse runs.
		n := len(l)
		if len(r) < n {
			n = len(r)
		}
		for i := 0; i < n; i++ {
			walkContextDiff(fmt.Sprintf("%s[%d]", field, i), l[i], r[i], out)
		}
	default:
		// Primitive: compare via JSON encoding so ints / floats / strings
		// / nulls / bools all render canonically.
		le := encodeJSON(left)
		re := encodeJSON(right)
		if le != re {
			out.Entries = append(out.Entries, DiffEntry{
				Kind:  DiffKindContextField,
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

// Marker var to keep strings import alive even if all helpers are inlined
// by future edits. The build hint is harmless and self-documenting.
var _ = strings.Builder{}
