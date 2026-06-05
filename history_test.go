package fate_test

import (
	"context"
	"testing"

	sc "github.com/arisros/fate"
)

// Tape-player metaphor for history. A `player` compound has two modes:
// `stopped` (single leaf) and `playing`, which is itself compound with
// `normal` / `rewind` / `forward` children. When transitioning from
// `player` to `paused` (outside) and back via the history pseudo-state,
// the last active mode must be restored.

type tapeCtx struct{}
type tapeEvt interface{ isTapeEvt() }

type evtPlay struct{}
type evtPause struct{}
type evtRewind struct{}
type evtResume struct{}

func (evtPlay) isTapeEvt()          {}
func (evtPause) isTapeEvt()         {}
func (evtRewind) isTapeEvt()        {}
func (evtResume) isTapeEvt()        {}
func (evtPlay) EventName() string   { return "PLAY" }
func (evtPause) EventName() string  { return "PAUSE" }
func (evtRewind) EventName() string { return "REWIND" }
func (evtResume) EventName() string { return "RESUME" }

func TestHistory_ShallowRestoresLastChild(t *testing.T) {
	m, err := sc.CreateMachine(sc.MachineConfig[tapeCtx, tapeEvt]{
		ID:      "tape",
		Initial: "player",
		States: map[string]sc.StateNodeConfig[tapeCtx, tapeEvt]{
			"player": {
				Initial: "stopped",
				On: map[string][]sc.TransitionConfig[tapeCtx, tapeEvt]{
					"PAUSE": {{Target: "paused"}},
				},
				States: map[string]sc.StateNodeConfig[tapeCtx, tapeEvt]{
					"stopped": {
						On: map[string][]sc.TransitionConfig[tapeCtx, tapeEvt]{
							"PLAY": {{Target: "playing"}},
						},
					},
					"playing": {
						On: map[string][]sc.TransitionConfig[tapeCtx, tapeEvt]{
							"REWIND": {{Target: "rewinding"}},
						},
					},
					"rewinding": {
						On: map[string][]sc.TransitionConfig[tapeCtx, tapeEvt]{
							"PLAY": {{Target: "playing"}},
						},
					},
					"hist": {
						Type: sc.NodeHistory,
						// History: HistoryShallow (zero value)
					},
				},
			},
			"paused": {
				On: map[string][]sc.TransitionConfig[tapeCtx, tapeEvt]{
					"RESUME": {{Target: "player.hist"}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	a := sc.NewActor(m)
	_ = a.Start(context.Background())
	check := func(want string) {
		t.Helper()
		if got := a.Snapshot().Value.Path(); got != want {
			t.Errorf("got %q want %q", got, want)
		}
	}

	check("player.stopped")
	_ = a.Send(context.Background(), evtPlay{})
	check("player.playing")
	_ = a.Send(context.Background(), evtRewind{})
	check("player.rewinding")

	// Pause: leaving the player compound. History should record "rewinding".
	_ = a.Send(context.Background(), evtPause{})
	check("paused")

	// Resume via history pseudo-state — must restore "rewinding".
	_ = a.Send(context.Background(), evtResume{})
	check("player.rewinding")
}

func TestHistory_FallsBackToDefault(t *testing.T) {
	// When no memory exists yet (first entry through history), the default
	// target on the history node is used.
	m, err := sc.CreateMachine(sc.MachineConfig[tapeCtx, tapeEvt]{
		ID:      "tape-default",
		Initial: "paused", // start outside the player; no history recorded yet
		States: map[string]sc.StateNodeConfig[tapeCtx, tapeEvt]{
			"paused": {
				On: map[string][]sc.TransitionConfig[tapeCtx, tapeEvt]{
					"RESUME": {{Target: "player.hist"}},
				},
			},
			"player": {
				Initial: "stopped",
				States: map[string]sc.StateNodeConfig[tapeCtx, tapeEvt]{
					"stopped": {},
					"playing": {},
					"hist": {
						Type:    sc.NodeHistory,
						Default: "playing",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	a := sc.NewActor(m)
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), evtResume{})
	if got := a.Snapshot().Value.Path(); got != "player.playing" {
		t.Errorf("default fallback: got %q want %q", got, "player.playing")
	}
}

func TestHistory_FallsBackToInitialWhenNoDefault(t *testing.T) {
	m, err := sc.CreateMachine(sc.MachineConfig[tapeCtx, tapeEvt]{
		ID:      "tape-initial",
		Initial: "paused",
		States: map[string]sc.StateNodeConfig[tapeCtx, tapeEvt]{
			"paused": {
				On: map[string][]sc.TransitionConfig[tapeCtx, tapeEvt]{
					"RESUME": {{Target: "player.hist"}},
				},
			},
			"player": {
				Initial: "stopped",
				States: map[string]sc.StateNodeConfig[tapeCtx, tapeEvt]{
					"stopped": {},
					"hist":    {Type: sc.NodeHistory},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	a := sc.NewActor(m)
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), evtResume{})
	if got := a.Snapshot().Value.Path(); got != "player.stopped" {
		t.Errorf("initial fallback: got %q want %q", got, "player.stopped")
	}
}

func TestHistory_RejectsNestedStates(t *testing.T) {
	_, err := sc.CreateMachine(sc.MachineConfig[tapeCtx, tapeEvt]{
		ID:      "bad",
		Initial: "p",
		States: map[string]sc.StateNodeConfig[tapeCtx, tapeEvt]{
			"p": {
				Initial: "x",
				States: map[string]sc.StateNodeConfig[tapeCtx, tapeEvt]{
					"x": {},
					"hist": {
						Type:    sc.NodeHistory,
						Initial: "y",
						States:  map[string]sc.StateNodeConfig[tapeCtx, tapeEvt]{"y": {}},
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected error for history state with nested States")
	}
}
