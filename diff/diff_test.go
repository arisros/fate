package diff_test

import (
	"strings"
	"testing"

	sc "github.com/arisros/fate"
	"github.com/arisros/fate/diff"
)

type diffCtx struct {
	Stage string         `json:"stage"`
	Score int            `json:"score"`
	Flags map[string]any `json:"flags,omitempty"`
	Tags  []string       `json:"tags,omitempty"`
}

func snap(value sc.StateValue, status sc.ActorStatus, ctx diffCtx) sc.Snapshot[diffCtx] {
	return sc.Snapshot[diffCtx]{Value: value, Status: status, Context: ctx}
}

func TestSnapshots_EmptyWhenEqual(t *testing.T) {
	a := snap(sc.AtomicValue("active"), sc.StatusRunning, diffCtx{Stage: "pin", Score: 5})
	b := snap(sc.AtomicValue("active"), sc.StatusRunning, diffCtx{Stage: "pin", Score: 5})
	d := diff.Snapshots(a, b)
	if !d.Empty() {
		t.Errorf("expected empty diff; got %v", d.Strings())
	}
}

func TestSnapshots_StateValueDifference(t *testing.T) {
	a := snap(sc.AtomicValue("verif"), sc.StatusRunning, diffCtx{})
	b := snap(sc.AtomicValue("asset_doc"), sc.StatusRunning, diffCtx{})
	d := diff.Snapshots(a, b)
	if d.Empty() {
		t.Fatal("expected non-empty diff")
	}
	var found bool
	for _, e := range d.Entries {
		if e.Kind == diff.KindStateValue && e.From == "verif" && e.To == "asset_doc" {
			found = true
		}
	}
	if !found {
		t.Errorf("missing state-value diff entry; got: %v", d.Strings())
	}
}

func TestSnapshots_StatusDifference(t *testing.T) {
	a := snap(sc.AtomicValue("x"), sc.StatusRunning, diffCtx{})
	b := snap(sc.AtomicValue("x"), sc.StatusDone, diffCtx{})
	d := diff.Snapshots(a, b)
	var found bool
	for _, e := range d.Entries {
		if e.Kind == diff.KindStatus && e.From == "running" && e.To == "done" {
			found = true
		}
	}
	if !found {
		t.Errorf("missing status diff entry; got: %v", d.Strings())
	}
}

func TestSnapshots_ContextFieldDifference(t *testing.T) {
	a := snap(sc.AtomicValue("x"), sc.StatusRunning, diffCtx{Stage: "pin", Score: 5})
	b := snap(sc.AtomicValue("x"), sc.StatusRunning, diffCtx{Stage: "pin", Score: 9})
	d := diff.Snapshots(a, b)
	if d.Empty() {
		t.Fatal("expected non-empty diff")
	}
	var found bool
	for _, e := range d.Entries {
		if e.Kind == diff.KindContextField && e.Field == "score" && e.From == "5" && e.To == "9" {
			found = true
		}
	}
	if !found {
		t.Errorf("missing context-field diff entry for score; got: %v", d.Strings())
	}
}

func TestSnapshots_MissingFieldSurfacesAsContextField(t *testing.T) {
	a := snap(sc.AtomicValue("x"), sc.StatusRunning, diffCtx{Stage: "pin", Flags: map[string]any{"vip": true}})
	b := snap(sc.AtomicValue("x"), sc.StatusRunning, diffCtx{Stage: "pin"})
	d := diff.Snapshots(a, b)
	var found bool
	for _, e := range d.Entries {
		if e.Kind == diff.KindContextField && strings.HasPrefix(e.Field, "flags") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected context-field diff under flags; got: %v", d.Strings())
	}
}

func TestSnapshots_ArrayLengthDifference(t *testing.T) {
	a := snap(sc.AtomicValue("x"), sc.StatusRunning, diffCtx{Tags: []string{"a", "b"}})
	b := snap(sc.AtomicValue("x"), sc.StatusRunning, diffCtx{Tags: []string{"a", "b", "c"}})
	d := diff.Snapshots(a, b)
	var found bool
	for _, e := range d.Entries {
		if e.Kind == diff.KindContextField && e.Field == "tags.length" && e.From == "2" && e.To == "3" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected tags.length diff; got: %v", d.Strings())
	}
}

func TestSnapshots_StringsAreSortedAndDeterministic(t *testing.T) {
	a := snap(sc.AtomicValue("verif"), sc.StatusRunning, diffCtx{Stage: "pin", Score: 5})
	b := snap(sc.AtomicValue("asset_doc"), sc.StatusDone, diffCtx{Stage: "verif", Score: 9})
	d := diff.Snapshots(a, b)

	got := d.Strings()
	if len(got) < 3 {
		t.Fatalf("expected at least 3 diff entries; got %v", got)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Errorf("Strings() not sorted: %q > %q\nfull: %v", got[i-1], got[i], got)
			return
		}
	}
}
