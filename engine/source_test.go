package engine_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/arisros/fate/action"
	"github.com/arisros/fate/effect"
	"github.com/arisros/fate/engine"
)

type srcCtx struct{ N int }

type srcEvt struct{ Name string }

func (e srcEvt) EventName() string { return e.Name }

func srcIsPositive(c srcCtx, _ srcEvt) bool { return c.N > 0 }

func srcBump(c srcCtx, _ srcEvt) srcCtx { c.N++; return c }

type srcCustom struct{}

func (srcCustom) Apply(c srcCtx, _ srcEvt, _ action.Sink[srcEvt]) srcCtx { return c }

const srcFile = "github.com/arisros/fate/engine_test/source_test.go"

func srcMachine(t *testing.T) *engine.Machine[srcCtx, srcEvt] {
	t.Helper()
	m, err := engine.CreateMachine(engine.MachineConfig[srcCtx, srcEvt]{
		ID:      "src",
		Initial: "idle",
		States: map[string]engine.StateNodeConfig[srcCtx, srcEvt]{
			"idle": {
				Entry: []action.Action[srcCtx, srcEvt]{action.Log[srcCtx, srcEvt]("hi"), srcCustom{}},
				On: map[string][]engine.TransitionConfig[srcCtx, srcEvt]{
					"GO": {
						{Target: "busy", Guard: srcIsPositive, Actions: []action.Action[srcCtx, srcEvt]{
							action.Named("bump", action.Assign(srcBump)),
						}},
						{Target: "busy", Guard: action.Not(srcIsPositive)},
					},
					"PING": {{Actions: []action.Action[srcCtx, srcEvt]{action.Raise[srcCtx](srcEvt{Name: "GO"})}}},
				},
			},
			"busy": {
				Initial: "inner",
				States: map[string]engine.StateNodeConfig[srcCtx, srcEvt]{
					"inner": {Exit: []action.Action[srcCtx, srcEvt]{
						action.Assign(func(c srcCtx, _ srcEvt) srcCtx { return c }),
					}},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSources_LocatesMachineGuardsAndActions(t *testing.T) {
	src := srcMachine(t).Sources()

	if src.Machine == nil || src.Machine.File != srcFile || src.Machine.Line == 0 || src.Machine.Func != "" {
		t.Fatalf("machine location = %+v", src.Machine)
	}
	if !strings.HasSuffix(src.Machine.Path, "source_test.go") {
		t.Errorf("machine path = %q", src.Machine.Path)
	}

	idle := src.States["idle"]
	if len(idle.Entry) != 2 || idle.Entry[0] != nil {
		t.Fatalf("idle entry = %+v, want the Log unlocated", idle.Entry)
	}
	if got := idle.Entry[1]; got == nil || got.Func != "engine_test.srcCustom.Apply" || got.File != srcFile {
		t.Errorf("custom action = %+v", got)
	}

	gos := idle.On["GO"]
	if len(gos) != 2 {
		t.Fatalf("GO transitions = %d", len(gos))
	}
	if g := gos[0].Guard; g == nil || g.Func != "engine_test.srcIsPositive" || g.File != srcFile {
		t.Errorf("guard = %+v", g)
	}
	if a := gos[0].Actions; len(a) != 1 || a[0] == nil || a[0].Func != "engine_test.srcBump" {
		t.Errorf("named assign = %+v", a)
	}
	if gos[1].Guard != nil {
		t.Errorf("combinator guard located at %+v, want nil", gos[1].Guard)
	}
	if _, ok := idle.On["PING"]; ok {
		t.Error("an event with only a Raise should carry no sources")
	}

	inner := src.States["busy.inner"]
	if len(inner.Exit) != 1 || inner.Exit[0] == nil || !strings.HasPrefix(inner.Exit[0].Func, "engine_test.srcMachine.func") {
		t.Errorf("closure exit = %+v", inner.Exit)
	}
	if inner.Exit[0].Line <= src.Machine.Line {
		t.Errorf("closure line %d should follow the machine line %d", inner.Exit[0].Line, src.Machine.Line)
	}
	if _, ok := src.States["busy"]; ok {
		t.Error("a state with nothing located should be left out")
	}
}

func TestSources_SetupLocatesItsCaller(t *testing.T) {
	s := engine.NewSetup[srcCtx, srcEvt]().WithGuard("positive", srcIsPositive).WithAction("bump", action.Assign(srcBump))
	m, err := s.CreateMachine(engine.MachineConfig[srcCtx, srcEvt]{
		ID:      "setup",
		Initial: "a",
		States: map[string]engine.StateNodeConfig[srcCtx, srcEvt]{
			"a": {On: map[string][]engine.TransitionConfig[srcCtx, srcEvt]{
				"GO": {{Target: "b", Guard: s.Guard("positive"), Actions: []action.Action[srcCtx, srcEvt]{s.Action("bump")}}},
			}},
			"b": {},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	src := m.Sources()
	if src.Machine == nil || src.Machine.File != srcFile {
		t.Fatalf("machine location = %+v", src.Machine)
	}
	tr := src.States["a"].On["GO"][0]
	if tr.Guard == nil || tr.Guard.Func != "engine_test.srcIsPositive" {
		t.Errorf("registered guard = %+v", tr.Guard)
	}
	if tr.Actions[0] == nil || tr.Actions[0].Func != "engine_test.srcBump" {
		t.Errorf("registered action = %+v", tr.Actions[0])
	}
}

func TestSources_LeaveTheDescriptorUntouched(t *testing.T) {
	b, err := json.Marshal(srcMachine(t).Describe())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "source_test.go") {
		t.Error("the descriptor must not carry source locations")
	}
}

type srcChecker struct{ min int }

func (k srcChecker) Check(c srcCtx, _ srcEvt) bool { return c.N > k.min }

func srcOutput(c srcCtx) any { return c.N }

func srcInput(c srcCtx) any { return c.N }

func srcDone(any) srcEvt { return srcEvt{Name: "DONE"} }

func TestSources_LocatesAfterInvokeAndOutput(t *testing.T) {
	m, err := engine.CreateMachine(engine.MachineConfig[srcCtx, srcEvt]{
		ID:      "effects",
		Initial: "waiting",
		States: map[string]engine.StateNodeConfig[srcCtx, srcEvt]{
			"waiting": {
				After: map[time.Duration][]engine.TransitionConfig[srcCtx, srcEvt]{
					30 * time.Second: {{Target: "end", Guard: srcIsPositive, Actions: []action.Action[srcCtx, srcEvt]{action.Assign(srcBump)}}},
					time.Minute:      {{Target: "end"}},
				},
				Invoke: []effect.Invocation[srcCtx, srcEvt]{
					{ID: "plain", Src: "work"},
					{ID: "mapped", Src: "work", Input: srcInput, OnDone: srcDone},
				},
			},
			"end": {Type: engine.NodeFinal, Output: srcOutput},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	src := m.Sources()

	waiting := src.States["waiting"]
	after := waiting.After["30s"]
	if len(after) != 1 || after[0].Guard == nil || after[0].Guard.Func != "engine_test.srcIsPositive" {
		t.Fatalf("after 30s = %+v", after)
	}
	if a := after[0].Actions; len(a) != 1 || a[0] == nil || a[0].Func != "engine_test.srcBump" {
		t.Errorf("after action = %+v", a)
	}
	if _, ok := waiting.After["1m0s"]; ok {
		t.Error("a delay with nothing located should be left out")
	}

	inv := waiting.Invoke
	if len(inv) != 2 || inv[0].ID != "plain" || inv[0].Input != nil || inv[0].OnDone != nil {
		t.Fatalf("invoke = %+v", inv)
	}
	if inv[1].ID != "mapped" || inv[1].Input == nil || inv[1].Input.Func != "engine_test.srcInput" ||
		inv[1].OnDone == nil || inv[1].OnDone.Func != "engine_test.srcDone" || inv[1].OnError != nil {
		t.Errorf("mapped invocation = %+v", inv[1])
	}

	if out := src.States["end"].Output; out == nil || out.Func != "engine_test.srcOutput" || out.File != srcFile {
		t.Errorf("output = %+v", out)
	}
}

func TestSources_LocatesWrappedMethods(t *testing.T) {
	k := srcChecker{min: 1}
	m, err := engine.CreateMachine(engine.MachineConfig[srcCtx, srcEvt]{
		ID:      "wrapped",
		Initial: "a",
		States: map[string]engine.StateNodeConfig[srcCtx, srcEvt]{
			"a": {
				Entry: []action.Action[srcCtx, srcEvt]{&srcCustom{}},
				On: map[string][]engine.TransitionConfig[srcCtx, srcEvt]{
					"GO": {{Target: "b", Guard: k.Check}},
				},
			},
			"b": {},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	a := m.Sources().States["a"]

	if e := a.Entry; len(e) != 1 || e[0] == nil || e[0].Func != "engine_test.srcCustom.Apply" || e[0].File != srcFile || e[0].Line == 0 {
		t.Errorf("value-receiver action behind a pointer = %+v", e)
	}
	g := a.On["GO"][0].Guard
	if g == nil || g.Func != "engine_test.srcChecker.Check" {
		t.Fatalf("method value guard = %+v", g)
	}
	if g.File != "" || g.Line != 0 || g.Path != "" {
		t.Errorf("a method value is located by name only, got %+v", g)
	}
}

func TestSources_ReturnsACopy(t *testing.T) {
	m := srcMachine(t)
	want := m.Sources().Machine.Line
	m.Sources().Machine.Line = -1
	if got := m.Sources().Machine.Line; got != want {
		t.Errorf("machine line = %d after a caller changed its copy, want %d", got, want)
	}
}

func ExampleMachine_Sources() {
	m, _ := engine.CreateMachine(engine.MachineConfig[srcCtx, srcEvt]{
		ID:      "door",
		Initial: "closed",
		States: map[string]engine.StateNodeConfig[srcCtx, srcEvt]{
			"closed": {On: map[string][]engine.TransitionConfig[srcCtx, srcEvt]{
				"OPEN": {{Target: "open", Guard: srcIsPositive, GuardName: "isPositive"}},
			}},
			"open": {},
		},
	})
	src := m.Sources()
	fmt.Println(src.Machine.File)
	fmt.Println(src.States["closed"].On["OPEN"][0].Guard.Func)
	// Output:
	// github.com/arisros/fate/engine_test/source_test.go
	// engine_test.srcIsPositive
}
