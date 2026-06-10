package render_test

// ASCII graph rendering tests.

import (
	"strings"
	"testing"

	sc "github.com/arisros/fate"
	"github.com/arisros/fate/render"
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

func TestASCII_IncludesHeaderAndInitialMarker(t *testing.T) {
	d := buildGraphFixture(t)
	out := render.ASCII(d, render.Options{})
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

func TestASCII_BracketsCompoundAndShowsFinal(t *testing.T) {
	d := buildGraphFixture(t)
	out := render.ASCII(d, render.Options{})
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

func TestASCII_ParallelRegionsHaveDividers(t *testing.T) {
	d := buildGraphFixture(t)
	out := render.ASCII(d, render.Options{})
	if !strings.Contains(out, "[parallel]") {
		t.Errorf("parallel tag missing; got:\n%s", out)
	}
	if !strings.Contains(out, "~~~") {
		t.Errorf("parallel-region divider missing; got:\n%s", out)
	}
}

func TestASCII_HighlightAppearsOnMatchAndAncestor(t *testing.T) {
	d := buildGraphFixture(t)
	out := render.ASCII(d, render.Options{
		Highlight: map[string]rune{"active.running": '▶'},
	})
	if !strings.Contains(out, "▶ running") {
		t.Errorf("highlight missing on active.running; got:\n%s", out)
	}
	if !strings.Contains(out, "▶ ┌─ active") {
		t.Errorf("ancestor highlight missing on active; got:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "paused") && strings.Contains(line, "▶") {
			t.Errorf("sibling paused incorrectly highlighted: %q", line)
		}
	}
}

func TestTransitions_ListsEventsWithInternalTag(t *testing.T) {
	d := buildGraphFixture(t)
	got := render.Transitions(d, "active.running")
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

func TestTransitions_NoneStateShowsNone(t *testing.T) {
	d := buildGraphFixture(t)
	got := render.Transitions(d, "stopped")
	if !strings.Contains(got, "(none)") {
		t.Errorf("expected '(none)' for stopped final; got:\n%s", got)
	}
}

func TestTransitions_UnknownPathReturnsEmpty(t *testing.T) {
	d := buildGraphFixture(t)
	got := render.Transitions(d, "no.such.path")
	if got != "" {
		t.Errorf("unknown path: expected empty, got %q", got)
	}
}

func TestASCII_DeterministicOutputAcrossRuns(t *testing.T) {
	d := buildGraphFixture(t)
	out1 := render.ASCII(d, render.Options{})
	out2 := render.ASCII(d, render.Options{})
	if out1 != out2 {
		t.Errorf("non-deterministic ASCII output across runs:\nrun1:\n%s\nrun2:\n%s", out1, out2)
	}
}
