package testing_test

import (
	"testing"

	fate "github.com/arisros/fate"
	fatetest "github.com/arisros/fate/testing"
)

func machine(t *testing.T) *fate.Machine[struct{}, string] {
	t.Helper()
	m, err := fate.CreateMachine(fate.MachineConfig[struct{}, string]{
		ID:      "tl",
		Initial: "a",
		States: map[string]fate.StateNodeConfig[struct{}, string]{
			"a": {On: map[string][]fate.TransitionConfig[struct{}, string]{"GO": {{Target: "b"}}}},
			"b": {On: map[string][]fate.TransitionConfig[struct{}, string]{"GO": {{Target: "c"}}}},
			"c": {Type: fate.NodeFinal},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestTraceAndPaths(t *testing.T) {
	a := fate.NewActor(machine(t))
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
	a := fate.NewActor(machine(t))
	_ = a.Start(fatetest.DefaultContext())
	_ = a.Send(fatetest.DefaultContext(), "GO")

	snap, err := fatetest.WaitFor(a, func(s fate.Snapshot[struct{}]) bool {
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
	a := fate.NewActor(machine(t))
	_ = a.Start(fatetest.DefaultContext())
	_, err := fatetest.WaitFor(a, func(fate.Snapshot[struct{}]) bool { return false }, 5_000_000) // 5ms
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
