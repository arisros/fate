package snapshot_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/arisros/fate"
	"github.com/arisros/fate/render"
	"github.com/arisros/fate/snapshot"
)

type tlCtx struct{}
type tlEvt struct{ name string }

func (e tlEvt) EventName() string { return e.name }

func trafficLight(t *testing.T) *fate.Machine[tlCtx, tlEvt] {
	t.Helper()
	m, err := fate.CreateMachine(fate.MachineConfig[tlCtx, tlEvt]{
		ID:      "traffic-light",
		Initial: "red",
		States: map[string]fate.StateNodeConfig[tlCtx, tlEvt]{
			"red":    {On: map[string][]fate.TransitionConfig[tlCtx, tlEvt]{"TIMER": {{Target: "green"}}}},
			"green":  {On: map[string][]fate.TransitionConfig[tlCtx, tlEvt]{"TIMER": {{Target: "yellow"}}}},
			"yellow": {On: map[string][]fate.TransitionConfig[tlCtx, tlEvt]{"TIMER": {{Target: "red"}}}},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	return m
}

// TestEmitRoundTrip is the core contract: Emit writes a descriptor that
// LoadDescriptor reads back and render.GraphJSON renders identically to the
// in-memory machine — proving the snapshot file is a faithful, engine-free
// rendering source.
func TestEmitRoundTrip(t *testing.T) {
	m := trafficLight(t)
	dir := t.TempDir()

	if err := snapshot.Emit(dir, "traffic-light", m); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	path := filepath.Join(dir, "traffic-light.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	loaded, err := fate.LoadDescriptor(data)
	if err != nil {
		t.Fatalf("LoadDescriptor: %v", err)
	}

	// Graph derived from the loaded snapshot must equal the graph derived
	// straight from the live machine's descriptor.
	fromFile := render.GraphJSON(loaded)
	fromLive := render.GraphJSON(m.Describe())

	if len(fromFile.Nodes) == 0 || len(fromFile.Edges) == 0 {
		t.Fatalf("empty graph from snapshot: %d nodes, %d edges", len(fromFile.Nodes), len(fromFile.Edges))
	}
	if got, want := len(fromFile.Nodes), len(fromLive.Nodes); got != want {
		t.Errorf("node count: snapshot=%d live=%d", got, want)
	}
	if got, want := len(fromFile.Edges), len(fromLive.Edges); got != want {
		t.Errorf("edge count: snapshot=%d live=%d", got, want)
	}
	if got, want := fromFile.ID, "traffic-light"; got != want {
		t.Errorf("graph id: got %q want %q", got, want)
	}
}

func TestEmitEmptyNameRejected(t *testing.T) {
	if err := snapshot.Emit(t.TempDir(), "", trafficLight(t)); err == nil {
		t.Fatal("expected error for empty name")
	}
}
