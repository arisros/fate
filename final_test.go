package fate_test

import (
	"context"
	"testing"

	sc "github.com/arisros/fate"
)

// finalCtx + finalEvt — small example walking through a 3-step wizard.
type finalCtx struct {
	Stepped int
}

type finalEvt interface{ isFinalEvt() }

type evtNext struct{}
type evtRestart struct{}

func (evtNext) isFinalEvt()          {}
func (evtRestart) isFinalEvt()       {}
func (evtNext) EventName() string    { return "NEXT" }
func (evtRestart) EventName() string { return "RESTART" }

func TestFinal_TopLevelFinalSetsStatusDone(t *testing.T) {
	m, err := sc.CreateMachine(sc.MachineConfig[finalCtx, finalEvt]{
		ID:      "wizard-flat",
		Initial: "step1",
		States: map[string]sc.StateNodeConfig[finalCtx, finalEvt]{
			"step1": {
				On: map[string][]sc.TransitionConfig[finalCtx, finalEvt]{
					"NEXT": {{Target: "done"}},
				},
			},
			"done": {Type: sc.NodeFinal},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	a := sc.NewActor(m)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := a.Snapshot().Status; got != sc.StatusRunning {
		t.Fatalf("pre-final status: got %q want %q", got, sc.StatusRunning)
	}
	_ = a.Send(context.Background(), evtNext{})
	if got := a.Snapshot().Status; got != sc.StatusDone {
		t.Errorf("post-final status: got %q want %q", got, sc.StatusDone)
	}
	if got := a.Snapshot().Value.Path(); got != "done" {
		t.Errorf("value: got %q want %q", got, "done")
	}
	// Further sends should be silently dropped.
	if err := a.Send(context.Background(), evtRestart{}); err != nil {
		t.Errorf("Send after done: unexpected error %v", err)
	}
	if got := a.Snapshot().Status; got != sc.StatusDone {
		t.Errorf("status after post-done send: got %q want %q", got, sc.StatusDone)
	}
}

func TestFinal_CompoundOnDoneFires(t *testing.T) {
	// A compound `wizard` with three child steps; the third is final. When
	// the compound's child reaches final, onDone fires and transitions to
	// `completed`.
	m, err := sc.CreateMachine(sc.MachineConfig[finalCtx, finalEvt]{
		ID:      "wizard-compound",
		Initial: "wizard",
		States: map[string]sc.StateNodeConfig[finalCtx, finalEvt]{
			"wizard": {
				Initial: "s1",
				OnDone:  []sc.TransitionConfig[finalCtx, finalEvt]{{Target: "completed"}},
				States: map[string]sc.StateNodeConfig[finalCtx, finalEvt]{
					"s1": {On: map[string][]sc.TransitionConfig[finalCtx, finalEvt]{"NEXT": {{Target: "s2"}}}},
					"s2": {On: map[string][]sc.TransitionConfig[finalCtx, finalEvt]{"NEXT": {{Target: "s3"}}}},
					"s3": {Type: sc.NodeFinal},
				},
			},
			"completed": {},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	a := sc.NewActor(m)
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), evtNext{}) // s1 -> s2
	_ = a.Send(context.Background(), evtNext{}) // s2 -> s3 (final), onDone -> completed
	if got := a.Snapshot().Value.Path(); got != "completed" {
		t.Errorf("path: got %q want %q", got, "completed")
	}
	if got := a.Snapshot().Status; got != sc.StatusRunning {
		// `completed` is not final, so the actor stays Running.
		t.Errorf("status: got %q want %q", got, sc.StatusRunning)
	}
}

func TestFinal_CompoundOnDoneActions(t *testing.T) {
	// Verify onDone Actions run between exit-set and entry-set, with proper
	// ordering relative to entry/exit of the surrounding compound.
	// Reuses engineCtx/engineEvt from engine_test.go.
	m, err := sc.CreateMachine(sc.MachineConfig[engineCtx, engineEvt]{
		ID:      "wizard-actions",
		Initial: "wizard",
		States: map[string]sc.StateNodeConfig[engineCtx, engineEvt]{
			"wizard": {
				Initial: "s1",
				Entry:   []sc.Action[engineCtx, engineEvt]{appendTrail("enter:wizard")},
				Exit:    []sc.Action[engineCtx, engineEvt]{appendTrail("exit:wizard")},
				OnDone: []sc.TransitionConfig[engineCtx, engineEvt]{{
					Target:  "completed",
					Actions: []sc.Action[engineCtx, engineEvt]{appendTrail("ondone:wizard")},
				}},
				States: map[string]sc.StateNodeConfig[engineCtx, engineEvt]{
					"s1": {
						Entry: []sc.Action[engineCtx, engineEvt]{appendTrail("enter:s1")},
						Exit:  []sc.Action[engineCtx, engineEvt]{appendTrail("exit:s1")},
						On: map[string][]sc.TransitionConfig[engineCtx, engineEvt]{
							"A": {{Target: "s2"}},
						},
					},
					"s2": {
						Type:  sc.NodeFinal,
						Entry: []sc.Action[engineCtx, engineEvt]{appendTrail("enter:s2-final")},
					},
				},
			},
			"completed": {
				Entry: []sc.Action[engineCtx, engineEvt]{appendTrail("enter:completed")},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	a := sc.NewActor(m)
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), evtA{})

	want := []string{
		"enter:wizard", "enter:s1", // initial
		"exit:s1", "enter:s2-final", // s1 -> s2 transition
		// onDone fires settling: exit wizard, run onDone actions, enter completed.
		// (s2 itself has no Exit action declared, so no exit:s2-final line.)
		"exit:wizard",
		"ondone:wizard",
		"enter:completed",
	}
	got := a.Snapshot().Context.Trail
	if !equalStrings(got, want) {
		t.Errorf("trail mismatch:\n got %v\nwant %v", got, want)
	}
}

func TestFinal_OnDoneWithGuard(t *testing.T) {
	m, err := sc.CreateMachine(sc.MachineConfig[engineCtx, engineEvt]{
		ID:      "retry",
		Initial: "loop",
		Context: engineCtx{Allowed: false},
		States: map[string]sc.StateNodeConfig[engineCtx, engineEvt]{
			"loop": {
				Initial: "working",
				OnDone: []sc.TransitionConfig[engineCtx, engineEvt]{
					{Target: "success", Guard: func(c engineCtx, _ engineEvt) bool { return c.Allowed }},
					{Target: "retry"},
				},
				States: map[string]sc.StateNodeConfig[engineCtx, engineEvt]{
					"working": {
						On: map[string][]sc.TransitionConfig[engineCtx, engineEvt]{
							"A": {{Target: "finished"}},
						},
					},
					"finished": {Type: sc.NodeFinal},
				},
			},
			"retry":   {},
			"success": {},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	a := sc.NewActor(m)
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), evtA{})
	if got := a.Snapshot().Value.Path(); got != "retry" {
		t.Errorf("not allowed → retry: got %q", got)
	}

	a2 := sc.NewActor(m, sc.WithInitialValue[engineCtx, engineEvt](sc.AtomicValue("loop")))
	_ = a2.Start(context.Background())
	// We can't easily seed ctx.Allowed=true without a fresh machine; rebuild.
	m2, _ := sc.CreateMachine(sc.MachineConfig[engineCtx, engineEvt]{
		ID:      "retry2",
		Initial: "loop",
		Context: engineCtx{Allowed: true},
		States: map[string]sc.StateNodeConfig[engineCtx, engineEvt]{
			"loop": {
				Initial: "working",
				OnDone: []sc.TransitionConfig[engineCtx, engineEvt]{
					{Target: "success", Guard: func(c engineCtx, _ engineEvt) bool { return c.Allowed }},
					{Target: "retry"},
				},
				States: map[string]sc.StateNodeConfig[engineCtx, engineEvt]{
					"working":  {On: map[string][]sc.TransitionConfig[engineCtx, engineEvt]{"A": {{Target: "finished"}}}},
					"finished": {Type: sc.NodeFinal},
				},
			},
			"retry":   {},
			"success": {},
		},
	})
	a3 := sc.NewActor(m2)
	_ = a3.Start(context.Background())
	_ = a3.Send(context.Background(), evtA{})
	if got := a3.Snapshot().Value.Path(); got != "success" {
		t.Errorf("allowed → success: got %q", got)
	}
}

func TestFinal_RejectsNestedStatesInFinal(t *testing.T) {
	_, err := sc.CreateMachine(sc.MachineConfig[finalCtx, finalEvt]{
		ID:      "bad-final",
		Initial: "x",
		States: map[string]sc.StateNodeConfig[finalCtx, finalEvt]{
			"x": {
				Type:    sc.NodeFinal,
				Initial: "y",
				States: map[string]sc.StateNodeConfig[finalCtx, finalEvt]{
					"y": {},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected error for final state with nested States, got nil")
	}
}
