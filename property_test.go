package fate_test

import (
	"bytes"
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"

	fate "github.com/arisros/fate"
)

// Property-based tests for the two core invariants from the determinism
// contract (ADR-0007 of the proof-of-concept):
//
//  1. Determinism — the same machine driven by the same operation sequence
//     yields a byte-identical persisted snapshot.
//  2. Persist/restore transparency — persisting mid-flight, restoring into a
//     fresh actor, and continuing yields the same snapshot as never persisting.
//
// We use a seeded math/rand source rather than a third-party property library
// so the module stays zero-dependency even under test. Each failing seed is
// printed, making failures reproducible.

type propCtx struct{ Jobs int }

// propMachine exercises the persistence-sensitive features at once: parallel
// regions, a delayed (after) transition, an invocation, guards, and context
// assignment. Events are plain strings.
func propMachine(t *testing.T) *fate.Machine[propCtx, string] {
	t.Helper()
	m, err := fate.CreateMachine(fate.MachineConfig[propCtx, string]{
		ID:      "prop",
		Initial: "active",
		States: map[string]fate.StateNodeConfig[propCtx, string]{
			"active": {
				Type: fate.NodeParallel,
				States: map[string]fate.StateNodeConfig[propCtx, string]{
					"main": {
						Initial: "idle",
						States: map[string]fate.StateNodeConfig[propCtx, string]{
							"idle": {
								On: map[string][]fate.TransitionConfig[propCtx, string]{
									"GO": {{Target: "busy"}},
								},
								After: map[time.Duration][]fate.TransitionConfig[propCtx, string]{
									10 * time.Millisecond: {{Target: "busy"}},
								},
							},
							"busy": {
								Invoke: []fate.Invocation[propCtx, string]{{
									ID:      "job",
									Src:     "svc",
									Input:   func(c propCtx) any { return c.Jobs },
									OnDone:  func(any) string { return "JOB_DONE" },
									OnError: func(error) string { return "JOB_FAIL" },
								}},
								On: map[string][]fate.TransitionConfig[propCtx, string]{
									"CANCEL":   {{Target: "idle"}},
									"JOB_FAIL": {{Target: "idle"}},
									"JOB_DONE": {{
										Target: "reported",
										Actions: []fate.Action[propCtx, string]{
											fate.Assign(func(c propCtx, _ string) propCtx { c.Jobs++; return c }),
										},
									}},
								},
							},
							"reported": {
								On: map[string][]fate.TransitionConfig[propCtx, string]{
									"AGAIN": {{Target: "idle"}},
								},
							},
						},
					},
					"audit": {
						Initial: "watching",
						States: map[string]fate.StateNodeConfig[propCtx, string]{
							"watching": {On: map[string][]fate.TransitionConfig[propCtx, string]{
								"FLAG": {{Target: "flagged"}},
							}},
							"flagged": {On: map[string][]fate.TransitionConfig[propCtx, string]{
								"CLEAR": {{Target: "watching"}},
							}},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("create prop machine: %v", err)
	}
	return m
}

type opKind int

const (
	opEvent opKind = iota
	opTimer
	opInvokeOK
	opInvokeErr
)

type op struct {
	kind  opKind
	event string
}

var propEvents = []string{"GO", "CANCEL", "FLAG", "CLEAR", "AGAIN", "NOOP"}

func genOps(r *rand.Rand, n int) []op {
	ops := make([]op, n)
	for i := range ops {
		switch r.Intn(4) {
		case 0:
			ops[i] = op{kind: opEvent, event: propEvents[r.Intn(len(propEvents))]}
		case 1:
			ops[i] = op{kind: opTimer}
		case 2:
			ops[i] = op{kind: opInvokeOK}
		default:
			ops[i] = op{kind: opInvokeErr}
		}
	}
	return ops
}

func applyOp(a *fate.Actor[propCtx, string], o op) {
	ctx := context.Background()
	switch o.kind {
	case opEvent:
		_ = a.Send(ctx, o.event)
	case opTimer:
		if p := a.PendingTimers(); len(p) > 0 {
			a.FireTimer(p[0].ID)
		}
	case opInvokeOK:
		if p := a.PendingInvocations(); len(p) > 0 {
			a.ResolveInvocation(p[0].ID, true)
		}
	case opInvokeErr:
		if p := a.PendingInvocations(); len(p) > 0 {
			a.RejectInvocation(p[0].ID, errors.New("x"))
		}
	}
}

func runOps(t *testing.T, ops []op) *fate.Actor[propCtx, string] {
	a := fate.NewActor(propMachine(t))
	_ = a.Start(context.Background())
	for _, o := range ops {
		applyOp(a, o)
	}
	return a
}

func TestProperty_Determinism(t *testing.T) {
	for seed := int64(0); seed < 300; seed++ {
		ops := genOps(rand.New(rand.NewSource(seed)), 40)
		a1 := runOps(t, ops)
		a2 := runOps(t, ops)
		b1, err := a1.PersistDeterministic()
		if err != nil {
			t.Fatalf("seed %d: persist: %v", seed, err)
		}
		b2, err := a2.PersistDeterministic()
		if err != nil {
			t.Fatalf("seed %d: persist: %v", seed, err)
		}
		if !bytes.Equal(b1, b2) {
			t.Fatalf("seed %d: non-deterministic persist:\n a=%s\n b=%s", seed, b1, b2)
		}
	}
}

func TestProperty_PersistRoundTripTransparent(t *testing.T) {
	for seed := int64(0); seed < 300; seed++ {
		r := rand.New(rand.NewSource(seed + 1000))
		ops := genOps(r, 40)
		split := r.Intn(len(ops) + 1)
		prefix, suffix := ops[:split], ops[split:]

		// Path A: run prefix, continue suffix without persisting.
		a := runOps(t, prefix)
		for _, o := range suffix {
			applyOp(a, o)
		}
		want, err := a.PersistDeterministic()
		if err != nil {
			t.Fatalf("seed %d: persist A: %v", seed, err)
		}

		// Path B: run prefix, persist, restore, continue suffix.
		b := runOps(t, prefix)
		blob, err := b.Persist()
		if err != nil {
			t.Fatalf("seed %d: persist B mid: %v", seed, err)
		}
		restored, err := fate.NewActorFromSnapshot[propCtx, string](propMachine(t), blob)
		if err != nil {
			t.Fatalf("seed %d: restore: %v", seed, err)
		}
		for _, o := range suffix {
			applyOp(restored, o)
		}
		got, err := restored.PersistDeterministic()
		if err != nil {
			t.Fatalf("seed %d: persist B final: %v", seed, err)
		}

		if !bytes.Equal(want, got) {
			t.Fatalf("seed %d: restore not transparent (split=%d):\n want=%s\n got =%s",
				seed, split, want, got)
		}
	}
}

func TestProperty_PersistStable(t *testing.T) {
	// Persisting the same actor twice must be byte-identical.
	for seed := int64(0); seed < 100; seed++ {
		ops := genOps(rand.New(rand.NewSource(seed+5000)), 25)
		a := runOps(t, ops)
		b1, _ := a.PersistDeterministic()
		b2, _ := a.PersistDeterministic()
		if !bytes.Equal(b1, b2) {
			t.Fatalf("seed %d: persist not stable across calls", seed)
		}
	}
}
