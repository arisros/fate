package fate_test

import (
	"context"
	"slices"
	"testing"
	"time"

	fate "github.com/arisros/fate"
)

type edCtx struct{ Log []string }

type (
	edS = fate.StateNodeConfig[edCtx, string]
	edT = fate.TransitionConfig[edCtx, string]
	edA = fate.Action[edCtx, string]
)

func edRec(label string) []edA {
	return []edA{fate.Assign(func(c edCtx, _ string) edCtx {
		c.Log = append(slices.Clone(c.Log), label)
		return c
	})}
}

func edStart(t *testing.T, cfg fate.MachineConfig[edCtx, string]) *fate.Actor[edCtx, string] {
	t.Helper()
	m, err := fate.CreateMachine(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a := fate.NewActor(m)
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	return a
}

func edSend(t *testing.T, a *fate.Actor[edCtx, string], e string) {
	t.Helper()
	if err := a.Send(context.Background(), e); err != nil {
		t.Fatal(err)
	}
}

func edTimers(a *fate.Actor[edCtx, string]) (out []string) {
	for _, p := range a.PendingTimers() {
		out = append(out, string(p.ID))
	}
	return out
}

func edInvokes(a *fate.Actor[edCtx, string]) (out []string) {
	for _, p := range a.PendingInvocations() {
		out = append(out, string(p.ID))
	}
	return out
}

var edAfter = map[time.Duration][]edT{time.Second: {{Target: "c2"}}}

// Passes on main, fails on 50f4885: an internal transition whose target is its
// own source has an empty chain, so enterBelow never runs, while commitValue
// resets the value to the initial child.
func TestEntry_InternalSelfTargetEntersInitialChild(t *testing.T) {
	a := edStart(t, fate.MachineConfig[edCtx, string]{
		ID: "m", Initial: "c",
		States: map[string]edS{
			"c": {
				Initial: "c1",
				On:      map[string][]edT{"RESET": {{Target: "c", Internal: true}}},
				States: map[string]edS{
					"c1": {Entry: edRec("+c1"), After: edAfter, On: map[string][]edT{"NEXT": {{Target: "c2"}}}},
					"c2": {},
				},
			},
		},
	})
	edSend(t, a, "NEXT")
	edSend(t, a, "RESET")

	snap := a.Snapshot()
	if !snap.Matches("c.c1") {
		t.Fatalf("value = %s, want c.c1", snap.Value.Path())
	}
	if got, want := snap.Context.Log, []string{"+c1", "+c1"}; !slices.Equal(got, want) {
		t.Errorf("entry log = %v, want %v", got, want)
	}
	if got := edTimers(a); len(got) != 1 {
		t.Errorf("c.c1 is active but PendingTimers = %v, want its 1s timer", got)
	}
}

// Same root cause on a parallel node: every region exits, none re-enters.
func TestEntry_InternalSelfTargetOnParallelReEntersRegions(t *testing.T) {
	a := edStart(t, fate.MachineConfig[edCtx, string]{
		ID: "m", Initial: "p",
		States: map[string]edS{
			"p": {
				Type: fate.NodeParallel,
				On:   map[string][]edT{"RESET": {{Target: "p", Internal: true}}},
				States: map[string]edS{
					"a": {Initial: "c1", States: map[string]edS{
						"c1": {After: edAfter},
						"c2": {},
					}},
					"b": {Initial: "b1", States: map[string]edS{
						"b1": {Invoke: []fate.Invocation[edCtx, string]{{ID: "i", Src: "x"}}},
					}},
				},
			},
		},
	})
	edSend(t, a, "RESET")

	snap := a.Snapshot()
	if !snap.Matches("p.a.c1") || !snap.Matches("p.b.b1") {
		t.Fatalf("value = %s", snap.Value.Path())
	}
	if len(edTimers(a)) != 1 || len(edInvokes(a)) != 1 {
		t.Errorf("value %s reports a.c1 and b.b1 active, but timers=%v invokes=%v", snap.Value.Path(), edTimers(a), edInvokes(a))
	}
}

// Deep history restoring into a parallel node: the entry set enters every
// sibling region at its *initial* state, while spliceValueAt restores the saved
// one. The wrong state's Entry runs, its effects are armed but can never be
// accepted, and the restored state's own effects are missing.
func TestEntry_DeepHistoryIntoParallelEntersTheRestoredStates(t *testing.T) {
	a := edStart(t, fate.MachineConfig[edCtx, string]{
		ID: "m", Initial: "app",
		States: map[string]edS{
			"app": {
				Initial: "player",
				On:      map[string][]edT{"AWAY": {{Target: "away"}}},
				States: map[string]edS{
					"hist": {Type: fate.NodeHistory, History: fate.HistoryDeep},
					"player": {
						Type: fate.NodeParallel,
						States: map[string]edS{
							"a": {Initial: "a1", States: map[string]edS{
								"a1": {On: map[string][]edT{"NA": {{Target: "a2"}}}},
								"a2": {Entry: edRec("+a2")},
							}},
							"b": {Initial: "b1", States: map[string]edS{
								"b1": {
									Entry:  edRec("+b1"),
									Invoke: []fate.Invocation[edCtx, string]{{ID: "i1", Src: "x"}},
									On:     map[string][]edT{"NB": {{Target: "b2"}}},
								},
								"b2": {
									Entry:  edRec("+b2"),
									Invoke: []fate.Invocation[edCtx, string]{{ID: "i2", Src: "x"}},
								},
							}},
						},
					},
				},
			},
			"away": {On: map[string][]edT{"BACK": {{Target: "app.hist"}}}},
		},
	})
	edSend(t, a, "NA")
	edSend(t, a, "NB")
	edSend(t, a, "AWAY")
	before := len(a.Snapshot().Context.Log)
	edSend(t, a, "BACK")

	snap := a.Snapshot()
	if !snap.Matches("app.player.a.a2") || !snap.Matches("app.player.b.b2") {
		t.Fatalf("value = %s, want a.a2 | b.b2", snap.Value.Path())
	}
	if got, want := snap.Context.Log[before:], []string{"+a2", "+b2"}; !slices.Equal(got, want) {
		t.Errorf("entry actions on restore = %v, want %v", got, want)
	}
	if got, want := edInvokes(a), []string{"app.player.b.b2#invoke#i2"}; !slices.Equal(got, want) {
		t.Errorf("PendingInvocations = %v, want %v", got, want)
	}
}

func TestEntry_RegionsInDocumentOrder(t *testing.T) {
	leaf := func(name string) edS { return edS{Entry: edRec("+" + name)} }
	region := func(name, initial string, states map[string]edS) edS {
		return edS{Entry: edRec("+" + name), Initial: initial, States: states}
	}
	a := edStart(t, fate.MachineConfig[edCtx, string]{
		ID: "m", Initial: "idle",
		States: map[string]edS{
			"idle": {On: map[string][]edT{"GO": {{Target: "p.a.a2"}}}},
			"p": {
				Type:  fate.NodeParallel,
				Entry: edRec("+p"),
				States: map[string]edS{
					"a": region("a", "a1", map[string]edS{"a1": leaf("a.a1"), "a2": leaf("a.a2")}),
					"b": region("b", "b1", map[string]edS{"b1": leaf("b.b1")}),
					"c": region("c", "c1", map[string]edS{"c1": leaf("c.c1")}),
				},
			},
		},
	})
	edSend(t, a, "GO")
	want := []string{"+p", "+a", "+a.a2", "+b", "+b.b1", "+c", "+c.c1"}
	if got := a.Snapshot().Context.Log; !slices.Equal(got, want) {
		t.Fatalf("entry order = %v, want %v", got, want)
	}
}
