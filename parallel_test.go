package fate_test

import (
	"context"
	"testing"

	sc "github.com/arisros/fate"
)

// Two-region traffic light + pedestrian crossing — the canonical XState
// parallel example. The `lights` region cycles red → green → yellow → red.
// The `pedestrian` region cycles waiting → walking. Both run simultaneously
// and respond to independent events.

type parCtx struct{}
type parEvt interface{ isParEvt() }

type evtCarTimer struct{}
type evtPedPress struct{}
type evtPedTimer struct{}

func (evtCarTimer) isParEvt()         {}
func (evtPedPress) isParEvt()         {}
func (evtPedTimer) isParEvt()         {}
func (evtCarTimer) EventName() string { return "CAR_TIMER" }
func (evtPedPress) EventName() string { return "PED_PRESS" }
func (evtPedTimer) EventName() string { return "PED_TIMER" }

func newCrossing(t *testing.T) *sc.Machine[parCtx, parEvt] {
	t.Helper()
	m, err := sc.CreateMachine(sc.MachineConfig[parCtx, parEvt]{
		ID:      "crossing",
		Initial: "intersection",
		States: map[string]sc.StateNodeConfig[parCtx, parEvt]{
			"intersection": {
				Type: sc.NodeParallel,
				States: map[string]sc.StateNodeConfig[parCtx, parEvt]{
					"lights": {
						Initial: "red",
						States: map[string]sc.StateNodeConfig[parCtx, parEvt]{
							"red":    {On: map[string][]sc.TransitionConfig[parCtx, parEvt]{"CAR_TIMER": {{Target: "green"}}}},
							"green":  {On: map[string][]sc.TransitionConfig[parCtx, parEvt]{"CAR_TIMER": {{Target: "yellow"}}}},
							"yellow": {On: map[string][]sc.TransitionConfig[parCtx, parEvt]{"CAR_TIMER": {{Target: "red"}}}},
						},
					},
					"pedestrian": {
						Initial: "waiting",
						States: map[string]sc.StateNodeConfig[parCtx, parEvt]{
							"waiting": {On: map[string][]sc.TransitionConfig[parCtx, parEvt]{"PED_PRESS": {{Target: "walking"}}}},
							"walking": {On: map[string][]sc.TransitionConfig[parCtx, parEvt]{"PED_TIMER": {{Target: "waiting"}}}},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	return m
}

func TestParallel_InitialConfigurationHasBothRegions(t *testing.T) {
	m := newCrossing(t)
	a := sc.NewActor(m)
	_ = a.Start(context.Background())

	if !a.Snapshot().Matches("intersection.lights.red") {
		t.Errorf("lights region: want red active; got path %q", a.Snapshot().Value.Path())
	}
	if !a.Snapshot().Matches("intersection.pedestrian.waiting") {
		t.Errorf("pedestrian region: want waiting active; got path %q", a.Snapshot().Value.Path())
	}
}

func TestParallel_RegionsTransitionIndependently(t *testing.T) {
	m := newCrossing(t)
	a := sc.NewActor(m)
	_ = a.Start(context.Background())

	// Car timer: lights advances, pedestrian unchanged.
	_ = a.Send(context.Background(), evtCarTimer{})
	if !a.Snapshot().Matches("intersection.lights.green") {
		t.Errorf("after CAR_TIMER: lights should be green; got %q", a.Snapshot().Value.Path())
	}
	if !a.Snapshot().Matches("intersection.pedestrian.waiting") {
		t.Errorf("after CAR_TIMER: pedestrian should still be waiting; got %q", a.Snapshot().Value.Path())
	}

	// Pedestrian press: pedestrian advances, lights unchanged.
	_ = a.Send(context.Background(), evtPedPress{})
	if !a.Snapshot().Matches("intersection.lights.green") {
		t.Errorf("after PED_PRESS: lights should still be green; got %q", a.Snapshot().Value.Path())
	}
	if !a.Snapshot().Matches("intersection.pedestrian.walking") {
		t.Errorf("after PED_PRESS: pedestrian should be walking; got %q", a.Snapshot().Value.Path())
	}
}
