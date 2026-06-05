package fate_test

import (
	"context"
	"testing"

	sc "github.com/arisros/fate"
	scTest "github.com/arisros/fate/testing"
)

// TrafficLight is the canonical three-state example: red → green → yellow → red.
// Exercises atomic states + single-region transitions.

type tlCtx struct{}
type tlEvt interface{ isTLEvt() }

type evtTimer struct{}

func (evtTimer) isTLEvt()          {}
func (evtTimer) EventName() string { return "TIMER" }

func newTrafficLight(t *testing.T) *sc.Machine[tlCtx, tlEvt] {
	t.Helper()
	m, err := sc.CreateMachine(sc.MachineConfig[tlCtx, tlEvt]{
		ID:      "traffic_light",
		Initial: "red",
		States: map[string]sc.StateNodeConfig[tlCtx, tlEvt]{
			"red":    {On: map[string][]sc.TransitionConfig[tlCtx, tlEvt]{"TIMER": {{Target: "green"}}}},
			"green":  {On: map[string][]sc.TransitionConfig[tlCtx, tlEvt]{"TIMER": {{Target: "yellow"}}}},
			"yellow": {On: map[string][]sc.TransitionConfig[tlCtx, tlEvt]{"TIMER": {{Target: "red"}}}},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	return m
}

func TestTrafficLight_Cycle(t *testing.T) {
	m := newTrafficLight(t)
	a := sc.NewActor(m)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got, want := a.Snapshot().Value.Path(), "red"; got != want {
		t.Fatalf("initial state: got %q want %q", got, want)
	}

	for i, want := range []string{"green", "yellow", "red", "green"} {
		if err := a.Send(context.Background(), evtTimer{}); err != nil {
			t.Fatalf("Send #%d: %v", i, err)
		}
		if got := a.Snapshot().Value.Path(); got != want {
			t.Fatalf("after Send #%d: got %q want %q", i, got, want)
		}
	}
}

func TestTrafficLight_TraceCaptures(t *testing.T) {
	m := newTrafficLight(t)
	a := sc.NewActor(m)

	trace := scTest.NewTrace(a)
	defer trace.Stop()

	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	for range 3 {
		if err := a.Send(context.Background(), evtTimer{}); err != nil {
			t.Fatalf("Send: %v", err)
		}
	}

	wantPaths := []string{"red", "green", "yellow", "red"}
	got := trace.Paths()
	if len(got) != len(wantPaths) {
		t.Fatalf("trace length: got %d want %d (paths: %v)", len(got), len(wantPaths), got)
	}
	for i := range wantPaths {
		if got[i] != wantPaths[i] {
			t.Errorf("trace[%d]: got %q want %q", i, got[i], wantPaths[i])
		}
	}
}

func TestTrafficLight_UnknownEventDropped(t *testing.T) {
	m := newTrafficLight(t)
	a := sc.NewActor(m)
	_ = a.Start(context.Background())

	if err := a.Send(context.Background(), evtUnknown{}); err != nil {
		t.Fatalf("Send unknown: %v", err)
	}
	if got, want := a.Snapshot().Value.Path(), "red"; got != want {
		t.Fatalf("after unknown event: got %q want %q", got, want)
	}
}

type evtUnknown struct{}

func (evtUnknown) isTLEvt()          {}
func (evtUnknown) EventName() string { return "UNKNOWN" }
