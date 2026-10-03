package testing_test

import (
	"testing"

	"github.com/arisros/fate/engine"
	"github.com/arisros/fate/persist"
	fatetest "github.com/arisros/fate/testing"
)

func machine(t *testing.T) *engine.Machine[struct{}, string] {
	t.Helper()
	m, err := engine.CreateMachine(engine.MachineConfig[struct{}, string]{
		ID:      "tl",
		Initial: "a",
		States: map[string]engine.StateNodeConfig[struct{}, string]{
			"a": {On: map[string][]engine.TransitionConfig[struct{}, string]{"GO": {{Target: "b"}}}},
			"b": {On: map[string][]engine.TransitionConfig[struct{}, string]{"GO": {{Target: "c"}}}},
			"c": {Type: engine.NodeFinal},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestTraceAndPaths(t *testing.T) {
	a := engine.NewActor(machine(t))
	tr := fatetest.NewTrace[struct{}, string](a)
	defer tr.Stop()

	_ = a.Start(fatetest.DefaultContext())
	_ = a.Send(fatetest.DefaultContext(), "GO")
	_ = a.Send(fatetest.DefaultContext(), "GO")

	paths := tr.Paths()
	want := []string{"a", "b", "c"}
	if len(paths) != len(want) {
		t.Fatalf("want %v, got %v", want, paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("step %d: want %q, got %q (%v)", i, want[i], paths[i], paths)
		}
	}
	tr.Stop() // idempotent
}

func TestWaitFor(t *testing.T) {
	a := engine.NewActor(machine(t))
	_ = a.Start(fatetest.DefaultContext())
	_ = a.Send(fatetest.DefaultContext(), "GO")

	snap, err := fatetest.WaitFor(a, func(s persist.Snapshot[struct{}]) bool {
		return s.Matches("b")
	}, 0)
	if err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if !snap.Matches("b") {
		t.Fatalf("want b, got %s", snap.Value.Path())
	}
}

func TestWaitForTimeout(t *testing.T) {
	a := engine.NewActor(machine(t))
	_ = a.Start(fatetest.DefaultContext())
	_, err := fatetest.WaitFor(a, func(persist.Snapshot[struct{}]) bool { return false }, 5_000_000) // 5ms
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
