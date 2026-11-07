package engine_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/arisros/fate/action"
	"github.com/arisros/fate/effect"
	"github.com/arisros/fate/engine"
)

type stressCtx struct{ Trace []string }

type stressCfg = engine.StateNodeConfig[stressCtx, string]

type stressOn = map[string][]engine.TransitionConfig[stressCtx, string]

var stressRegions = []string{"r1", "r2", "r3", "r4", "r5", "r6", "r7", "r8"}

func stressLog(tag string) action.Action[stressCtx, string] {
	return action.Assign(func(c stressCtx, _ string) stressCtx {
		c.Trace = append(append([]string(nil), c.Trace...), tag)
		return c
	})
}

func stressState(name string, s stressCfg) stressCfg {
	s.Entry = append(s.Entry, stressLog("in:"+name))
	s.Exit = append(s.Exit, stressLog("out:"+name))
	return s
}

func stressTick(r string) []engine.TransitionConfig[stressCtx, string] {
	return []engine.TransitionConfig[stressCtx, string]{{Actions: []action.Action[stressCtx, string]{stressLog("tick:" + r)}}}
}

func stressRegion(r string) stressCfg {
	return stressState(r, stressCfg{
		Initial: "a",
		States: map[string]stressCfg{
			"a": stressState(r+".a", stressCfg{
				On:    stressOn{"NEXT": {{Target: "b"}}, "TICK": stressTick(r)},
				After: map[time.Duration][]engine.TransitionConfig[stressCtx, string]{time.Second: {{Target: "b"}}},
			}),
			"b": stressState(r+".b", stressCfg{
				Initial: "x",
				On:      stressOn{"LOADED:" + r: {{Target: "done"}}, "TICK": stressTick(r)},
				Invoke: []effect.Invocation[stressCtx, string]{{
					ID:     "load",
					Src:    "load",
					OnDone: func(any) string { return "LOADED:" + r },
				}},
				States: map[string]stressCfg{
					"x": stressState(r+".b.x", stressCfg{On: stressOn{"DEEP": {{Target: "y"}}}}),
					"y": stressState(r+".b.y", stressCfg{}),
				},
			}),
			"done": stressState(r+".done", stressCfg{Type: engine.NodeFinal}),
		},
	})
}

func stressMachine(t *testing.T) *engine.Machine[stressCtx, string] {
	t.Helper()
	regions := map[string]stressCfg{}
	for _, r := range stressRegions {
		regions[r] = stressRegion(r)
	}
	m, err := engine.CreateMachine(engine.MachineConfig[stressCtx, string]{
		ID:      "stress",
		Initial: "work",
		States: map[string]stressCfg{
			"work": stressState("work", stressCfg{
				Initial: "par",
				On:      stressOn{"PAUSE": {{Target: "paused"}}},
				States: map[string]stressCfg{
					"par": stressState("par", stressCfg{
						Type:   engine.NodeParallel,
						States: regions,
						OnDone: []engine.TransitionConfig[stressCtx, string]{{Target: "finished"}},
					}),
					"hist":     {Type: engine.NodeHistory, History: engine.HistoryDeep},
					"finished": stressState("finished", stressCfg{}),
				},
			}),
			"paused": stressState("paused", stressCfg{On: stressOn{"RESUME": {{Target: "work.hist"}}}}),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

type stressRecorder struct {
	t   *testing.T
	out strings.Builder
}

func (r *stressRecorder) watch(a *engine.Actor[stressCtx, string]) {
	a.SubscribeSteps(func(s engine.Step) {
		b, err := json.Marshal(s)
		if err != nil {
			r.t.Fatal(err)
		}
		fmt.Fprintf(&r.out, "step:%s\n", b)
	})
}

func (r *stressRecorder) effects(a *engine.Actor[stressCtx, string]) {
	fmt.Fprintf(&r.out, "timers:%v\ninvocations:%v\n", a.PendingTimers(), a.PendingInvocations())
}

func (r *stressRecorder) persist(a *engine.Actor[stressCtx, string]) []byte {
	b, err := a.Persist()
	if err != nil {
		r.t.Fatal(err)
	}
	fmt.Fprintf(&r.out, "persist:%s\n", b)
	return b
}

func stressRun(t *testing.T) (string, []string, string) {
	ctx := context.Background()
	m := stressMachine(t)
	rec := &stressRecorder{t: t}

	a := engine.NewActor(m)
	rec.watch(a)
	if err := a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	_ = a.Send(ctx, "TICK")
	rec.effects(a)
	for _, tm := range a.PendingTimers()[:3] {
		a.FireTimer(tm.ID)
	}
	for _, ev := range []string{"NEXT", "DEEP", "TICK", "PAUSE", "RESUME"} {
		_ = a.Send(ctx, ev)
	}
	rec.effects(a)
	blob := rec.persist(a)

	b, err := engine.NewActorFromSnapshot(m, blob)
	if err != nil {
		t.Fatal(err)
	}
	rec.watch(b)
	if err := b.Start(ctx); err != nil {
		t.Fatal(err)
	}
	rec.effects(b)
	for _, inv := range b.PendingInvocations() {
		b.ResolveInvocation(inv.ID, nil)
	}
	rec.effects(b)
	rec.persist(b)

	snap := b.Snapshot()
	return rec.out.String(), snap.Context.Trace, snap.Value.Path()
}

func TestDeterminismStress(t *testing.T) {
	want, trace, path := stressRun(t)
	if path != "work.finished" {
		t.Fatalf("final state = %q, want work.finished", path)
	}
	var entry []string
	for _, r := range stressRegions {
		entry = append(entry, "in:"+r, "in:"+r+".a")
	}
	if got := strings.Join(trace[2:2+len(entry)], " "); got != strings.Join(entry, " ") {
		t.Fatalf("regions entered as %s, want %s", got, strings.Join(entry, " "))
	}

	runs := 1000
	if testing.Short() {
		runs = 50
	}
	for i := range runs {
		if got, _, _ := stressRun(t); got != want {
			t.Fatalf("run %d differs from run 0:\n%s", i+1, firstDiff(want, got))
		}
	}
}

func firstDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(w) && i < len(g); i++ {
		if w[i] != g[i] {
			return fmt.Sprintf("line %d\nwant %s\ngot  %s", i+1, w[i], g[i])
		}
	}
	return fmt.Sprintf("want %d lines, got %d", len(w), len(g))
}
