package fate

import (
	"context"
	"testing"
)

type depCtx struct{ Count int }

func TestDeprecatedAliasesDriveTheEngine(t *testing.T) {
	isLow := func(c depCtx, _ string) bool { return c.Count < 2 }
	m, err := NewSetup[depCtx, string]().
		WithGuard("low", And(isLow, Or(AlwaysTrue[depCtx, string](), Not(isLow)))).
		CreateMachine(MachineConfig[depCtx, string]{
			ID:      "counter",
			Initial: "active",
			States: map[string]StateNodeConfig[depCtx, string]{
				"active": {On: map[string][]TransitionConfig[depCtx, string]{
					"INC": {{
						Guard: isLow,
						Cond:  CondAllOf(InState("active"), CondAnyOf(StateIn("active")), CondNot(InState("done"))),
						Actions: []Action[depCtx, string]{
							Named("bump", Assign(func(c depCtx, _ string) depCtx { c.Count++; return c })),
							Log[depCtx, string]("bumped"),
							EnqueueActions(func(enq *Enqueuer[depCtx, string]) {}),
						},
						CondMeta: Gates(Field("$.Count").Lt(2)).Build(),
					}},
					"FINISH": {{Target: "done", Actions: []Action[depCtx, string]{Raise[depCtx, string]("NOOP")}}},
				}},
				"done": {Type: NodeFinal},
			},
		})
	if err != nil {
		t.Fatal(err)
	}

	a := NewActor(m, WithLogger(func(string) {}))
	ctx := context.Background()
	if err := a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := a.Send(ctx, "INC"); err != nil {
			t.Fatal(err)
		}
	}
	if got := a.Snapshot(); got.Context.Count != 2 || got.Status != StatusRunning {
		t.Fatalf("snapshot = %+v, want Count 2 and running", got)
	}

	blob, err := a.Persist()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := NewActorFromSnapshot(m, blob)
	if err != nil {
		t.Fatal(err)
	}
	if err := restored.Send(ctx, "FINISH"); err != nil {
		t.Fatal(err)
	}
	if got := restored.Snapshot(); got.Status != StatusDone || !got.Value.Matches("done") {
		t.Fatalf("snapshot = %+v, want done", got)
	}

	seeded := NewActor(m, WithInitialValue[depCtx, string](AtomicValue("done")))
	if got := seeded.Snapshot().Value; got.Path() != "done" {
		t.Fatalf("seeded value = %q, want done", got.Path())
	}
	if got := CompoundValue(map[string]StateValue{"a": AtomicValue("b")}).Path(); got != "a.b" {
		t.Fatalf("compound path = %q, want a.b", got)
	}
}
