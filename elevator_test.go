package fate_test

import (
	"context"
	"testing"

	sc "github.com/arisros/fate"
)

// Elevator exercises compound states. The door is a compound parent with
// {open, closed} children; the motion is a separate compound with {idle,
// moving}. In the P3 skeleton (no parallel regions yet) they are modeled
// as a single hierarchy: door.{open, closed} where closed has motion sub-states.

type elevCtx struct{}
type elevEvt interface{ isElevEvt() }

type evtOpen struct{}
type evtClose struct{}
type evtGo struct{}
type evtArrive struct{}

func (evtOpen) isElevEvt()          {}
func (evtClose) isElevEvt()         {}
func (evtGo) isElevEvt()            {}
func (evtArrive) isElevEvt()        {}
func (evtOpen) EventName() string   { return "OPEN" }
func (evtClose) EventName() string  { return "CLOSE" }
func (evtGo) EventName() string     { return "GO" }
func (evtArrive) EventName() string { return "ARRIVE" }

func TestElevator_NestedCompoundCycle(t *testing.T) {
	m, err := sc.CreateMachine(sc.MachineConfig[elevCtx, elevEvt]{
		ID:      "elevator",
		Initial: "door",
		States: map[string]sc.StateNodeConfig[elevCtx, elevEvt]{
			"door": {
				Initial: "open",
				States: map[string]sc.StateNodeConfig[elevCtx, elevEvt]{
					"open": {
						On: map[string][]sc.TransitionConfig[elevCtx, elevEvt]{
							"CLOSE": {{Target: "closed"}},
						},
					},
					"closed": {
						Initial: "idle",
						States: map[string]sc.StateNodeConfig[elevCtx, elevEvt]{
							"idle": {
								On: map[string][]sc.TransitionConfig[elevCtx, elevEvt]{
									"GO":   {{Target: "moving"}},
									"OPEN": {{Target: "open"}},
								},
							},
							"moving": {
								On: map[string][]sc.TransitionConfig[elevCtx, elevEvt]{
									"ARRIVE": {{Target: "idle"}},
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}

	a := sc.NewActor(m)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	steps := []struct {
		evt  elevEvt
		want string
	}{
		{nil, "door.open"}, // initial
		{evtClose{}, "door.closed.idle"},
		{evtGo{}, "door.closed.moving"},
		{evtArrive{}, "door.closed.idle"},
		{evtOpen{}, "door.open"},
	}

	for i, step := range steps {
		if step.evt != nil {
			if err := a.Send(context.Background(), step.evt); err != nil {
				t.Fatalf("Send #%d: %v", i, err)
			}
		}
		if got := a.Snapshot().Value.Path(); got != step.want {
			t.Fatalf("step %d: got %q want %q", i, got, step.want)
		}
	}
}

func TestElevator_MatchesAtAnyDepth(t *testing.T) {
	m, _ := sc.CreateMachine(sc.MachineConfig[elevCtx, elevEvt]{
		ID:      "elev2",
		Initial: "door",
		States: map[string]sc.StateNodeConfig[elevCtx, elevEvt]{
			"door": {
				Initial: "closed",
				States: map[string]sc.StateNodeConfig[elevCtx, elevEvt]{
					"closed": {Initial: "idle", States: map[string]sc.StateNodeConfig[elevCtx, elevEvt]{
						"idle":   {},
						"moving": {},
					}},
				},
			},
		},
	})
	snap := sc.NewActor(m).Snapshot()
	cases := []struct {
		target string
		want   bool
	}{
		{"door", true},
		{"door.closed", true},
		{"door.closed.idle", true},
		{"door.closed.moving", false},
		{"door.open", false},
	}
	for _, c := range cases {
		if got := snap.Matches(c.target); got != c.want {
			t.Errorf("Matches(%q): got %v want %v", c.target, got, c.want)
		}
	}
}
