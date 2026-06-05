package fate_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	sc "github.com/arisros/fate"
)

// persistCtx is a typical task context: accumulating form data fields and a
// few flags. Must be JSON-marshalable (exported fields).
type persistCtx struct {
	Step      int               `json:"step"`
	Form      map[string]string `json:"form"`
	Flags     []string          `json:"flags"`
	Submitted bool              `json:"submitted"`
}

type persistEvt interface{ isPersistEvt() }
type evtStep struct{ Field, Value string }
type evtSubmit struct{}

func (evtStep) isPersistEvt()       {}
func (evtSubmit) isPersistEvt()     {}
func (evtStep) EventName() string   { return "STEP" }
func (evtSubmit) EventName() string { return "SUBMIT" }

func newPersistMachine(t *testing.T) *sc.Machine[persistCtx, persistEvt] {
	t.Helper()
	m, err := sc.CreateMachine(sc.MachineConfig[persistCtx, persistEvt]{
		ID:      "persist-form",
		Initial: "filling",
		Context: persistCtx{Form: map[string]string{}, Flags: []string{}},
		States: map[string]sc.StateNodeConfig[persistCtx, persistEvt]{
			"filling": {
				On: map[string][]sc.TransitionConfig[persistCtx, persistEvt]{
					"STEP": {{
						Target: "filling",
						Actions: []sc.Action[persistCtx, persistEvt]{
							sc.Assign(func(c persistCtx, e persistEvt) persistCtx {
								if step, ok := e.(evtStep); ok {
									c.Form[step.Field] = step.Value
									c.Step++
								}
								return c
							}),
						},
					}},
					"SUBMIT": {{
						Target: "submitted",
						Actions: []sc.Action[persistCtx, persistEvt]{
							sc.Assign(func(c persistCtx, _ persistEvt) persistCtx {
								c.Submitted = true
								return c
							}),
						},
					}},
				},
			},
			"submitted": {Type: sc.NodeFinal},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	return m
}

func TestPersist_RoundTripPreservesState(t *testing.T) {
	m := newPersistMachine(t)
	a := sc.NewActor(m)
	_ = a.Start(context.Background())

	// Drive a few events to accumulate state.
	_ = a.Send(context.Background(), evtStep{Field: "name", Value: "Aris"})
	_ = a.Send(context.Background(), evtStep{Field: "age", Value: "30"})

	persisted, err := a.Persist()
	if err != nil {
		t.Fatalf("Persist: %v", err)
	}

	// Restore into a fresh actor.
	a2, err := sc.NewActorFromSnapshot(m, persisted)
	if err != nil {
		t.Fatalf("NewActorFromSnapshot: %v", err)
	}

	// Snapshots must match.
	s1 := a.Snapshot()
	s2 := a2.Snapshot()
	if s1.Value.Path() != s2.Value.Path() {
		t.Errorf("value path: original %q vs restored %q", s1.Value.Path(), s2.Value.Path())
	}
	if s1.Status != s2.Status {
		t.Errorf("status: original %q vs restored %q", s1.Status, s2.Status)
	}
	if s1.Context.Step != s2.Context.Step {
		t.Errorf("step: original %d vs restored %d", s1.Context.Step, s2.Context.Step)
	}
	if s1.Context.Form["name"] != s2.Context.Form["name"] || s1.Context.Form["age"] != s2.Context.Form["age"] {
		t.Errorf("form: original %v vs restored %v", s1.Context.Form, s2.Context.Form)
	}

	// Continuing both actors with the same events must yield the same final state.
	_ = a.Send(context.Background(), evtSubmit{})
	_ = a2.Send(context.Background(), evtSubmit{})
	if a.Snapshot().Status != a2.Snapshot().Status {
		t.Errorf("status divergence after replay: orig %q vs restored %q", a.Snapshot().Status, a2.Snapshot().Status)
	}
	if a.Snapshot().Value.Path() != a2.Snapshot().Value.Path() {
		t.Errorf("value divergence after replay: orig %q vs restored %q", a.Snapshot().Value.Path(), a2.Snapshot().Value.Path())
	}
}

func TestPersist_DeterministicBytes(t *testing.T) {
	m := newPersistMachine(t)
	a := sc.NewActor(m)
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), evtStep{Field: "k1", Value: "v1"})
	_ = a.Send(context.Background(), evtStep{Field: "k2", Value: "v2"})
	_ = a.Send(context.Background(), evtStep{Field: "k3", Value: "v3"})

	b1, err := a.Persist()
	if err != nil {
		t.Fatalf("Persist 1: %v", err)
	}

	// Persist again from the same actor — must be byte-identical.
	b2, err := a.Persist()
	if err != nil {
		t.Fatalf("Persist 2: %v", err)
	}
	if !bytes.Equal(b1, b2) {
		t.Errorf("non-deterministic Persist:\n  b1=%s\n  b2=%s", string(b1), string(b2))
	}

	// Restore and persist — also byte-identical.
	a2, err := sc.NewActorFromSnapshot(m, b1)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	b3, err := a2.Persist()
	if err != nil {
		t.Fatalf("Persist 3: %v", err)
	}
	if !bytes.Equal(b1, b3) {
		t.Errorf("Restore+Persist not byte-identical:\n  orig=%s\n  rest=%s", string(b1), string(b3))
	}
}

func TestPersist_RejectsFutureVersion(t *testing.T) {
	m := newPersistMachine(t)
	bad := []byte(`{"version":99,"status":"running","value":"filling","context":{"step":0,"form":{},"flags":null,"submitted":false}}`)
	_, err := sc.NewActorFromSnapshot(m, bad)
	if err == nil || !strings.Contains(err.Error(), "newer than supported") {
		t.Errorf("expected version-mismatch error, got %v", err)
	}
}

func TestPersist_PreservesHistoryMemory(t *testing.T) {
	// Use the tape-player history machine from history_test.go.
	m, err := sc.CreateMachine(sc.MachineConfig[tapeCtx, tapeEvt]{
		ID:      "tape-persist",
		Initial: "player",
		States: map[string]sc.StateNodeConfig[tapeCtx, tapeEvt]{
			"player": {
				Initial: "stopped",
				On: map[string][]sc.TransitionConfig[tapeCtx, tapeEvt]{
					"PAUSE": {{Target: "paused"}},
				},
				States: map[string]sc.StateNodeConfig[tapeCtx, tapeEvt]{
					"stopped":   {On: map[string][]sc.TransitionConfig[tapeCtx, tapeEvt]{"PLAY": {{Target: "playing"}}}},
					"playing":   {On: map[string][]sc.TransitionConfig[tapeCtx, tapeEvt]{"REWIND": {{Target: "rewinding"}}}},
					"rewinding": {On: map[string][]sc.TransitionConfig[tapeCtx, tapeEvt]{"PLAY": {{Target: "playing"}}}},
					"hist":      {Type: sc.NodeHistory},
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
	_ = a.Send(context.Background(), evtPlay{})
	_ = a.Send(context.Background(), evtRewind{})
	_ = a.Send(context.Background(), evtPause{}) // records history: player → rewinding

	persisted, err := a.Persist()
	if err != nil {
		t.Fatalf("Persist: %v", err)
	}
	a2, err := sc.NewActorFromSnapshot(m, persisted)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// Resume on the restored actor — must land on "rewinding" (history memory).
	_ = a2.Send(context.Background(), evtResume{})
	if got := a2.Snapshot().Value.Path(); got != "player.rewinding" {
		t.Errorf("history memory lost after persist: got %q want %q", got, "player.rewinding")
	}
}
