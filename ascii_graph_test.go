package fate_test

// ASCII graph rendering tests. Loose-shape (substring) assertions for now;
// the P7 studio will add golden-file tests for canonical layouts.

import (
	"strings"
	"testing"

	sc "github.com/arisros/fate"
)

type gCtx struct{}
type gEvt interface{ isGEvt() }
type gTick struct{}

func (gTick) isGEvt()           {}
func (gTick) EventName() string { return "TICK" }

func buildGraphFixture(t *testing.T) sc.MachineDescriptor {
	t.Helper()
	m, err := sc.CreateMachine(sc.MachineConfig[gCtx, gEvt]{
		ID:      "graph-fixture",
		Initial: "active",
		States: map[string]sc.StateNodeConfig[gCtx, gEvt]{
			"active": {
				Initial: "running",
				On: map[string][]sc.TransitionConfig[gCtx, gEvt]{
					"TICK": {{Target: "stopped"}},
				},
				States: map[string]sc.StateNodeConfig[gCtx, gEvt]{
					"running": {
						On: map[string][]sc.TransitionConfig[gCtx, gEvt]{
							"TICK": {{Target: "paused", Internal: true}},
						},
					},
					"paused": {},
				},
			},
			"stopped": {Type: sc.NodeFinal},
			"regions": {
				Type: sc.NodeParallel,
				States: map[string]sc.StateNodeConfig[gCtx, gEvt]{
					"a": {Initial: "a1", States: map[string]sc.StateNodeConfig[gCtx, gEvt]{"a1": {}}},
					"b": {Initial: "b1", States: map[string]sc.StateNodeConfig[gCtx, gEvt]{"b1": {}}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	return m.Describe()
}

func TestRenderASCII_IncludesHeaderAndInitialMarker(t *testing.T) {
	d := buildGraphFixture(t)
	out := sc.RenderASCII(d, sc.RenderOptions{})
	if !strings.Contains(out, "Machine: graph-fixture") {
		t.Errorf("missing header; got:\n%s", out)
	}
	if !strings.Contains(out, "(initial: active)") {
		t.Errorf("missing initial annotation; got:\n%s", out)
	}
	if !strings.Contains(out, "(initial)") {
		t.Errorf("missing per-state initial tag; got:\n%s", out)
	}
}

func TestRenderASCII_BracketsCompoundAndShowsFinal(t *testing.T) {
	d := buildGraphFixture(t)
	out := sc.RenderASCII(d, sc.RenderOptions{})
	if !strings.Contains(out, "┌─ active") {
		t.Errorf("compound open missing; got:\n%s", out)
	}
	if !strings.Contains(out, "└─ active") {
		t.Errorf("compound close missing; got:\n%s", out)
	}
	if !strings.Contains(out, "stopped  [final]") {
		t.Errorf("final tag missing; got:\n%s", out)
	}
}

func TestRenderASCII_ParallelRegionsHaveDividers(t *testing.T) {
	d := buildGraphFixture(t)
	out := sc.RenderASCII(d, sc.RenderOptions{})
	if !strings.Contains(out, "[parallel]") {
		t.Errorf("parallel tag missing; got:\n%s", out)
	}
	if !strings.Contains(out, "~~~") {
		t.Errorf("parallel-region divider missing; got:\n%s", out)
	}
}

func TestRenderASCII_HighlightAppearsOnMatchAndAncestor(t *testing.T) {
	d := buildGraphFixture(t)
	out := sc.RenderASCII(d, sc.RenderOptions{
		Highlight: map[string]rune{"active.running": '▶'},
	})
	if !strings.Contains(out, "▶ running") {
		t.Errorf("highlight missing on active.running; got:\n%s", out)
	}
	// Ancestor 'active' should also receive the marker via the prefix rule.
	if !strings.Contains(out, "▶ ┌─ active") {
		t.Errorf("ancestor highlight missing on active; got:\n%s", out)
	}
	// Sibling 'paused' should NOT be marked.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "paused") && strings.Contains(line, "▶") {
			t.Errorf("sibling paused incorrectly highlighted: %q", line)
		}
	}
}

func TestRenderTransitions_ListsEventsWithInternalTag(t *testing.T) {
	d := buildGraphFixture(t)
	got := sc.RenderTransitions(d, "active.running")
	if !strings.Contains(got, "TICK") {
		t.Errorf("TICK event missing; got:\n%s", got)
	}
	if !strings.Contains(got, "→ paused") {
		t.Errorf("target missing; got:\n%s", got)
	}
	if !strings.Contains(got, "{internal}") {
		t.Errorf("internal flag missing; got:\n%s", got)
	}
}

func TestRenderTransitions_NoneStateShowsNone(t *testing.T) {
	d := buildGraphFixture(t)
	got := sc.RenderTransitions(d, "stopped")
	if !strings.Contains(got, "(none)") {
		t.Errorf("expected '(none)' for stopped final; got:\n%s", got)
	}
}

func TestRenderTransitions_UnknownPathReturnsEmpty(t *testing.T) {
	d := buildGraphFixture(t)
	got := sc.RenderTransitions(d, "no.such.path")
	if got != "" {
		t.Errorf("unknown path: expected empty, got %q", got)
	}
}

func TestRenderASCII_DeterministicOutputAcrossRuns(t *testing.T) {
	d := buildGraphFixture(t)
	out1 := sc.RenderASCII(d, sc.RenderOptions{})
	out2 := sc.RenderASCII(d, sc.RenderOptions{})
	if out1 != out2 {
		t.Errorf("non-deterministic ASCII output across runs:\nrun1:\n%s\nrun2:\n%s", out1, out2)
	}
}
