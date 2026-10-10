package engine_test

import (
	"context"
	"testing"

	"github.com/arisros/fate/action"
	"github.com/arisros/fate/engine"
	"github.com/arisros/fate/persist"
)

type doneCtx struct {
	Approved bool
	Joined   int
}

type (
	doneState = engine.StateNodeConfig[doneCtx, string]
	doneTrans = engine.TransitionConfig[doneCtx, string]
)

func doneRegion(event string) doneState {
	return doneState{Initial: "open", States: map[string]doneState{
		"open": {On: map[string][]doneTrans{event: {{Target: "closed"}}}},
		"closed": {Type: engine.NodeFinal, Output: func(doneCtx) any {
			return event
		}},
	}}
}

func doneActor(t *testing.T, states map[string]doneState, events ...string) *engine.Actor[doneCtx, string] {
	t.Helper()
	m, err := engine.CreateMachine(engine.MachineConfig[doneCtx, string]{
		ID: "review", Initial: "review", States: states,
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	a := engine.NewActor(m)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	for _, e := range events {
		if err := a.Send(context.Background(), e); err != nil {
			t.Fatalf("Send %s: %v", e, err)
		}
	}
	return a
}

func joinStates(onDone ...doneTrans) map[string]doneState {
	return map[string]doneState{
		"review": {
			Type:   engine.NodeParallel,
			States: map[string]doneState{"book": doneRegion("BOOK"), "ux": doneRegion("UX")},
			OnDone: onDone,
		},
		"joined":   {},
		"rejected": {},
	}
}

func TestParallelOnDone_JoinsWhenEveryRegionIsFinal(t *testing.T) {
	count := action.Assign(func(c doneCtx, _ string) doneCtx { c.Joined++; return c })
	for _, order := range [][]string{{"BOOK", "UX"}, {"UX", "BOOK"}} {
		a := doneActor(t, joinStates(doneTrans{Target: "joined", Actions: []action.Action[doneCtx, string]{count}}), order...)
		snap := a.Snapshot()
		if got := snap.Value.Path(); got != "joined" {
			t.Errorf("%v: value %q, want %q", order, got, "joined")
		}
		if snap.Context.Joined != 1 {
			t.Errorf("%v: OnDone ran %d times, want 1", order, snap.Context.Joined)
		}
	}
}

func TestParallelOnDone_WaitsForEveryRegion(t *testing.T) {
	for event, want := range map[string]string{
		"BOOK": "review.book.closed | review.ux.open",
		"UX":   "review.book.open | review.ux.closed",
	} {
		snap := doneActor(t, joinStates(doneTrans{Target: "joined"}), event).Snapshot()
		if got := snap.Value.Path(); got != want {
			t.Errorf("%s: value %q, want %q", event, got, want)
		}
		if snap.Status != persist.StatusRunning {
			t.Errorf("%s: status %q, want running", event, snap.Status)
		}
	}
}

func TestParallelOnDone_TakesFirstPassingCandidate(t *testing.T) {
	approved := func(c doneCtx, _ string) bool { return c.Approved }
	a := doneActor(t, joinStates(
		doneTrans{Target: "joined", Guard: approved},
		doneTrans{Target: "rejected"},
	), "BOOK", "UX")
	if got := a.Snapshot().Value.Path(); got != "rejected" {
		t.Errorf("value %q, want %q", got, "rejected")
	}
}

func TestParallelOnDone_SurvivesPersistBetweenRegions(t *testing.T) {
	states := joinStates(doneTrans{Target: "joined"})
	a := doneActor(t, states, "UX")
	raw, err := a.Persist()
	if err != nil {
		t.Fatalf("Persist: %v", err)
	}
	m, err := engine.CreateMachine(engine.MachineConfig[doneCtx, string]{ID: "review", Initial: "review", States: states})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	restored, err := engine.NewActorFromSnapshot[doneCtx, string](m, raw)
	if err != nil {
		t.Fatalf("NewActorFromSnapshot: %v", err)
	}
	_ = restored.Send(context.Background(), "BOOK")
	if got := restored.Snapshot().Value.Path(); got != "joined" {
		t.Errorf("value %q, want %q", got, "joined")
	}
}

func TestParallelDone_TopLevelCompletesTheActor(t *testing.T) {
	a := doneActor(t, joinStates(), "BOOK")
	if got := a.Snapshot().Status; got != persist.StatusRunning {
		t.Fatalf("one region final: status %q, want running", got)
	}
	_ = a.Send(context.Background(), "UX")
	snap := a.Snapshot()
	if snap.Status != persist.StatusDone {
		t.Errorf("status %q, want done", snap.Status)
	}
	if got := string(snap.Output); got != `"BOOK"` {
		t.Errorf("output %s, want the first region's", got)
	}
}

func TestParallelDone_NestedParallelRegion(t *testing.T) {
	states := map[string]doneState{
		"review": {
			Type: engine.NodeParallel,
			States: map[string]doneState{
				"docs": {Type: engine.NodeParallel, States: map[string]doneState{
					"book": doneRegion("BOOK"), "slip": doneRegion("SLIP"),
				}},
				"ux": doneRegion("UX"),
			},
			OnDone: []doneTrans{{Target: "joined"}},
		},
		"joined": {},
	}
	a := doneActor(t, states, "UX", "SLIP")
	if got := a.Snapshot().Value.Path(); got == "joined" {
		t.Fatalf("joined before the nested region finished")
	}
	_ = a.Send(context.Background(), "BOOK")
	if got := a.Snapshot().Value.Path(); got != "joined" {
		t.Errorf("value %q, want %q", got, "joined")
	}
}

func TestCompoundOnDone_FiresInEveryRegion(t *testing.T) {
	step := func(event string) doneState {
		return doneState{Initial: "step", States: map[string]doneState{
			"step": {
				Initial: "open",
				OnDone:  []doneTrans{{Target: "after"}},
				States: map[string]doneState{
					"open":   {On: map[string][]doneTrans{event: {{Target: "closed"}}}},
					"closed": {Type: engine.NodeFinal},
				},
			},
			"after": {},
		}}
	}
	states := map[string]doneState{
		"review": {Type: engine.NodeParallel, States: map[string]doneState{"book": step("BOOK"), "ux": step("UX")}},
	}
	for _, tc := range []struct {
		events []string
		want   string
	}{
		{[]string{"BOOK"}, "review.book.after | review.ux.step.open"},
		{[]string{"UX"}, "review.book.step.open | review.ux.after"},
		{[]string{"UX", "BOOK"}, "review.book.after | review.ux.after"},
	} {
		if got := doneActor(t, states, tc.events...).Snapshot().Value.Path(); got != tc.want {
			t.Errorf("%v: value %q, want %q", tc.events, got, tc.want)
		}
	}
}
