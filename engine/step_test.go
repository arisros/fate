package engine_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/arisros/fate/action"
	"github.com/arisros/fate/effect"
	"github.com/arisros/fate/engine"
)

type (
	stepState = engine.StateNodeConfig[int, string]
	stepTrans = engine.TransitionConfig[int, string]
)

func stepMachine(t *testing.T) *engine.Machine[int, string] {
	t.Helper()
	bump := action.Assign(func(n int, _ string) int { return n + 1 })
	m, err := engine.CreateMachine(engine.MachineConfig[int, string]{
		ID: "task", Initial: "review",
		States: map[string]stepState{
			"review": {
				Type:   engine.NodeParallel,
				OnDone: []stepTrans{{Target: "approval"}},
				States: map[string]stepState{
					"bpkb": {Initial: "open", States: map[string]stepState{
						"open":   {On: map[string][]stepTrans{"OK": {{Target: "closed"}}}},
						"closed": {Type: engine.NodeFinal},
					}},
					"uw": {Initial: "open", States: map[string]stepState{
						"open": {On: map[string][]stepTrans{
							"OK":   {{Target: "closed", Actions: []action.Action[int, string]{action.Raise[int, string]("NOTE")}}},
							"NOTE": {{Actions: []action.Action[int, string]{bump}}},
						}},
						"closed": {Type: engine.NodeFinal, On: map[string][]stepTrans{"NOTE": {{Actions: []action.Action[int, string]{bump}}}}},
					}},
				},
			},
			"approval": {
				After: map[time.Duration][]stepTrans{time.Hour: {{Target: "expired"}}},
				Invoke: []effect.Invocation[int, string]{{
					ID: "score", Src: "score",
					OnDone:  func(any) string { return "SCORED" },
					OnError: func(error) string { return "FAILED" },
				}},
				On: map[string][]stepTrans{
					"RETRY":  {{Target: "approval"}},
					"SCORED": {{Target: "approved"}},
					"FAILED": {{Actions: []action.Action[int, string]{bump}}},
				},
			},
			"approved": {Type: engine.NodeFinal},
			"expired":  {Type: engine.NodeFinal},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	return m
}

func recordSteps(a *engine.Actor[int, string]) *[]engine.Step {
	var steps []engine.Step
	a.SubscribeSteps(func(s engine.Step) { steps = append(steps, s) })
	return &steps
}

func TestSteps_OneSendRecordsEveryStepInOrder(t *testing.T) {
	a := engine.NewActor(stepMachine(t))
	steps := recordSteps(a)
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), "OK")

	type brief struct {
		Seq     uint64
		Cause   engine.StepCause
		Event   string
		Exited  []string
		Entered []string
		Path    string
	}
	var got []brief
	for _, s := range *steps {
		got = append(got, brief{s.Seq, s.Cause, s.Event, s.Exited, s.Entered, s.Value.Path()})
	}
	want := []brief{
		{1, engine.StepStart, "", nil,
			[]string{"review", "review.bpkb", "review.bpkb.open", "review.uw", "review.uw.open"},
			"review.bpkb.open | uw.open"},
		{2, engine.StepEvent, "OK",
			[]string{"review.bpkb.open", "review.uw.open"},
			[]string{"review.bpkb.closed", "review.uw.closed"},
			"review.bpkb.closed | uw.closed"},
		{3, engine.StepRaise, "NOTE", nil, nil, "review.bpkb.closed | uw.closed"},
		{4, engine.StepDone, "",
			[]string{"review.bpkb.closed", "review.uw.closed", "review.bpkb", "review.uw", "review"},
			[]string{"approval"},
			"approval"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("steps:\n got %+v\nwant %+v", got, want)
	}
	raised := (*steps)[2].Transitions
	if len(raised) != 1 || raised[0].Source != "review.uw.closed" || raised[0].Target != "" {
		t.Errorf("targetless transition recorded as %+v", raised)
	}
}

func TestSteps_ReEntryShowsInBothLists(t *testing.T) {
	a := engine.NewActor(stepMachine(t))
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), "OK")
	before := a.PendingInvocations()
	steps := recordSteps(a)

	_ = a.Send(context.Background(), "RETRY")

	if after := a.PendingInvocations(); !reflect.DeepEqual(before, after) {
		t.Fatalf("pending invocations changed, the test no longer shows the hidden re-entry: %v to %v", before, after)
	}
	s := (*steps)[0]
	if !reflect.DeepEqual(s.Exited, []string{"approval"}) || !reflect.DeepEqual(s.Entered, []string{"approval"}) {
		t.Errorf("re-entry: exited %v entered %v, want approval in both", s.Exited, s.Entered)
	}
	if got := s.Transitions[0]; got.Source != "approval" || got.Target != "approval" {
		t.Errorf("transition %+v", got)
	}
}

func TestSteps_TimerAndInvocationCarryTheirEffect(t *testing.T) {
	a := engine.NewActor(stepMachine(t))
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), "OK")
	steps := recordSteps(a)

	inv := a.PendingInvocations()[0].ID
	a.RejectInvocation(inv, errors.New("down"))
	_ = a.Send(context.Background(), "RETRY")
	a.Send(context.Background(), "UNKNOWN") //nolint:errcheck // dropped on purpose
	timer := a.PendingTimers()[0].ID
	a.FireTimer(timer)

	type brief struct {
		Cause  engine.StepCause
		Event  string
		Effect string
	}
	var got []brief
	for _, s := range *steps {
		got = append(got, brief{s.Cause, s.Event, s.Effect})
	}
	want := []brief{
		{engine.StepInvoke, "FAILED", string(inv)},
		{engine.StepEvent, "RETRY", ""},
		{engine.StepTimer, "", string(timer)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("steps %+v, want %+v", got, want)
	}
	if last := (*steps)[len(*steps)-1]; last.Value.Path() != "expired" {
		t.Errorf("timer step value %q", last.Value.Path())
	}
}

func TestSteps_SeqSurvivesRestore(t *testing.T) {
	m := stepMachine(t)
	a := engine.NewActor(m)
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), "OK")
	blob, err := a.Persist()
	if err != nil {
		t.Fatalf("Persist: %v", err)
	}
	var shape struct {
		Seq uint64 `json:"seq"`
	}
	if err := json.Unmarshal(blob, &shape); err != nil || shape.Seq != 4 {
		t.Fatalf("persisted seq %d (err %v), want 4", shape.Seq, err)
	}

	restored, err := engine.NewActorFromSnapshot[int, string](m, blob)
	if err != nil {
		t.Fatalf("NewActorFromSnapshot: %v", err)
	}
	steps := recordSteps(restored)
	_ = restored.Send(context.Background(), "RETRY")
	if got := (*steps)[0].Seq; got != 5 {
		t.Errorf("seq after restore %d, want 5", got)
	}
}

func TestSteps_UnsubscribeStopsDelivery(t *testing.T) {
	a := engine.NewActor(stepMachine(t))
	n := 0
	stop := a.SubscribeSteps(func(engine.Step) { n++ })
	_ = a.Start(context.Background())
	stop()
	_ = a.Send(context.Background(), "OK")
	if n != 1 {
		t.Errorf("observer called %d times, want 1", n)
	}
}
