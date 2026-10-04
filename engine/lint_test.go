package engine_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/arisros/fate/engine"
)

type (
	lintState = engine.StateNodeConfig[struct{}, string]
	lintTrans = engine.TransitionConfig[struct{}, string]
)

func lintMachine(t *testing.T, states map[string]lintState) *engine.Machine[struct{}, string] {
	t.Helper()
	m, err := engine.CreateMachine(engine.MachineConfig[struct{}, string]{ID: "task", Initial: "draft", States: states})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	return m
}

func to(target string) []lintTrans { return []lintTrans{{Target: target}} }

func TestLint_CleanMachineHasNoFindings(t *testing.T) {
	m := lintMachine(t, map[string]lintState{
		"draft": {On: map[string][]lintTrans{"SUBMIT": to("review")}},
		"review": {
			Type:   engine.NodeParallel,
			OnDone: to("done"),
			On:     map[string][]lintTrans{"CANCEL": to("cancelled")},
			States: map[string]lintState{
				"book": {Initial: "open", States: map[string]lintState{
					"open":   {On: map[string][]lintTrans{"OK": to("closed")}},
					"closed": {Type: engine.NodeFinal},
				}},
				"ux": {Initial: "open", States: map[string]lintState{
					"open":   {After: map[time.Duration][]lintTrans{time.Hour: to("closed")}},
					"hold":   {On: map[string][]lintTrans{"RESUME": to("hist")}},
					"hist":   {Type: engine.NodeHistory, Default: "open"},
					"closed": {Type: engine.NodeFinal},
				}, On: map[string][]lintTrans{"HOLD": to("hold")}},
			},
		},
		"done":      {Type: engine.NodeFinal},
		"cancelled": {Type: engine.NodeFinal},
	})
	if got := m.Lint(); got != nil {
		t.Errorf("findings on a clean machine: %+v", got)
	}
}

func TestLint_ReportsUnreachableDeadEndAndOnDone(t *testing.T) {
	m := lintMachine(t, map[string]lintState{
		"draft": {On: map[string][]lintTrans{"SUBMIT": to("review"), "PARK": to("parked")}},
		"review": {
			Initial: "open",
			OnDone:  to("draft"),
			States: map[string]lintState{
				"open":   {On: map[string][]lintTrans{"NOTE": {{}}}},
				"orphan": {Initial: "inner", States: map[string]lintState{"inner": {}}},
			},
		},
		"parked":  {On: map[string][]lintTrans{"NOTE": {{}}}},
		"legacy":  {On: map[string][]lintTrans{"BACK": to("draft")}},
		"waiting": {Type: engine.NodeParallel, OnDone: to("draft"), States: map[string]lintState{"a": {}, "b": {}}},
	})
	var got []string
	for _, f := range m.Lint() {
		got = append(got, fmt.Sprintf("%s %s", f.State, f.Kind))
	}
	want := []string{
		"legacy unreachable",
		"parked dead_end",
		"review on_done_never_fires",
		"review.orphan unreachable",
		"waiting unreachable",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings:\n got %q\nwant %q", got, want)
	}
}

func TestLint_ParallelOnDoneNeedsEveryRegionToComplete(t *testing.T) {
	m := lintMachine(t, map[string]lintState{
		"draft": {
			Type:   engine.NodeParallel,
			OnDone: to("done"),
			On:     map[string][]lintTrans{"CANCEL": to("done")},
			States: map[string]lintState{
				"a": {Initial: "open", States: map[string]lintState{
					"open":   {On: map[string][]lintTrans{"OK": to("closed")}},
					"closed": {Type: engine.NodeFinal},
				}},
				"b": {},
			},
		},
		"done": {Type: engine.NodeFinal},
	})
	got := m.Lint()
	want := []engine.Finding{{Kind: engine.FindingOnDoneNeverFires, State: "draft", Message: "declares OnDone but can never complete"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings %+v, want %+v", got, want)
	}
}

func TestCreateMachine_RejectsAnUnresolvedOnDoneTarget(t *testing.T) {
	_, err := engine.CreateMachine(engine.MachineConfig[struct{}, string]{
		ID: "task", Initial: "review",
		States: map[string]lintState{
			"review": {Initial: "open", OnDone: to("nowhere"), States: map[string]lintState{
				"open": {Type: engine.NodeFinal},
			}},
		},
	})
	if !errors.Is(err, engine.ErrUnknownTarget) {
		t.Errorf("err %v, want ErrUnknownTarget", err)
	}
}
