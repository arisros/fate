package engine_test

import (
	"context"
	"strings"
	"testing"

	"github.com/arisros/fate/action"
	"github.com/arisros/fate/engine"
)

type engineCtx struct {
	Count   int
	Trail   []string
	Allowed bool
}

type engineEvt interface{ isEngineEvt() }

type evtA struct{}
type evtB struct{}
type evtC struct{}
type evtRaiseB struct{}
type evtRaiseChain struct{ N int }

func (evtA) isEngineEvt()          {}
func (evtB) isEngineEvt()          {}
func (evtC) isEngineEvt()          {}
func (evtRaiseB) isEngineEvt()     {}
func (evtRaiseChain) isEngineEvt() {}

func (evtA) EventName() string          { return "A" }
func (evtB) EventName() string          { return "B" }
func (evtC) EventName() string          { return "C" }
func (evtRaiseB) EventName() string     { return "RAISE_B" }
func (evtRaiseChain) EventName() string { return "RAISE_CHAIN" }

func appendTrail(tag string) action.Action[engineCtx, engineEvt] {
	return action.Assign(func(c engineCtx, _ engineEvt) engineCtx {
		c.Trail = append(c.Trail, tag)
		return c
	})
}

func TestEngine_AssignUpdatesContext(t *testing.T) {
	m, err := engine.CreateMachine(engine.MachineConfig[engineCtx, engineEvt]{
		ID:      "assign",
		Initial: "x",
		States: map[string]engine.StateNodeConfig[engineCtx, engineEvt]{
			"x": {
				On: map[string][]engine.TransitionConfig[engineCtx, engineEvt]{
					"A": {{Target: "x", Actions: []action.Action[engineCtx, engineEvt]{
						action.Assign(func(c engineCtx, _ engineEvt) engineCtx {
							c.Count++
							return c
						}),
					}}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	a := engine.NewActor(m)
	_ = a.Start(context.Background())
	for range 5 {
		if err := a.Send(context.Background(), evtA{}); err != nil {
			t.Fatalf("Send: %v", err)
		}
	}
	if got := a.Snapshot().Context.Count; got != 5 {
		t.Errorf("Count: got %d want 5", got)
	}
}

func TestEngine_EntryExitActionOrder(t *testing.T) {
	m, _ := engine.CreateMachine(engine.MachineConfig[engineCtx, engineEvt]{
		ID:      "order",
		Initial: "a",
		States: map[string]engine.StateNodeConfig[engineCtx, engineEvt]{
			"a": {
				Entry: []action.Action[engineCtx, engineEvt]{appendTrail("enter:a")},
				Exit:  []action.Action[engineCtx, engineEvt]{appendTrail("exit:a")},
				On: map[string][]engine.TransitionConfig[engineCtx, engineEvt]{
					"A": {{Target: "b", Actions: []action.Action[engineCtx, engineEvt]{appendTrail("trans:a->b")}}},
				},
			},
			"b": {
				Entry: []action.Action[engineCtx, engineEvt]{appendTrail("enter:b")},
				Exit:  []action.Action[engineCtx, engineEvt]{appendTrail("exit:b")},
			},
		},
	})
	a := engine.NewActor(m)
	_ = a.Start(context.Background())
	if err := a.Send(context.Background(), evtA{}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	want := []string{"enter:a", "exit:a", "trans:a->b", "enter:b"}
	got := a.Snapshot().Context.Trail
	if !equalStrings(got, want) {
		t.Errorf("trail: got %v want %v", got, want)
	}
}

func TestEngine_TargetlessTransitionRunsActionsOnly(t *testing.T) {
	m, _ := engine.CreateMachine(engine.MachineConfig[engineCtx, engineEvt]{
		ID:      "targetless",
		Initial: "x",
		States: map[string]engine.StateNodeConfig[engineCtx, engineEvt]{
			"x": {
				Entry: []action.Action[engineCtx, engineEvt]{appendTrail("enter:x")},
				Exit:  []action.Action[engineCtx, engineEvt]{appendTrail("exit:x")},
				On: map[string][]engine.TransitionConfig[engineCtx, engineEvt]{
					"A": {{Actions: []action.Action[engineCtx, engineEvt]{appendTrail("trans:A")}}},
				},
			},
		},
	})
	a := engine.NewActor(m)
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), evtA{})
	want := []string{"enter:x", "trans:A"}
	if got := a.Snapshot().Context.Trail; !equalStrings(got, want) {
		t.Errorf("targetless trail: got %v want %v", got, want)
	}
}

func TestEngine_InternalTransitionKeepsSourceEntered(t *testing.T) {
	m, _ := engine.CreateMachine(engine.MachineConfig[engineCtx, engineEvt]{
		ID:      "internal",
		Initial: "parent",
		States: map[string]engine.StateNodeConfig[engineCtx, engineEvt]{
			"parent": {
				Initial: "child",
				Entry:   []action.Action[engineCtx, engineEvt]{appendTrail("enter:parent")},
				Exit:    []action.Action[engineCtx, engineEvt]{appendTrail("exit:parent")},
				On: map[string][]engine.TransitionConfig[engineCtx, engineEvt]{
					"A": {{Target: "child", Internal: true, Actions: []action.Action[engineCtx, engineEvt]{appendTrail("trans:A")}}},
				},
				States: map[string]engine.StateNodeConfig[engineCtx, engineEvt]{
					"child": {
						Entry: []action.Action[engineCtx, engineEvt]{appendTrail("enter:child")},
						Exit:  []action.Action[engineCtx, engineEvt]{appendTrail("exit:child")},
					},
				},
			},
		},
	})
	a := engine.NewActor(m)
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), evtA{})
	want := []string{"enter:parent", "enter:child", "exit:child", "trans:A", "enter:child"}
	if got := a.Snapshot().Context.Trail; !equalStrings(got, want) {
		t.Errorf("internal trail: got %v want %v", got, want)
	}
	parentEnterCount := countEqual(a.Snapshot().Context.Trail, "enter:parent")
	parentExitCount := countEqual(a.Snapshot().Context.Trail, "exit:parent")
	if parentEnterCount != 1 || parentExitCount != 0 {
		t.Errorf("parent trail: enter=%d exit=%d want enter=1 exit=0", parentEnterCount, parentExitCount)
	}
}

func countEqual(xs []string, target string) int {
	n := 0
	for _, x := range xs {
		if x == target {
			n++
		}
	}
	return n
}

func TestEngine_GuardSelectsCandidate(t *testing.T) {
	m, _ := engine.CreateMachine(engine.MachineConfig[engineCtx, engineEvt]{
		ID:      "guarded",
		Initial: "x",
		Context: engineCtx{Allowed: false},
		States: map[string]engine.StateNodeConfig[engineCtx, engineEvt]{
			"x": {
				On: map[string][]engine.TransitionConfig[engineCtx, engineEvt]{
					"A": {
						{Target: "allowed", Guard: func(c engineCtx, _ engineEvt) bool { return c.Allowed }},
						{Target: "denied"},
					},
				},
			},
			"allowed": {},
			"denied":  {},
		},
	})

	a := engine.NewActor(m)
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), evtA{})
	if got := a.Snapshot().Value.Path(); got != "denied" {
		t.Errorf("denied: got %q want %q", got, "denied")
	}

	mt, _ := engine.CreateMachine(engine.MachineConfig[engineCtx, engineEvt]{
		ID:      "guarded3",
		Initial: "x",
		Context: engineCtx{Allowed: true},
		States: map[string]engine.StateNodeConfig[engineCtx, engineEvt]{
			"x": {
				On: map[string][]engine.TransitionConfig[engineCtx, engineEvt]{
					"A": {
						{Target: "allowed", Guard: func(c engineCtx, _ engineEvt) bool { return c.Allowed }},
						{Target: "denied"},
					},
				},
			},
			"allowed": {},
			"denied":  {},
		},
	})
	at := engine.NewActor(mt)
	_ = at.Start(context.Background())
	_ = at.Send(context.Background(), evtA{})
	if got := at.Snapshot().Value.Path(); got != "allowed" {
		t.Errorf("allowed: got %q want %q", got, "allowed")
	}
}

func TestEngine_GuardCombinators(t *testing.T) {
	always := action.AlwaysTrue[engineCtx, engineEvt]()
	never := action.Not(always)
	if got := action.And(always, never)(engineCtx{}, evtA{}); got {
		t.Error("And(always,never) should be false")
	}
	if got := action.Or(always, never)(engineCtx{}, evtA{}); !got {
		t.Error("Or(always,never) should be true")
	}
	if got := action.Not(always)(engineCtx{}, evtA{}); got {
		t.Error("Not(always) should be false")
	}
}

func TestEngine_RaiseProcessedBeforeSendReturns(t *testing.T) {
	m, _ := engine.CreateMachine(engine.MachineConfig[engineCtx, engineEvt]{
		ID:      "raise",
		Initial: "x",
		States: map[string]engine.StateNodeConfig[engineCtx, engineEvt]{
			"x": {
				On: map[string][]engine.TransitionConfig[engineCtx, engineEvt]{
					"RAISE_B": {{Target: "x", Actions: []action.Action[engineCtx, engineEvt]{
						action.Raise[engineCtx, engineEvt](evtB{}),
					}}},
					"B": {{Target: "y", Actions: []action.Action[engineCtx, engineEvt]{appendTrail("got:B")}}},
				},
			},
			"y": {},
		},
	})
	a := engine.NewActor(m)
	_ = a.Start(context.Background())
	if err := a.Send(context.Background(), evtRaiseB{}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := a.Snapshot().Value.Path(); got != "y" {
		t.Errorf("after raise: got %q want %q", got, "y")
	}
	if got := a.Snapshot().Context.Trail; len(got) != 1 || got[0] != "got:B" {
		t.Errorf("trail: got %v want [got:B]", got)
	}
}

func TestEngine_RaiseLoopDoesNotHang(t *testing.T) {
	m, _ := engine.CreateMachine(engine.MachineConfig[engineCtx, engineEvt]{
		ID:      "raiseloop",
		Initial: "x",
		States: map[string]engine.StateNodeConfig[engineCtx, engineEvt]{
			"x": {
				On: map[string][]engine.TransitionConfig[engineCtx, engineEvt]{
					"A": {{Target: "x", Actions: []action.Action[engineCtx, engineEvt]{
						action.Assign(func(c engineCtx, _ engineEvt) engineCtx {
							c.Count++
							return c
						}),
						action.Raise[engineCtx, engineEvt](evtA{}),
					}}},
				},
			},
		},
	})
	a := engine.NewActor(m)
	_ = a.Start(context.Background())

	logged := []string{}
	a2 := engine.NewActor(m, engine.WithLogger(func(s string) { logged = append(logged, s) }))
	_ = a2.Start(context.Background())
	if err := a2.Send(context.Background(), evtA{}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := a2.Snapshot().Context.Count; got < 100 {
		t.Errorf("Count: got %d, expected high count after raise loop", got)
	}
	found := false
	for _, msg := range logged {
		if strings.Contains(msg, "drain cap") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected drain-cap warning in logger, got %v", logged)
	}
}

func TestEngine_EnqueueActionsBatches(t *testing.T) {
	m, _ := engine.CreateMachine(engine.MachineConfig[engineCtx, engineEvt]{
		ID:      "enq",
		Initial: "x",
		States: map[string]engine.StateNodeConfig[engineCtx, engineEvt]{
			"x": {
				On: map[string][]engine.TransitionConfig[engineCtx, engineEvt]{
					"A": {{Target: "x", Actions: []action.Action[engineCtx, engineEvt]{
						action.EnqueueActions(func(enq *action.Enqueuer[engineCtx, engineEvt]) {
							enq.Assign(func(c engineCtx, _ engineEvt) engineCtx { c.Count++; return c })
							enq.Assign(func(c engineCtx, _ engineEvt) engineCtx { c.Count += 10; return c })
							enq.Raise(evtB{})
						}),
					}}},
					"B": {{Target: "y"}},
				},
			},
			"y": {},
		},
	})
	a := engine.NewActor(m)
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), evtA{})
	if got := a.Snapshot().Value.Path(); got != "y" {
		t.Errorf("after enqueue raise: got %q want %q", got, "y")
	}
	if got := a.Snapshot().Context.Count; got != 11 {
		t.Errorf("count after batch: got %d want 11", got)
	}
}

func TestEngine_WildcardHandler(t *testing.T) {
	m, _ := engine.CreateMachine(engine.MachineConfig[engineCtx, engineEvt]{
		ID:      "wild",
		Initial: "x",
		States: map[string]engine.StateNodeConfig[engineCtx, engineEvt]{
			"x": {
				On: map[string][]engine.TransitionConfig[engineCtx, engineEvt]{
					"*": {{Target: "y"}},
				},
			},
			"y": {},
		},
	})
	a := engine.NewActor(m)
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), evtC{})
	if got := a.Snapshot().Value.Path(); got != "y" {
		t.Errorf("after wildcard: got %q want %q", got, "y")
	}
}

func TestEngine_EventBubblesToAncestor(t *testing.T) {
	m, _ := engine.CreateMachine(engine.MachineConfig[engineCtx, engineEvt]{
		ID:      "bubble",
		Initial: "outer",
		States: map[string]engine.StateNodeConfig[engineCtx, engineEvt]{
			"outer": {
				Initial: "inner",
				On: map[string][]engine.TransitionConfig[engineCtx, engineEvt]{
					"A": {{Target: "done"}},
				},
				States: map[string]engine.StateNodeConfig[engineCtx, engineEvt]{
					"inner": {},
				},
			},
			"done": {},
		},
	})
	a := engine.NewActor(m)
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), evtA{})
	if got := a.Snapshot().Value.Path(); got != "done" {
		t.Errorf("after bubble: got %q want %q", got, "done")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
