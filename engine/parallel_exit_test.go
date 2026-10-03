package engine_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/arisros/fate/action"
	"github.com/arisros/fate/effect"
	"github.com/arisros/fate/engine"
)

type pxCtx struct {
	Exited []string
}

func pxExit(label string) action.Action[pxCtx, string] {
	return action.Named(label, action.Assign(func(c pxCtx, _ string) pxCtx {
		c.Exited = append(slices.Clone(c.Exited), label)
		return c
	}))
}

func pxMachine(t *testing.T, audioEffect func(*engine.StateNodeConfig[pxCtx, string])) *engine.Machine[pxCtx, string] {
	t.Helper()

	playing := engine.StateNodeConfig[pxCtx, string]{
		Exit: []action.Action[pxCtx, string]{pxExit("audio.playing")},
		On: map[string][]engine.TransitionConfig[pxCtx, string]{
			"MUTE": {{Target: "muted"}},
		},
	}
	if audioEffect != nil {
		audioEffect(&playing)
	}

	m, err := engine.CreateMachine(engine.MachineConfig[pxCtx, string]{
		ID:      "player",
		Initial: "player",
		States: map[string]engine.StateNodeConfig[pxCtx, string]{
			"player": {
				Type: engine.NodeParallel,
				States: map[string]engine.StateNodeConfig[pxCtx, string]{
					"audio": {
						Initial: "playing",
						States: map[string]engine.StateNodeConfig[pxCtx, string]{
							"playing": playing,
							"muted":   {},
						},
					},
					"captions": {
						Initial: "off",
						States: map[string]engine.StateNodeConfig[pxCtx, string]{
							"off": {
								Exit: []action.Action[pxCtx, string]{pxExit("captions.off")},
								On: map[string][]engine.TransitionConfig[pxCtx, string]{
									"SHOW": {{Target: "on"}},
								},
							},
							"on": {},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return m
}

func startedPxActor(t *testing.T, effect func(*engine.StateNodeConfig[pxCtx, string])) *engine.Actor[pxCtx, string] {
	t.Helper()
	a := engine.NewActor(pxMachine(t, effect))
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	return a
}

func TestParallelTransitionExitsOnlyItsOwnRegion(t *testing.T) {
	a := startedPxActor(t, nil)

	if err := a.Send(context.Background(), "SHOW"); err != nil {
		t.Fatalf("send SHOW: %v", err)
	}

	got := a.Snapshot().Context.Exited
	want := []string{"captions.off"}
	if !slices.Equal(got, want) {
		t.Errorf("exit actions ran = %#v, want %#v", got, want)
	}
	if slices.Contains(got, "audio.playing") {
		t.Error("a transition in the captions region ran the audio region's exit action")
	}
}

func TestParallelTransitionLeavesTheOtherRegionActive(t *testing.T) {
	a := startedPxActor(t, nil)

	if err := a.Send(context.Background(), "SHOW"); err != nil {
		t.Fatalf("send SHOW: %v", err)
	}

	snap := a.Snapshot()
	if !snap.Matches("player.audio.playing") {
		t.Errorf("audio region left %q, want it still in player.audio.playing", snap.Value.Path())
	}
	if !snap.Matches("player.captions.on") {
		t.Errorf("captions region at %q, want player.captions.on", snap.Value.Path())
	}
}

func TestParallelTransitionDoesNotDisarmAnotherRegionsTimer(t *testing.T) {
	withTimer := func(s *engine.StateNodeConfig[pxCtx, string]) {
		s.After = map[time.Duration][]engine.TransitionConfig[pxCtx, string]{
			5 * time.Second: {{Target: "muted"}},
		}
	}
	a := startedPxActor(t, withTimer)

	before := a.PendingTimers()
	if len(before) != 1 {
		t.Fatalf("PendingTimers before = %d, want 1", len(before))
	}
	timerID := before[0].ID

	if err := a.Send(context.Background(), "SHOW"); err != nil {
		t.Fatalf("send SHOW: %v", err)
	}

	after := a.PendingTimers()
	if len(after) != 1 {
		t.Fatalf("PendingTimers after = %d, want 1: the audio region's timer was disarmed by a captions transition", len(after))
	}
	if !a.FireTimer(timerID) {
		t.Fatal("FireTimer = false: the audio region's timer no longer resolves")
	}
	if got := a.Snapshot(); !got.Matches("player.audio.muted") {
		t.Errorf("audio region at %q, want player.audio.muted after its timer fired", got.Value.Path())
	}
}

func TestParallelTransitionDoesNotCancelAnotherRegionsInvocation(t *testing.T) {
	withInvoke := func(s *engine.StateNodeConfig[pxCtx, string]) {
		s.Invoke = []effect.Invocation[pxCtx, string]{{
			ID:     "decode",
			Src:    "activity:decode",
			Input:  func(pxCtx) any { return nil },
			OnDone: func(any) string { return "MUTE" },
		}}
	}
	a := startedPxActor(t, withInvoke)

	before := a.PendingInvocations()
	if len(before) != 1 {
		t.Fatalf("PendingInvocations before = %d, want 1", len(before))
	}
	invokeID := before[0].ID

	if err := a.Send(context.Background(), "SHOW"); err != nil {
		t.Fatalf("send SHOW: %v", err)
	}

	after := a.PendingInvocations()
	if len(after) != 1 {
		t.Fatalf("PendingInvocations after = %d, want 1: the audio region's invocation was cancelled by a captions transition", len(after))
	}
	if !a.ResolveInvocation(invokeID, nil) {
		t.Fatal("ResolveInvocation = false: the audio region's invocation no longer resolves")
	}
	if got := a.Snapshot(); !got.Matches("player.audio.muted") {
		t.Errorf("audio region at %q, want player.audio.muted after its invocation resolved", got.Value.Path())
	}
}

func TestParallelTransitionInTheFirstRegionAlsoExitsOnlyItself(t *testing.T) {
	a := startedPxActor(t, nil)

	if err := a.Send(context.Background(), "MUTE"); err != nil {
		t.Fatalf("send MUTE: %v", err)
	}

	got := a.Snapshot().Context.Exited
	want := []string{"audio.playing"}
	if !slices.Equal(got, want) {
		t.Errorf("exit actions ran = %#v, want %#v", got, want)
	}
	snap := a.Snapshot()
	if !snap.Matches("player.audio.muted") || !snap.Matches("player.captions.off") {
		t.Errorf("configuration = %q, want player.audio.muted with captions still off", snap.Value.Path())
	}
}

func pxLeavingMachine(t *testing.T) *engine.Machine[pxCtx, string] {
	t.Helper()
	m, err := engine.CreateMachine(engine.MachineConfig[pxCtx, string]{
		ID:      "player",
		Initial: "player",
		States: map[string]engine.StateNodeConfig[pxCtx, string]{
			"player": {
				Type: engine.NodeParallel,
				Exit: []action.Action[pxCtx, string]{pxExit("player")},
				On: map[string][]engine.TransitionConfig[pxCtx, string]{
					"STOP": {{Target: "stopped"}},
				},
				States: map[string]engine.StateNodeConfig[pxCtx, string]{
					"audio": {
						Initial: "playing",
						Exit:    []action.Action[pxCtx, string]{pxExit("audio")},
						States: map[string]engine.StateNodeConfig[pxCtx, string]{
							"playing": {Exit: []action.Action[pxCtx, string]{pxExit("audio.playing")}},
						},
					},
					"captions": {
						Initial: "off",
						Exit:    []action.Action[pxCtx, string]{pxExit("captions")},
						States: map[string]engine.StateNodeConfig[pxCtx, string]{
							"off": {
								Exit: []action.Action[pxCtx, string]{pxExit("captions.off")},
								On: map[string][]engine.TransitionConfig[pxCtx, string]{
									"SHOW": {{Target: "on"}},
								},
							},
							"on":   {Exit: []action.Action[pxCtx, string]{pxExit("captions.on")}},
							"hist": {Type: engine.NodeHistory, Default: "off"},
						},
					},
				},
			},
			"stopped": {On: map[string][]engine.TransitionConfig[pxCtx, string]{
				"RESUME": {{Target: "player.captions.hist"}},
			}},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return m
}

func TestLeavingTheParallelNodeExitsEveryRegion(t *testing.T) {
	a := engine.NewActor(pxLeavingMachine(t))
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := a.Send(context.Background(), "STOP"); err != nil {
		t.Fatalf("send STOP: %v", err)
	}

	got := a.Snapshot().Context.Exited
	want := []string{"audio.playing", "captions.off", "audio", "captions", "player"}
	if !slices.Equal(got, want) {
		t.Errorf("exit actions ran = %#v, want %#v", got, want)
	}
	if p := a.Snapshot().Value.Path(); p != "stopped" {
		t.Errorf("state = %q, want %q", p, "stopped")
	}
}

func TestParallelRegionRecordsItsOwnShallowHistory(t *testing.T) {
	a := engine.NewActor(pxLeavingMachine(t))
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := a.Send(context.Background(), "SHOW"); err != nil {
		t.Fatalf("send SHOW: %v", err)
	}
	if !a.Snapshot().Matches("player.captions.on") {
		t.Fatalf("setup failed: captions at %q, want player.captions.on", a.Snapshot().Value.Path())
	}

	if err := a.Send(context.Background(), "STOP"); err != nil {
		t.Fatalf("send STOP: %v", err)
	}
	if err := a.Send(context.Background(), "RESUME"); err != nil {
		t.Fatalf("send RESUME: %v", err)
	}

	if snap := a.Snapshot(); !snap.Matches("player.captions.on") {
		t.Errorf("captions restored to %q, want player.captions.on: the region's history was not recorded", snap.Value.Path())
	}
}

func pxDomainMachine(t *testing.T, parallelOn map[string][]engine.TransitionConfig[pxCtx, string]) *engine.Machine[pxCtx, string] {
	t.Helper()
	region := func(name, initial, other string, on map[string][]engine.TransitionConfig[pxCtx, string]) engine.StateNodeConfig[pxCtx, string] {
		return engine.StateNodeConfig[pxCtx, string]{
			Initial: initial,
			States: map[string]engine.StateNodeConfig[pxCtx, string]{
				initial: {
					Exit: []action.Action[pxCtx, string]{pxExit(name + "." + initial)},
					After: map[time.Duration][]engine.TransitionConfig[pxCtx, string]{
						5 * time.Second: {{Target: other}},
					},
					On: on,
				},
				other: {},
			},
		}
	}
	m, err := engine.CreateMachine(engine.MachineConfig[pxCtx, string]{
		ID:      "player",
		Initial: "player",
		States: map[string]engine.StateNodeConfig[pxCtx, string]{
			"player": {
				Type: engine.NodeParallel,
				On:   parallelOn,
				States: map[string]engine.StateNodeConfig[pxCtx, string]{
					"audio": region("audio", "playing", "muted", map[string][]engine.TransitionConfig[pxCtx, string]{
						"CROSS_FROM_AUDIO": {{Target: "captions.on"}},
						"SELF_AUDIO":       {{Target: "audio"}},
					}),
					"captions": region("captions", "off", "on", map[string][]engine.TransitionConfig[pxCtx, string]{
						"CROSS_FROM_CAPTIONS": {{Target: "audio.muted"}},
					}),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return m
}

func assertNoOrphanedTimer(t *testing.T, a *engine.Actor[pxCtx, string], statePath, label string) {
	t.Helper()
	if !a.Snapshot().Matches(statePath) {
		return
	}
	for _, timer := range a.PendingTimers() {
		if strings.HasPrefix(string(timer.ID), statePath) {
			return
		}
	}
	t.Errorf("%s: %s is reported active but its timer was disarmed; exited=%v",
		label, statePath, a.Snapshot().Context.Exited)
}

func TestCrossRegionTargetLeavesTheSourceRegionIntact(t *testing.T) {
	tests := []struct {
		name       string
		event      string
		stayActive string
		exited     []string
	}{
		{"sourced in the region that sorts second", "CROSS_FROM_CAPTIONS", "player.captions.off", []string{"audio.playing"}},
		{"sourced in the region that sorts first", "CROSS_FROM_AUDIO", "player.audio.playing", []string{"captions.off"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := engine.NewActor(pxDomainMachine(t, nil))
			if err := a.Start(context.Background()); err != nil {
				t.Fatalf("start: %v", err)
			}
			if got := len(a.PendingTimers()); got != 2 {
				t.Fatalf("PendingTimers before = %d, want 2", got)
			}
			if err := a.Send(context.Background(), tc.event); err != nil {
				t.Fatalf("send %s: %v", tc.event, err)
			}
			if got := a.Snapshot().Context.Exited; !slices.Equal(got, tc.exited) {
				t.Errorf("exit actions ran = %#v, want %#v", got, tc.exited)
			}
			assertNoOrphanedTimer(t, a, tc.stayActive, tc.name)
		})
	}
}

func TestHandlerOnParallelNodeTargetingItsOwnDescendant(t *testing.T) {
	a := engine.NewActor(pxDomainMachine(t, map[string][]engine.TransitionConfig[pxCtx, string]{
		"JUMP": {{Target: "audio.muted"}},
	}))
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := a.Send(context.Background(), "JUMP"); err != nil {
		t.Fatalf("send JUMP: %v", err)
	}
	if got := a.Snapshot().Context.Exited; !slices.Equal(got, []string{"audio.playing"}) {
		t.Errorf("exit actions ran = %#v, want [audio.playing]", got)
	}
	assertNoOrphanedTimer(t, a, "player.captions.off", "handler on parallel node")
}

func TestInternalTransitionOnParallelNode(t *testing.T) {
	a := engine.NewActor(pxDomainMachine(t, map[string][]engine.TransitionConfig[pxCtx, string]{
		"INNER": {{Target: "audio.muted", Internal: true}},
	}))
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := a.Send(context.Background(), "INNER"); err != nil {
		t.Fatalf("send INNER: %v", err)
	}
	assertNoOrphanedTimer(t, a, "player.captions.off", "internal transition on parallel node")
}

func TestExternalSelfTransitionOnARegion(t *testing.T) {
	a := engine.NewActor(pxDomainMachine(t, nil))
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := a.Send(context.Background(), "SELF_AUDIO"); err != nil {
		t.Fatalf("send SELF_AUDIO: %v", err)
	}
	assertNoOrphanedTimer(t, a, "player.captions.off", "external self-transition")
}
