package fate_test

import (
	"context"
	"slices"
	"testing"
	"time"

	fate "github.com/arisros/fate"
)

// Entry semantics under parallel regions. Exit and entry share one boundary, so
// these pin both directions: every region is entered when the domain sits above
// the parallel node, and none is re-entered when the transition stays inside it.

type peCtx struct {
	Entered []string
}

func peEnter(label string) fate.Action[peCtx, string] {
	return fate.Named("enter "+label, fate.Assign(func(c peCtx, _ string) peCtx {
		c.Entered = append(slices.Clone(c.Entered), label)
		return c
	}))
}

// A timer in "audio" and an invocation in "captions", so one round trip
// exercises both kinds of armed effect.
func peMachine(t *testing.T) *fate.Machine[peCtx, string] {
	t.Helper()

	m, err := fate.CreateMachine(fate.MachineConfig[peCtx, string]{
		ID:      "player",
		Initial: "player",
		States: map[string]fate.StateNodeConfig[peCtx, string]{
			"player": {
				Type: fate.NodeParallel,
				On: map[string][]fate.TransitionConfig[peCtx, string]{
					"STOP": {{Target: "stopped"}},
				},
				States: map[string]fate.StateNodeConfig[peCtx, string]{
					"audio": {
						Initial: "playing",
						States: map[string]fate.StateNodeConfig[peCtx, string]{
							"playing": {
								Entry: []fate.Action[peCtx, string]{peEnter("audio.playing")},
								After: map[time.Duration][]fate.TransitionConfig[peCtx, string]{
									5 * time.Second: {{Target: "muted"}},
								},
							},
							"muted": {Entry: []fate.Action[peCtx, string]{peEnter("audio.muted")}},
						},
					},
					"captions": {
						Initial: "off",
						States: map[string]fate.StateNodeConfig[peCtx, string]{
							"off": {
								Entry: []fate.Action[peCtx, string]{peEnter("captions.off")},
								Invoke: []fate.Invocation[peCtx, string]{{
									ID:     "render",
									Src:    "activity:render",
									Input:  func(peCtx) any { return nil },
									OnDone: func(any) string { return "SHOW" },
								}},
								On: map[string][]fate.TransitionConfig[peCtx, string]{
									"SHOW": {{Target: "on"}},
								},
							},
							"on": {Entry: []fate.Action[peCtx, string]{peEnter("captions.on")}},
						},
					},
				},
			},
			"stopped": {
				On: map[string][]fate.TransitionConfig[peCtx, string]{
					"GO":          {{Target: "player"}},
					"GO_CAPTIONS": {{Target: "player.captions.on"}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return m
}

func startedPeActor(t *testing.T) *fate.Actor[peCtx, string] {
	t.Helper()
	a := fate.NewActor(peMachine(t))
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	return a
}

func peSend(t *testing.T, a *fate.Actor[peCtx, string], evt string) {
	t.Helper()
	if err := a.Send(context.Background(), evt); err != nil {
		t.Fatalf("send %s: %v", evt, err)
	}
}

// The regression the exit-set fix produced on its own: without the entry-side
// expansion the machine reports audio.playing active with no timer behind it.
func TestReEnteringAParallelNodeReArmsEveryRegionsTimer(t *testing.T) {
	a := startedPeActor(t)

	if n := len(a.PendingTimers()); n != 1 {
		t.Fatalf("PendingTimers at start = %d, want 1", n)
	}
	peSend(t, a, "STOP")
	if n := len(a.PendingTimers()); n != 0 {
		t.Fatalf("PendingTimers after STOP = %d, want 0", n)
	}

	peSend(t, a, "GO")

	timers := a.PendingTimers()
	if len(timers) != 1 {
		t.Fatalf("PendingTimers after re-entry = %d, want 1: the audio region is active with nothing armed", len(timers))
	}
	if !a.FireTimer(timers[0].ID) {
		t.Fatal("FireTimer = false: the re-armed timer does not resolve")
	}
	if got := a.Snapshot(); !got.Matches("player.audio.muted") {
		t.Errorf("audio region at %q, want player.audio.muted after its timer fired", got.Value.Path())
	}
}

// The same guarantee for invocations, armed by the same entry pass.
func TestReEnteringAParallelNodeRestartsEveryRegionsInvocation(t *testing.T) {
	a := startedPeActor(t)

	if n := len(a.PendingInvocations()); n != 1 {
		t.Fatalf("PendingInvocations at start = %d, want 1", n)
	}
	peSend(t, a, "STOP")
	if n := len(a.PendingInvocations()); n != 0 {
		t.Fatalf("PendingInvocations after STOP = %d, want 0", n)
	}

	peSend(t, a, "GO")

	invokes := a.PendingInvocations()
	if len(invokes) != 1 {
		t.Fatalf("PendingInvocations after re-entry = %d, want 1: the captions region is active with nothing in flight", len(invokes))
	}
	if !a.ResolveInvocation(invokes[0].ID, nil) {
		t.Fatal("ResolveInvocation = false: the restarted invocation does not resolve")
	}
	if got := a.Snapshot(); !got.Matches("player.captions.on") {
		t.Errorf("captions region at %q, want player.captions.on after its invocation resolved", got.Value.Path())
	}
}

// The value the machine reports has to be the one the entry actions built.
func TestReEnteringAParallelNodeEntersEveryRegion(t *testing.T) {
	a := startedPeActor(t)
	peSend(t, a, "STOP")
	peSend(t, a, "GO")

	got := a.Snapshot().Context.Entered
	want := []string{"audio.playing", "captions.off", "audio.playing", "captions.off"}
	if !slices.Equal(got, want) {
		t.Errorf("entry actions ran = %#v, want %#v", got, want)
	}
}

// The value reports the siblings active either way, so without this they are
// active having never been entered.
func TestEnteringOneRegionFromOutsideStartsItsSiblings(t *testing.T) {
	a := startedPeActor(t)
	peSend(t, a, "STOP")

	peSend(t, a, "GO_CAPTIONS")

	snap := a.Snapshot()
	if !snap.Matches("player.captions.on") {
		t.Errorf("captions region at %q, want player.captions.on", snap.Value.Path())
	}
	if !snap.Matches("player.audio.playing") {
		t.Errorf("audio region at %q, want player.audio.playing", snap.Value.Path())
	}
	if got, want := snap.Context.Entered, []string{"audio.playing", "captions.off", "audio.playing", "captions.on"}; !slices.Equal(got, want) {
		t.Errorf("entry actions ran = %#v, want %#v", got, want)
	}
	if n := len(a.PendingTimers()); n != 1 {
		t.Errorf("PendingTimers = %d, want 1: the sibling region was reported active without its timer", n)
	}
}

// Passes before and after: a guard against "just widen the entry set", which is
// how the two halves drifted apart in the first place.
func TestCrossRegionTransitionDoesNotReEnterTheOtherRegion(t *testing.T) {
	a := startedPeActor(t)

	before := a.PendingTimers()
	if len(before) != 1 {
		t.Fatalf("PendingTimers before = %d, want 1", len(before))
	}

	peSend(t, a, "SHOW")

	got := a.Snapshot().Context.Entered
	want := []string{"audio.playing", "captions.off", "captions.on"}
	if !slices.Equal(got, want) {
		t.Errorf("entry actions ran = %#v, want %#v: the audio region was re-entered by a captions transition", got, want)
	}

	after := a.PendingTimers()
	if len(after) != 1 {
		t.Fatalf("PendingTimers after = %d, want 1", len(after))
	}
	if after[0].ID != before[0].ID {
		t.Errorf("timer ID changed from %q to %q: the audio region's live timer was replaced", before[0].ID, after[0].ID)
	}
}
