package fate_test

import (
	"context"
	"strings"
	"testing"
	"time"

	fate "github.com/arisros/fate"
)

// These tests exercise the public surface that the behavioural and property
// suites don't reach incidentally: the Cond combinators, actor lifecycle
// (Stop), Log / EnqueueActions, and the rendering / introspection helpers.

func TestCondCombinators(t *testing.T) {
	in := fate.InState("par.r1.y")
	cases := []struct {
		name string
		cond fate.Cond
		// machine drives r1 to y first, then evaluates via a transition.
		wantQ bool
	}{
		{"not", fate.CondNot(in), false},                              // we WILL be in y, so Not(in) is false
		{"allOf", fate.CondAllOf(in, fate.InState("par.r2.p")), true}, // both hold at decision time
		{"anyOf", fate.CondAnyOf(fate.InState("nope"), in), true},     // second holds
		{"anyOfNone", fate.CondAnyOf(fate.InState("nope")), false},
		{"allOfEmpty", fate.CondAllOf(), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := fate.CreateMachine(fate.MachineConfig[condCtx, condEvt]{
				ID:      "root",
				Initial: "par",
				States: map[string]fate.StateNodeConfig[condCtx, condEvt]{
					"par": {
						Type: fate.NodeParallel,
						States: map[string]fate.StateNodeConfig[condCtx, condEvt]{
							"r1": {Initial: "x", States: map[string]fate.StateNodeConfig[condCtx, condEvt]{
								"x": {On: map[string][]fate.TransitionConfig[condCtx, condEvt]{"condAdvance": {{Target: "y"}}}},
								"y": {},
							}},
							"r2": {Initial: "p", States: map[string]fate.StateNodeConfig[condCtx, condEvt]{
								"p": {On: map[string][]fate.TransitionConfig[condCtx, condEvt]{
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
			a := fate.NewActor(m)
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
	m, err := fate.CreateMachine(fate.MachineConfig[logCtx, logEvt]{
		ID:      "log",
		Initial: "a",
		States: map[string]fate.StateNodeConfig[logCtx, logEvt]{
			"a": {
				On: map[string][]fate.TransitionConfig[logCtx, logEvt]{
					"logGo": {{Target: "b", Actions: []fate.Action[logCtx, logEvt]{
						fate.Log[logCtx, logEvt]("transitioning"),
						fate.EnqueueActions(func(enq *fate.Enqueuer[logCtx, logEvt]) {
							enq.Assign(func(c logCtx, _ logEvt) logCtx { c.N += 5; return c })
							enq.Log("enqueued")
							_ = enq.Context()
						}),
					}}},
				},
			},
			"b": {After: map[time.Duration][]fate.TransitionConfig[logCtx, logEvt]{
				time.Second: {{Target: "a"}},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	a := fate.NewActor(m, fate.WithLogger(func(s string) { logs = append(logs, s) }))
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
	ascii := fate.RenderASCII(d, fate.RenderOptions{})
	if !strings.Contains(ascii, "after") {
		t.Fatalf("RenderASCII missing machine id:\n%s", ascii)
	}
	mer := fate.RenderMermaid(d, fate.MermaidOptions{})
	if !strings.Contains(mer, "stateDiagram") {
		t.Fatalf("RenderMermaid missing diagram header:\n%s", mer)
	}
	g := fate.RenderGraphJSON(d)
	if len(g.Nodes) == 0 || g.ID != "after" {
		t.Fatalf("RenderGraphJSON returned empty graph: %+v", g)
	}
}
