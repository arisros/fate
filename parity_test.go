package fate_test

import (
	"context"
	"testing"
	"time"

	fate "github.com/arisros/fate"
)

// fireAll drives every pending timer on the actor, the way an adapter would
// once it decides the delays have elapsed. Returns how many it fired.
func fireAll(a *fate.Actor[afCtx, afEvt]) int {
	pending := a.PendingTimers()
	for _, pt := range pending {
		a.FireTimer(pt.ID)
	}
	return len(pending)
}

// --- after / delayed transitions (pull-driven, clock-agnostic core) ---

type afCtx struct{ Ticks int }
type afEvt interface{ isAf() }
type afGo struct{}

func (afGo) isAf() {}

func afterMachine(t *testing.T) *fate.Machine[afCtx, afEvt] {
	t.Helper()
	m, err := fate.CreateMachine(fate.MachineConfig[afCtx, afEvt]{
		ID:      "after",
		Initial: "a",
		States: map[string]fate.StateNodeConfig[afCtx, afEvt]{
			"a": {
				On: map[string][]fate.TransitionConfig[afCtx, afEvt]{
					"afGo": {{Target: "c"}},
				},
				After: map[time.Duration][]fate.TransitionConfig[afCtx, afEvt]{
					10 * time.Millisecond: {{
						Target:  "b",
						Actions: []fate.Action[afCtx, afEvt]{fate.Assign(func(c afCtx, _ afEvt) afCtx { c.Ticks++; return c })},
					}},
				},
			},
			"b": {Type: fate.NodeFinal},
			"c": {},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return m
}

func TestAfterFiresWhenDriven(t *testing.T) {
	a := fate.NewActor(afterMachine(t))
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !a.Snapshot().Matches("a") {
		t.Fatalf("want a, got %s", a.Snapshot().Value.Path())
	}
	// The core never fires on its own: state stays put with no driver.
	pending := a.PendingTimers()
	if len(pending) != 1 || pending[0].Delay != 10*time.Millisecond {
		t.Fatalf("want one 10ms pending timer, got %+v", pending)
	}
	if !a.Snapshot().Matches("a") {
		t.Fatalf("clock-agnostic core must not self-fire, got %s", a.Snapshot().Value.Path())
	}
	// An adapter decides the delay elapsed and fires it.
	a.FireTimer(pending[0].ID)
	snap := a.Snapshot()
	if !snap.Matches("b") {
		t.Fatalf("want b after firing, got %s", snap.Value.Path())
	}
	if snap.Status != fate.StatusDone {
		t.Fatalf("want Done (b is final), got %s", snap.Status)
	}
	if snap.Context.Ticks != 1 {
		t.Fatalf("after action should have run once, ticks=%d", snap.Context.Ticks)
	}
	if n := len(a.PendingTimers()); n != 0 {
		t.Fatalf("want 0 pending after fire, got %d", n)
	}
}

func TestAfterCancelledOnEarlyExit(t *testing.T) {
	a := fate.NewActor(afterMachine(t))
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Leave "a" via event before any driver fires the timer.
	if err := a.Send(context.Background(), afGo{}); err != nil {
		t.Fatal(err)
	}
	if !a.Snapshot().Matches("c") {
		t.Fatalf("want c, got %s", a.Snapshot().Value.Path())
	}
	// The timer is gone: nothing left for an adapter to fire.
	if n := len(a.PendingTimers()); n != 0 {
		t.Fatalf("exiting the state must disarm its timer, still %d pending", n)
	}
	if n := fireAll(a); n != 0 {
		t.Fatalf("no timers should fire after exit, fired %d", n)
	}
	if !a.Snapshot().Matches("c") {
		t.Fatalf("want c, got %s", a.Snapshot().Value.Path())
	}
}

func TestAfterReArmsOnReEntry(t *testing.T) {
	// An external self-transition re-enters the state, which re-arms its timer
	// under the same deterministic ID.
	m, err := fate.CreateMachine(fate.MachineConfig[afCtx, afEvt]{
		ID:      "rearm",
		Initial: "wait",
		States: map[string]fate.StateNodeConfig[afCtx, afEvt]{
			"wait": {
				On: map[string][]fate.TransitionConfig[afCtx, afEvt]{
					"afGo": {{Target: "wait"}}, // external self-transition: exit+re-enter
				},
				After: map[time.Duration][]fate.TransitionConfig[afCtx, afEvt]{
					10 * time.Millisecond: {{Target: "done"}},
				},
			},
			"done": {Type: fate.NodeFinal},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	a := fate.NewActor(m)
	_ = a.Start(context.Background())
	first := a.PendingTimers()
	if len(first) != 1 {
		t.Fatalf("want one armed timer, got %d", len(first))
	}
	_ = a.Send(context.Background(), afGo{}) // re-enter wait
	again := a.PendingTimers()
	if len(again) != 1 || again[0].ID != first[0].ID {
		t.Fatalf("re-entry should re-arm the same timer ID, got %+v", again)
	}
	if !a.Snapshot().Matches("wait") {
		t.Fatalf("want wait, got %s", a.Snapshot().Value.Path())
	}
	a.FireTimer(again[0].ID)
	if !a.Snapshot().Matches("done") {
		t.Fatalf("want done after firing, got %s", a.Snapshot().Value.Path())
	}
}

// --- Cond / StateIn over a parallel configuration ---

type condCtx struct{}
type condEvt interface{ isCond() }
type condAdvance struct{}
type condCheck struct{}

func (condAdvance) isCond() {}
func (condCheck) isCond()   {}

func TestCondStateInGatesTransition(t *testing.T) {
	m, err := fate.CreateMachine(fate.MachineConfig[condCtx, condEvt]{
		ID:      "root",
		Initial: "par",
		States: map[string]fate.StateNodeConfig[condCtx, condEvt]{
			"par": {
				Type: fate.NodeParallel,
				States: map[string]fate.StateNodeConfig[condCtx, condEvt]{
					"r1": {
						Initial: "x",
						States: map[string]fate.StateNodeConfig[condCtx, condEvt]{
							"x": {On: map[string][]fate.TransitionConfig[condCtx, condEvt]{
								"condAdvance": {{Target: "y"}},
							}},
							"y": {},
						},
					},
					"r2": {
						Initial: "p",
						States: map[string]fate.StateNodeConfig[condCtx, condEvt]{
							"p": {On: map[string][]fate.TransitionConfig[condCtx, condEvt]{
								// Only reach q when r1 is already in y.
								"condCheck": {
									{Target: "q", Cond: fate.StateIn("par.r1.y")},
									{Target: "blocked"},
								},
							}},
							"q":       {},
							"blocked": {},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	a := fate.NewActor(m)
	_ = a.Start(context.Background())

	// r1 still in x: condCheck should fall through to "blocked".
	_ = a.Send(context.Background(), condCheck{})
	if !a.Snapshot().Matches("par.r2.blocked") {
		t.Fatalf("want r2 blocked while r1 in x, got %s", a.Snapshot().Value.Path())
	}

	// Move r1 to y, restart r2 path via a fresh machine to test the positive case.
	a2 := fate.NewActor(m)
	_ = a2.Start(context.Background())
	_ = a2.Send(context.Background(), condAdvance{}) // r1 -> y
	_ = a2.Send(context.Background(), condCheck{})   // r2: InState(par.r1.y) holds -> q
	if !a2.Snapshot().Matches("par.r2.q") {
		t.Fatalf("want r2 q once r1 in y, got %s", a2.Snapshot().Value.Path())
	}
}

// --- Setup builder ---

type suCtx struct{ N int }
type suEvt interface{ isSu() }
type suInc struct{}

func (suInc) isSu() {}

func TestSetupResolvesNamedGuardsAndActions(t *testing.T) {
	s := fate.NewSetup[suCtx, suEvt]().
		WithGuard("underTen", func(c suCtx, _ suEvt) bool { return c.N < 10 }).
		WithAction("inc", fate.Assign(func(c suCtx, _ suEvt) suCtx { c.N++; return c }))

	m, err := s.CreateMachine(fate.MachineConfig[suCtx, suEvt]{
		ID:      "counter",
		Initial: "active",
		States: map[string]fate.StateNodeConfig[suCtx, suEvt]{
			"active": {On: map[string][]fate.TransitionConfig[suCtx, suEvt]{
				"suInc": {{
					Guard:   s.Guard("underTen"),
					Actions: []fate.Action[suCtx, suEvt]{s.Action("inc")},
				}},
			}},
		},
	})
	if err != nil {
		t.Fatalf("setup create: %v", err)
	}
	a := fate.NewActor(m)
	_ = a.Start(context.Background())
	for i := 0; i < 3; i++ {
		_ = a.Send(context.Background(), suInc{})
	}
	if got := a.Snapshot().Context.N; got != 3 {
		t.Fatalf("want N=3, got %d", got)
	}
}

func TestSetupReportsMissingReferences(t *testing.T) {
	s := fate.NewSetup[suCtx, suEvt]()
	cfg := fate.MachineConfig[suCtx, suEvt]{
		ID:      "x",
		Initial: "a",
		States: map[string]fate.StateNodeConfig[suCtx, suEvt]{
			"a": {On: map[string][]fate.TransitionConfig[suCtx, suEvt]{
				"suInc": {{Guard: s.Guard("ghostGuard")}},
			}},
		},
	}
	if _, err := s.CreateMachine(cfg); err == nil {
		t.Fatal("expected error for unregistered guard reference")
	}
}
