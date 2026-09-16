package fate_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	fate "github.com/arisros/fate"
)

// Tests for the "did that do anything?" surface: Actor.Can, and the bool
// reported by FireTimer / ResolveInvocation / RejectInvocation.
//
// Send drops an unhandled event on purpose, so these are the only way a caller
// can tell "the machine ignored that" from "the machine acted on it".

type unCtx struct {
	Allow bool
	Hits  int
}

// Events are plain strings, so eventNameOf returns them unchanged and the On
// keys below read as themselves.
func unMachine(t *testing.T) *fate.Machine[unCtx, string] {
	t.Helper()
	m, err := fate.CreateMachine(fate.MachineConfig[unCtx, string]{
		ID:      "unhandled",
		Initial: "session",
		Context: unCtx{},
		States: map[string]fate.StateNodeConfig[unCtx, string]{
			"session": {
				Initial: "idle",
				// Declared on the parent, so it must be reachable by bubbling
				// from either child leaf.
				On: map[string][]fate.TransitionConfig[unCtx, string]{
					"ABORT": {{Target: "expired"}},
				},
				States: map[string]fate.StateNodeConfig[unCtx, string]{
					"idle": {
						On: map[string][]fate.TransitionConfig[unCtx, string]{
							"GO": {{Target: "work"}},
							"MAYBE": {{
								Target: "work",
								Guard:  func(c unCtx, _ string) bool { return c.Allow },
							}},
							"BUMP": {{Actions: []fate.Action[unCtx, string]{
								fate.Assign(func(c unCtx, _ string) unCtx { c.Hits++; return c }),
							}}},
						},
						After: map[time.Duration][]fate.TransitionConfig[unCtx, string]{
							5 * time.Second: {{Target: "expired"}},
						},
					},
					"work": {
						Invoke: []fate.Invocation[unCtx, string]{{
							ID:      "job",
							Src:     "activity:job",
							Input:   func(unCtx) any { return nil },
							OnDone:  func(any) string { return "JOB_OK" },
							OnError: func(error) string { return "JOB_ERR" },
						}},
						On: map[string][]fate.TransitionConfig[unCtx, string]{
							"JOB_OK":  {{Target: "done"}},
							"JOB_ERR": {{Target: "failed"}},
							"LEAVE":   {{Target: "idle"}},
						},
					},
				},
			},
			"expired": {Type: fate.NodeFinal},
			"done":    {Type: fate.NodeFinal},
			"failed":  {Type: fate.NodeFinal},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return m
}

func startedUnActor(t *testing.T) *fate.Actor[unCtx, string] {
	t.Helper()
	a := fate.NewActor(unMachine(t))
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	return a
}

func TestCanReportsWhetherEventIsHandled(t *testing.T) {
	a := startedUnActor(t)

	tests := []struct {
		name  string
		event string
		want  bool
	}{
		{"handled on the active leaf", "GO", true},
		{"handled by an ancestor via bubbling", "ABORT", true},
		{"handled with no target, actions only", "BUMP", true},
		{"declared on another state, not this one", "JOB_OK", false},
		{"never declared anywhere", "NONSENSE", false},
		{"empty event name", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := a.Can(tc.event); got != tc.want {
				t.Errorf("Can(%q) = %v, want %v", tc.event, got, tc.want)
			}
		})
	}
}

func TestCanEvaluatesGuards(t *testing.T) {
	a := startedUnActor(t)

	// Allow defaults to false, so the guarded transition must not select.
	if a.Can("MAYBE") {
		t.Fatal("Can(MAYBE) = true with a failing guard, want false")
	}

	// Flip the guard's input through an ordinary transition, then ask again.
	if err := a.Send(context.Background(), "BUMP"); err != nil {
		t.Fatalf("send BUMP: %v", err)
	}
	a2 := fate.NewActor(unMachine(t), fate.WithInitialValue[unCtx, string](a.Snapshot().Value))
	_ = a2.Start(context.Background())
	if a2.Can("MAYBE") {
		t.Fatal("Can(MAYBE) = true, want false: context still has Allow=false")
	}
}

func TestCanIsExactAboutGuardedTransitions(t *testing.T) {
	// A machine whose only transition is guarded true, to prove Can follows the
	// guard rather than merely finding the event name declared.
	m, err := fate.CreateMachine(fate.MachineConfig[unCtx, string]{
		ID:      "guarded",
		Initial: "idle",
		Context: unCtx{Allow: true},
		States: map[string]fate.StateNodeConfig[unCtx, string]{
			"idle": {On: map[string][]fate.TransitionConfig[unCtx, string]{
				"MAYBE": {{Target: "open", Guard: func(c unCtx, _ string) bool { return c.Allow }}},
			}},
			"open": {},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	a := fate.NewActor(m)
	_ = a.Start(context.Background())
	if !a.Can("MAYBE") {
		t.Fatal("Can(MAYBE) = false with a passing guard, want true")
	}
}

func TestCanMatchesWildcardHandler(t *testing.T) {
	m, err := fate.CreateMachine(fate.MachineConfig[unCtx, string]{
		ID:      "wild",
		Initial: "idle",
		States: map[string]fate.StateNodeConfig[unCtx, string]{
			"idle": {On: map[string][]fate.TransitionConfig[unCtx, string]{
				"*": {{Target: "seen"}},
			}},
			"seen": {},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	a := fate.NewActor(m)
	_ = a.Start(context.Background())
	if !a.Can("ANYTHING_AT_ALL") {
		t.Fatal("Can = false under a wildcard handler, want true")
	}
}

func TestCanIsFalseWhenActorIsNotRunning(t *testing.T) {
	t.Run("before Start", func(t *testing.T) {
		a := fate.NewActor(unMachine(t))
		if a.Can("GO") {
			t.Fatal("Can = true before Start, want false")
		}
	})

	t.Run("after Stop", func(t *testing.T) {
		a := startedUnActor(t)
		a.Stop()
		if a.Can("GO") {
			t.Fatal("Can = true after Stop, want false")
		}
	})

	t.Run("after reaching a final state", func(t *testing.T) {
		a := startedUnActor(t)
		if err := a.Send(context.Background(), "ABORT"); err != nil {
			t.Fatalf("send ABORT: %v", err)
		}
		if got := a.Snapshot().Status; got != fate.StatusDone {
			t.Fatalf("status = %v, want %v", got, fate.StatusDone)
		}
		if a.Can("GO") {
			t.Fatal("Can = true on a completed actor, want false")
		}
	})
}

// Can must be a question, not a move. A caller that asks before every Send
// would otherwise silently double-apply guard side effects or advance state.
func TestCanDoesNotMutateTheActor(t *testing.T) {
	a := startedUnActor(t)

	before, err := a.Persist()
	if err != nil {
		t.Fatalf("persist before: %v", err)
	}

	for _, evt := range []string{"GO", "MAYBE", "ABORT", "BUMP", "NONSENSE", ""} {
		a.Can(evt)
	}

	after, err := a.Persist()
	if err != nil {
		t.Fatalf("persist after: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("Can mutated the actor:\n before %s\n after  %s", before, after)
	}
}

func TestFireTimerReportsDelivery(t *testing.T) {
	t.Run("armed and active", func(t *testing.T) {
		a := startedUnActor(t)
		timers := a.PendingTimers()
		if len(timers) != 1 {
			t.Fatalf("PendingTimers = %d, want 1", len(timers))
		}
		if !a.FireTimer(timers[0].ID) {
			t.Fatal("FireTimer = false for an armed timer, want true")
		}
		if got := a.Snapshot().Value.Path(); got != "expired" {
			t.Fatalf("state = %q, want %q", got, "expired")
		}
	})

	t.Run("unknown id", func(t *testing.T) {
		a := startedUnActor(t)
		if a.FireTimer(fate.TimerID("no.such.timer")) {
			t.Fatal("FireTimer = true for an unknown id, want false")
		}
	})

	t.Run("already fired", func(t *testing.T) {
		a := startedUnActor(t)
		id := a.PendingTimers()[0].ID
		if !a.FireTimer(id) {
			t.Fatal("first FireTimer = false, want true")
		}
		if a.FireTimer(id) {
			t.Fatal("second FireTimer = true, want false: the timer is spent")
		}
	})

	t.Run("cancelled by leaving the owning state", func(t *testing.T) {
		a := startedUnActor(t)
		id := a.PendingTimers()[0].ID
		if err := a.Send(context.Background(), "GO"); err != nil {
			t.Fatalf("send GO: %v", err)
		}
		if a.FireTimer(id) {
			t.Fatal("FireTimer = true after exiting the owning state, want false")
		}
		if got := a.Snapshot().Value.Path(); got != "session.work" {
			t.Fatalf("state = %q, want %q: a stale timer must not move the machine", got, "session.work")
		}
	})

	t.Run("actor stopped", func(t *testing.T) {
		a := startedUnActor(t)
		id := a.PendingTimers()[0].ID
		a.Stop()
		if a.FireTimer(id) {
			t.Fatal("FireTimer = true on a stopped actor, want false")
		}
	})
}

func TestResolveAndRejectInvocationReportAcceptance(t *testing.T) {
	// Drive to "work", where the invocation is armed.
	inWork := func(t *testing.T) (*fate.Actor[unCtx, string], fate.InvokeID) {
		t.Helper()
		a := startedUnActor(t)
		if err := a.Send(context.Background(), "GO"); err != nil {
			t.Fatalf("send GO: %v", err)
		}
		pending := a.PendingInvocations()
		if len(pending) != 1 {
			t.Fatalf("PendingInvocations = %d, want 1", len(pending))
		}
		return a, pending[0].ID
	}

	t.Run("resolve an armed invocation", func(t *testing.T) {
		a, id := inWork(t)
		if !a.ResolveInvocation(id, "payload") {
			t.Fatal("ResolveInvocation = false for an armed invocation, want true")
		}
		if got := a.Snapshot().Value.Path(); got != "done" {
			t.Fatalf("state = %q, want %q", got, "done")
		}
	})

	t.Run("reject an armed invocation", func(t *testing.T) {
		a, id := inWork(t)
		if !a.RejectInvocation(id, errors.New("boom")) {
			t.Fatal("RejectInvocation = false for an armed invocation, want true")
		}
		if got := a.Snapshot().Value.Path(); got != "failed" {
			t.Fatalf("state = %q, want %q", got, "failed")
		}
	})

	t.Run("unknown id", func(t *testing.T) {
		a, _ := inWork(t)
		if a.ResolveInvocation(fate.InvokeID("no.such.invoke"), nil) {
			t.Error("ResolveInvocation = true for an unknown id, want false")
		}
		if a.RejectInvocation(fate.InvokeID("no.such.invoke"), errors.New("boom")) {
			t.Error("RejectInvocation = true for an unknown id, want false")
		}
	})

	t.Run("already settled", func(t *testing.T) {
		a, id := inWork(t)
		if !a.ResolveInvocation(id, nil) {
			t.Fatal("first ResolveInvocation = false, want true")
		}
		if a.ResolveInvocation(id, nil) {
			t.Error("second ResolveInvocation = true, want false: already settled")
		}
		if a.RejectInvocation(id, errors.New("boom")) {
			t.Error("RejectInvocation = true after settling, want false")
		}
	})

	t.Run("owning state already left", func(t *testing.T) {
		a, id := inWork(t)
		if err := a.Send(context.Background(), "LEAVE"); err != nil {
			t.Fatalf("send LEAVE: %v", err)
		}
		if a.ResolveInvocation(id, nil) {
			t.Fatal("ResolveInvocation = true after leaving the owning state, want false")
		}
		if got := a.Snapshot().Value.Path(); got != "session.idle" {
			t.Fatalf("state = %q, want %q: a late result must not move the machine", got, "session.idle")
		}
	})

	t.Run("actor stopped", func(t *testing.T) {
		a, id := inWork(t)
		a.Stop()
		if a.ResolveInvocation(id, nil) {
			t.Error("ResolveInvocation = true on a stopped actor, want false")
		}
		if a.RejectInvocation(id, errors.New("boom")) {
			t.Error("RejectInvocation = true on a stopped actor, want false")
		}
	})
}

// An invocation may decline to map its outcome. Accepting it still reports
// true: the invocation was consumed, it simply produced no event. This is the
// branch that distinguishes "accepted" from "delivered an event".
func TestInvocationWithoutMappersIsAcceptedButDeliversNothing(t *testing.T) {
	m, err := fate.CreateMachine(fate.MachineConfig[unCtx, string]{
		ID:      "unmapped",
		Initial: "work",
		States: map[string]fate.StateNodeConfig[unCtx, string]{
			"work": {
				Invoke: []fate.Invocation[unCtx, string]{{
					ID:    "job",
					Src:   "activity:job",
					Input: func(unCtx) any { return nil },
					// OnDone and OnError deliberately nil.
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	t.Run("resolve", func(t *testing.T) {
		a := fate.NewActor(m)
		_ = a.Start(context.Background())
		id := a.PendingInvocations()[0].ID
		if !a.ResolveInvocation(id, "ignored") {
			t.Fatal("ResolveInvocation = false, want true: accepted with no OnDone")
		}
		if got := a.Snapshot().Value.Path(); got != "work" {
			t.Fatalf("state = %q, want %q: nothing should have moved", got, "work")
		}
	})

	t.Run("reject", func(t *testing.T) {
		a := fate.NewActor(m)
		_ = a.Start(context.Background())
		id := a.PendingInvocations()[0].ID
		if !a.RejectInvocation(id, errors.New("boom")) {
			t.Fatal("RejectInvocation = false, want true: accepted with no OnError")
		}
		if got := a.Snapshot().Value.Path(); got != "work" {
			t.Fatalf("state = %q, want %q: nothing should have moved", got, "work")
		}
	})
}
