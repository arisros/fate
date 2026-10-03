package engine_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/arisros/fate/action"
	"github.com/arisros/fate/engine"
	"github.com/arisros/fate/render"
)

// These tests exercise the public surface that the behavioural and property
// suites don't reach incidentally: the Cond combinators, actor lifecycle
// (Stop), Log / EnqueueActions, and the rendering / introspection helpers.

func TestCondCombinators(t *testing.T) {
	in := action.InState("par.r1.y")
	cases := []struct {
		name string
		cond action.Cond
		// machine drives r1 to y first, then evaluates via a transition.
		wantQ bool
	}{
		{"not", action.CondNot(in), false},                                // we WILL be in y, so Not(in) is false
		{"allOf", action.CondAllOf(in, action.InState("par.r2.p")), true}, // both hold at decision time
		{"anyOf", action.CondAnyOf(action.InState("nope"), in), true},     // second holds
		{"anyOfNone", action.CondAnyOf(action.InState("nope")), false},
		{"allOfEmpty", action.CondAllOf(), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := engine.CreateMachine(engine.MachineConfig[condCtx, condEvt]{
				ID:      "root",
				Initial: "par",
				States: map[string]engine.StateNodeConfig[condCtx, condEvt]{
					"par": {
						Type: engine.NodeParallel,
						States: map[string]engine.StateNodeConfig[condCtx, condEvt]{
							"r1": {Initial: "x", States: map[string]engine.StateNodeConfig[condCtx, condEvt]{
								"x": {On: map[string][]engine.TransitionConfig[condCtx, condEvt]{"condAdvance": {{Target: "y"}}}},
								"y": {},
							}},
							"r2": {Initial: "p", States: map[string]engine.StateNodeConfig[condCtx, condEvt]{
								"p": {On: map[string][]engine.TransitionConfig[condCtx, condEvt]{
									"condCheck": {{Target: "q", Cond: tc.cond}, {Target: "blocked"}},
								}},
								"q":       {},
								"blocked": {},
							}},
						},
					},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			a := engine.NewActor(m)
			_ = a.Start(context.Background())
			_ = a.Send(context.Background(), condAdvance{}) // r1 -> y
			_ = a.Send(context.Background(), condCheck{})
			got := a.Snapshot().Matches("par.r2.q")
			if got != tc.wantQ {
				t.Fatalf("%s: reached q = %v, want %v (state %s)", tc.name, got, tc.wantQ, a.Snapshot().Value.Path())
			}
		})
	}
}

type logCtx struct{ N int }
type logEvt interface{ isLog() }
type logGo struct{}

func (logGo) isLog() {}

func TestLogAndEnqueueActionsAndStop(t *testing.T) {
	var logs []string
	m, err := engine.CreateMachine(engine.MachineConfig[logCtx, logEvt]{
		ID:      "log",
		Initial: "a",
		States: map[string]engine.StateNodeConfig[logCtx, logEvt]{
			"a": {
				On: map[string][]engine.TransitionConfig[logCtx, logEvt]{
					"logGo": {{Target: "b", Actions: []action.Action[logCtx, logEvt]{
						action.Log[logCtx, logEvt]("transitioning"),
						action.EnqueueActions(func(enq *action.Enqueuer[logCtx, logEvt]) {
							enq.Assign(func(c logCtx, _ logEvt) logCtx { c.N += 5; return c })
							enq.Log("enqueued")
							_ = enq.Context()
						}),
					}}},
				},
			},
			"b": {After: map[time.Duration][]engine.TransitionConfig[logCtx, logEvt]{
				time.Second: {{Target: "a"}},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	a := engine.NewActor(m, engine.WithLogger(func(s string) { logs = append(logs, s) }))
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), logGo{})
	if a.Snapshot().Context.N != 5 {
		t.Fatalf("EnqueueActions assign should set N=5, got %d", a.Snapshot().Context.N)
	}
	if len(logs) < 2 {
		t.Fatalf("expected Log + Enqueuer.Log entries, got %v", logs)
	}
	// In state b there is a pending timer; Stop must disarm it.
	if len(a.PendingTimers()) != 1 {
		t.Fatalf("expected one pending timer in b")
	}
	a.Stop()
	if len(a.PendingTimers()) != 0 {
		t.Fatalf("Stop must cancel pending timers")
	}
	if err := a.Send(context.Background(), logGo{}); err == nil {
		t.Fatal("Send after Stop should error")
	}
}

func TestIntrospectionAndRendering(t *testing.T) {
	m := afterMachine(t) // has events, after, guards, final, actions
	if m.ID() != "after" {
		t.Fatalf("Machine.ID = %q", m.ID())
	}
	d := m.Describe()
	if d.ID != "after" || len(d.States) == 0 {
		t.Fatalf("Describe returned empty descriptor")
	}
	ascii := render.ASCII(d, render.Options{})
	if !strings.Contains(ascii, "after") {
		t.Fatalf("render.ASCII missing machine id:\n%s", ascii)
	}
	mer := render.Mermaid(d, render.MermaidOptions{})
	if !strings.Contains(mer, "stateDiagram") {
		t.Fatalf("render.Mermaid missing diagram header:\n%s", mer)
	}
	g := render.GraphJSON(d)
	if len(g.Nodes) == 0 || g.ID != "after" {
		t.Fatalf("render.GraphJSON returned empty graph: %+v", g)
	}
}
