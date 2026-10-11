package engine_test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arisros/fate/action"
	"github.com/arisros/fate/describe"
	"github.com/arisros/fate/engine"
	"github.com/arisros/fate/httphandler"
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

func TestHandler_ServesSources(t *testing.T) {
	h := httphandler.New(srcMachine(t), func(name string) (srcEvt, error) { return srcEvt{Name: name}, nil })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/sources", nil))
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var got describe.MachineSources
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Machine == nil || got.Machine.File != srcFile {
		t.Errorf("served machine location = %+v", got.Machine)
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
