package fate_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	fate "github.com/arisros/fate"
)

type pxCtx struct {
	Exited []string
}

func pxExit(label string) fate.Action[pxCtx, string] {
	return fate.Named(label, fate.Assign(func(c pxCtx, _ string) pxCtx {
		c.Exited = append(slices.Clone(c.Exited), label)
		return c
	}))
}

func pxMachine(t *testing.T, audioEffect func(*fate.StateNodeConfig[pxCtx, string])) *fate.Machine[pxCtx, string] {
	t.Helper()

	playing := fate.StateNodeConfig[pxCtx, string]{
		Exit: []fate.Action[pxCtx, string]{pxExit("audio.playing")},
		On: map[string][]fate.TransitionConfig[pxCtx, string]{
			"MUTE": {{Target: "muted"}},
		},
	}
	if audioEffect != nil {
		audioEffect(&playing)
	}

	m, err := fate.CreateMachine(fate.MachineConfig[pxCtx, string]{
		ID:      "player",
		Initial: "player",
		States: map[string]fate.StateNodeConfig[pxCtx, string]{
			"player": {
				Type: fate.NodeParallel,
				States: map[string]fate.StateNodeConfig[pxCtx, string]{
					"audio": {
						Initial: "playing",
						States: map[string]fate.StateNodeConfig[pxCtx, string]{
							"playing": playing,
							"muted":   {},
						},
					},
					"captions": {
						Initial: "off",
						States: map[string]fate.StateNodeConfig[pxCtx, string]{
							"off": {
								Exit: []fate.Action[pxCtx, string]{pxExit("captions.off")},
								On: map[string][]fate.TransitionConfig[pxCtx, string]{
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

func startedPxActor(t *testing.T, effect func(*fate.StateNodeConfig[pxCtx, string])) *fate.Actor[pxCtx, string] {
	t.Helper()
	a := fate.NewActor(pxMachine(t, effect))
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
	withTimer := func(s *fate.StateNodeConfig[pxCtx, string]) {
		s.After = map[time.Duration][]fate.TransitionConfig[pxCtx, string]{
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
	withInvoke := func(s *fate.StateNodeConfig[pxCtx, string]) {
		s.Invoke = []fate.Invocation[pxCtx, string]{{
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

func pxLeavingMachine(t *testing.T) *fate.Machine[pxCtx, string] {
	t.Helper()
	m, err := fate.CreateMachine(fate.MachineConfig[pxCtx, string]{
		ID:      "player",
		Initial: "player",
		States: map[string]fate.StateNodeConfig[pxCtx, string]{
			"player": {
				Type: fate.NodeParallel,
				Exit: []fate.Action[pxCtx, string]{pxExit("player")},
				On: map[string][]fate.TransitionConfig[pxCtx, string]{
					"STOP": {{Target: "stopped"}},
				},
				States: map[string]fate.StateNodeConfig[pxCtx, string]{
					"audio": {
						Initial: "playing",
						Exit:    []fate.Action[pxCtx, string]{pxExit("audio")},
						States: map[string]fate.StateNodeConfig[pxCtx, string]{
							"playing": {Exit: []fate.Action[pxCtx, string]{pxExit("audio.playing")}},
						},
					},
					"captions": {
						Initial: "off",
						Exit:    []fate.Action[pxCtx, string]{pxExit("captions")},
						States: map[string]fate.StateNodeConfig[pxCtx, string]{
							"off": {
								Exit: []fate.Action[pxCtx, string]{pxExit("captions.off")},
								On: map[string][]fate.TransitionConfig[pxCtx, string]{
									"SHOW": {{Target: "on"}},
								},
							},
							"on":   {Exit: []fate.Action[pxCtx, string]{pxExit("captions.on")}},
							"hist": {Type: fate.NodeHistory, Default: "off"},
						},
					},
				},
			},
			"stopped": {On: map[string][]fate.TransitionConfig[pxCtx, string]{
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
	a := fate.NewActor(pxLeavingMachine(t))
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
	a := fate.NewActor(pxLeavingMachine(t))
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

func pxDomainMachine(t *testing.T, parallelOn map[string][]fate.TransitionConfig[pxCtx, string]) *fate.Machine[pxCtx, string] {
	t.Helper()
	region := func(name, initial, other string, on map[string][]fate.TransitionConfig[pxCtx, string]) fate.StateNodeConfig[pxCtx, string] {
		return fate.StateNodeConfig[pxCtx, string]{
			Initial: initial,
			States: map[string]fate.StateNodeConfig[pxCtx, string]{
				initial: {
					Exit: []fate.Action[pxCtx, string]{pxExit(name + "." + initial)},
					After: map[time.Duration][]fate.TransitionConfig[pxCtx, string]{
						5 * time.Second: {{Target: other}},
					},
					On: on,
				},
				other: {},
			},
		}
	}
	m, err := fate.CreateMachine(fate.MachineConfig[pxCtx, string]{
		ID:      "player",
		Initial: "player",
		States: map[string]fate.StateNodeConfig[pxCtx, string]{
			"player": {
				Type: fate.NodeParallel,
				On:   parallelOn,
				States: map[string]fate.StateNodeConfig[pxCtx, string]{
					"audio": region("audio", "playing", "muted", map[string][]fate.TransitionConfig[pxCtx, string]{
						"CROSS_FROM_AUDIO": {{Target: "captions.on"}},
						"SELF_AUDIO":       {{Target: "audio"}},
					}),
					"captions": region("captions", "off", "on", map[string][]fate.TransitionConfig[pxCtx, string]{
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

func assertNoOrphanedTimer(t *testing.T, a *fate.Actor[pxCtx, string], statePath, label string) {
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
			a := fate.NewActor(pxDomainMachine(t, nil))
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
	a := fate.NewActor(pxDomainMachine(t, map[string][]fate.TransitionConfig[pxCtx, string]{
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
	a := fate.NewActor(pxDomainMachine(t, map[string][]fate.TransitionConfig[pxCtx, string]{
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
	a := fate.NewActor(pxDomainMachine(t, nil))
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := a.Send(context.Background(), "SELF_AUDIO"); err != nil {
		t.Fatalf("send SELF_AUDIO: %v", err)
	}
	assertNoOrphanedTimer(t, a, "player.captions.off", "external self-transition")
}
