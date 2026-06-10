package render_test

import (
	"strings"
	"testing"

	sc "github.com/arisros/fate"
	"github.com/arisros/fate/render"
)

type mCtx struct{}
type mEvt interface{ isMEvt() }
type mNext struct{}

func (mNext) isMEvt()           {}
func (mNext) EventName() string { return "NEXT" }

func mermaidDescriptor(t *testing.T, build func() (*sc.Machine[mCtx, mEvt], error)) sc.MachineDescriptor {
	t.Helper()
	m, err := build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return m.Describe()
}

func buildTraffic() (*sc.Machine[mCtx, mEvt], error) {
	return sc.CreateMachine(sc.MachineConfig[mCtx, mEvt]{
		ID:      "traffic-light",
		Initial: "red",
		States: map[string]sc.StateNodeConfig[mCtx, mEvt]{
			"red":    {On: map[string][]sc.TransitionConfig[mCtx, mEvt]{"NEXT": {{Target: "green"}}}},
			"green":  {On: map[string][]sc.TransitionConfig[mCtx, mEvt]{"NEXT": {{Target: "yellow"}}}},
			"yellow": {On: map[string][]sc.TransitionConfig[mCtx, mEvt]{"NEXT": {{Target: "red"}}}},
		},
	})
}

func buildParallelMermaid() (*sc.Machine[mCtx, mEvt], error) {
	region := func(initial string) sc.StateNodeConfig[mCtx, mEvt] {
		return sc.StateNodeConfig[mCtx, mEvt]{
			Initial: initial,
			States: map[string]sc.StateNodeConfig[mCtx, mEvt]{
				initial: {On: map[string][]sc.TransitionConfig[mCtx, mEvt]{"NEXT": {{Target: "done"}}}},
				"done":  {Type: sc.NodeFinal},
			},
		}
	}
	return sc.CreateMachine(sc.MachineConfig[mCtx, mEvt]{
		ID:      "para",
		Initial: "active",
		States: map[string]sc.StateNodeConfig[mCtx, mEvt]{
			"active": {
				Type: sc.NodeParallel,
				States: map[string]sc.StateNodeConfig[mCtx, mEvt]{
					"a": region("a1"),
					"b": region("b1"),
				},
			},
		},
	})
}

func TestMermaid_Header(t *testing.T) {
	d := mermaidDescriptor(t, buildTraffic)
	out := render.Mermaid(d, render.MermaidOptions{})
	if !strings.HasPrefix(out, "stateDiagram-v2") {
		t.Errorf("must start with stateDiagram-v2; got:\n%s", out)
	}
	if !strings.Contains(out, "direction TB") {
		t.Errorf("missing default direction TB")
	}
	if !strings.Contains(out, "[*] --> s_red") {
		t.Errorf("missing initial edge to red; got:\n%s", out)
	}
}

func TestMermaid_AtomicTransitions(t *testing.T) {
	d := mermaidDescriptor(t, buildTraffic)
	out := render.Mermaid(d, render.MermaidOptions{})
	for _, want := range []string{
		`state "red" as s_red`,
		`state "green" as s_green`,
		"s_red --> s_green : NEXT",
		"s_green --> s_yellow : NEXT",
		"s_yellow --> s_red : NEXT",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestMermaid_ParallelRegionsAndCollisionFreeIDs(t *testing.T) {
	d := mermaidDescriptor(t, buildParallelMermaid)
	out := render.Mermaid(d, render.MermaidOptions{})
	if !strings.Contains(out, "--\n") {
		t.Errorf("missing parallel region divider; got:\n%s", out)
	}
	if !strings.Contains(out, "s_active_a_done") || !strings.Contains(out, "s_active_b_done") {
		t.Errorf("region-qualified done ids missing; got:\n%s", out)
	}
	if !strings.Contains(out, `state "active" as s_active {`) {
		t.Errorf("missing parallel composite block; got:\n%s", out)
	}
	if !strings.Contains(out, "classDef final") {
		t.Errorf("missing final classDef; got:\n%s", out)
	}
}

func TestMermaid_ActiveHighlight(t *testing.T) {
	d := mermaidDescriptor(t, buildParallelMermaid)
	out := render.Mermaid(d, render.MermaidOptions{
		Highlight: map[string]rune{"active.a.a1": '>'},
	})
	if !strings.Contains(out, "classDef active") {
		t.Errorf("missing active classDef; got:\n%s", out)
	}
	for _, want := range []string{"s_active_a_a1", "s_active_a", "s_active"} {
		if !strings.Contains(out, want) {
			t.Errorf("active class missing id %q; got:\n%s", want, out)
		}
	}
}

func TestMermaid_GuardActionInternalLabels(t *testing.T) {
	d := sc.MachineDescriptor{
		ID:      "labels",
		Initial: "a",
		States: map[string]sc.StateNodeDescriptor{
			"a": {Type: "atomic", On: map[string][]sc.TransitionDescriptor{
				"NEXT": {{Target: "b", Guard: "isReady", Actions: []string{"bump"}}},
				"PING": {{Target: "a", Internal: true}},
			}},
			"b": {Type: "atomic"},
		},
	}
	out := render.Mermaid(d, render.MermaidOptions{})
	if !strings.Contains(out, "NEXT [isReady] / bump") {
		t.Errorf("guard+action label missing; got:\n%s", out)
	}
	if !strings.Contains(out, "PING (internal)") {
		t.Errorf("internal marker missing; got:\n%s", out)
	}
}

func TestMermaid_Deterministic(t *testing.T) {
	d := mermaidDescriptor(t, buildParallelMermaid)
	a := render.Mermaid(d, render.MermaidOptions{})
	b := render.Mermaid(d, render.MermaidOptions{})
	if a != b {
		t.Errorf("non-deterministic output")
	}
}
