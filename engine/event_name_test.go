package engine_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/arisros/fate/action"
	"github.com/arisros/fate/effect"
	"github.com/arisros/fate/engine"
)

type kindEvt string

type enumEvt int64

const (
	enumSubmit enumEvt = iota + 1
	enumBack
)

type methodKindEvt string

func (methodKindEvt) EventName() string { return "FROM_METHOD" }

type ptrEvt struct{}

func (*ptrEvt) EventName() string { return "PTR" }

func twoStep[Evt any](t *testing.T, on string, namer func(Evt) string) *engine.Actor[struct{}, Evt] {
	t.Helper()
	m, err := engine.CreateMachine(engine.MachineConfig[struct{}, Evt]{
		ID:        "two_step",
		Initial:   "form",
		EventName: namer,
		States: map[string]engine.StateNodeConfig[struct{}, Evt]{
			"form": {On: map[string][]engine.TransitionConfig[struct{}, Evt]{on: {{Target: "done"}}}},
			"done": {},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	a := engine.NewActor(m)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return a
}

func sendTo[Evt any](t *testing.T, a *engine.Actor[struct{}, Evt], evt Evt, want string) {
	t.Helper()
	if err := a.Send(context.Background(), evt); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := a.Snapshot().Value.Path(); got != want {
		t.Fatalf("value = %q, want %q", got, want)
	}
}

func TestNamedStringEventDispatchesOnItsValue(t *testing.T) {
	a := twoStep[kindEvt](t, "SUBMIT", nil)
	if a.Can("BACK") {
		t.Fatal(`Can("BACK") = true, want false`)
	}
	if !a.Can("SUBMIT") {
		t.Fatal(`Can("SUBMIT") = false, want true`)
	}
	sendTo(t, a, "SUBMIT", "done")
}

func TestPointerToNamedStringEventDispatchesOnItsValue(t *testing.T) {
	evt := kindEvt("SUBMIT")
	sendTo(t, twoStep[*kindEvt](t, "SUBMIT", nil), &evt, "done")
}

func TestEventNameMethodWinsOverStringValue(t *testing.T) {
	sendTo(t, twoStep[methodKindEvt](t, "FROM_METHOD", nil), "ignored", "done")
}

func TestIntEnumWithoutNamerIsUnnamed(t *testing.T) {
	a := twoStep[enumEvt](t, "enumEvt", nil)
	var steps []engine.Step
	a.SubscribeSteps(func(s engine.Step) { steps = append(steps, s) })
	before := a.Snapshot()

	if a.Can(enumSubmit) {
		t.Fatal("Can = true for an unnamed event, want false")
	}
	err := a.Send(context.Background(), enumSubmit)
	if !errors.Is(err, engine.ErrUnnamedEvent) {
		t.Fatalf("Send err = %v, want ErrUnnamedEvent", err)
	}
	if !strings.Contains(err.Error(), "enumEvt") {
		t.Errorf("error %q does not name the event type", err)
	}
	if _, err := a.Preview(enumSubmit); !errors.Is(err, engine.ErrUnnamedEvent) {
		t.Fatalf("Preview err = %v, want ErrUnnamedEvent", err)
	}
	if got := a.Snapshot().Value.Path(); got != before.Value.Path() {
		t.Fatalf("value = %q, want %q (unchanged)", got, before.Value.Path())
	}
	if len(steps) != 0 {
		t.Fatalf("steps = %+v, want none for a rejected event", steps)
	}
}

func TestUnnamedEventDoesNotReachTheWildcard(t *testing.T) {
	a := twoStep[enumEvt](t, "*", nil)
	if err := a.Send(context.Background(), enumSubmit); !errors.Is(err, engine.ErrUnnamedEvent) {
		t.Fatalf("Send err = %v, want ErrUnnamedEvent", err)
	}
	if got := a.Snapshot().Value.Path(); got != "form" {
		t.Fatalf("value = %q, want form", got)
	}
}

func TestNilAndEmptyEventsAreUnnamed(t *testing.T) {
	if err := twoStep[any](t, "SUBMIT", nil).Send(context.Background(), nil); !errors.Is(err, engine.ErrUnnamedEvent) {
		t.Errorf("nil event: err = %v, want ErrUnnamedEvent", err)
	}
	if err := twoStep[*ptrEvt](t, "PTR", nil).Send(context.Background(), nil); !errors.Is(err, engine.ErrUnnamedEvent) {
		t.Errorf("nil pointer event: err = %v, want ErrUnnamedEvent", err)
	}
	if err := twoStep[string](t, "SUBMIT", nil).Send(context.Background(), ""); !errors.Is(err, engine.ErrUnnamedEvent) {
		t.Errorf("empty string event: err = %v, want ErrUnnamedEvent", err)
	}
	if err := twoStep[struct{}](t, "SUBMIT", nil).Send(context.Background(), struct{}{}); !errors.Is(err, engine.ErrUnnamedEvent) {
		t.Errorf("anonymous struct event: err = %v, want ErrUnnamedEvent", err)
	}
}

func TestPointerReceiverEventNameMethod(t *testing.T) {
	sendTo(t, twoStep[*ptrEvt](t, "PTR", nil), &ptrEvt{}, "done")
}

func TestMachineEventNameNamesIntEnum(t *testing.T) {
	names := map[enumEvt]string{enumSubmit: "SUBMIT", enumBack: "BACK"}
	a := twoStep(t, "SUBMIT", func(e enumEvt) string { return names[e] })
	if a.Can(enumBack) {
		t.Fatal("Can(BACK) = true, want false")
	}
	if got := a.Enabled(func(name string) (enumEvt, bool) { return enumSubmit, name == "SUBMIT" }); len(got) != 1 || got[0] != "SUBMIT" {
		t.Fatalf("Enabled = %v, want [SUBMIT]", got)
	}
	var steps []engine.Step
	a.SubscribeSteps(func(s engine.Step) { steps = append(steps, s) })
	sendTo(t, a, enumSubmit, "done")
	if len(steps) == 0 || steps[0].Event != "SUBMIT" {
		t.Fatalf("steps = %+v, want the first to carry event SUBMIT", steps)
	}
}

func TestMachineEventNameReplacesDefaultRules(t *testing.T) {
	a := twoStep(t, "FROM_NAMER", func(methodKindEvt) string { return "FROM_NAMER" })
	sendTo(t, a, "anything", "done")
}

func TestMachineEventNameEmptyIsUnnamed(t *testing.T) {
	a := twoStep(t, "SUBMIT", func(enumEvt) string { return "" })
	if err := a.Send(context.Background(), enumSubmit); !errors.Is(err, engine.ErrUnnamedEvent) {
		t.Fatalf("Send err = %v, want ErrUnnamedEvent", err)
	}
}

func TestRaisedUnnamedEventIsDroppedAndLogged(t *testing.T) {
	m, err := engine.CreateMachine(engine.MachineConfig[struct{}, enumEvt]{
		ID:      "raise_unnamed",
		Initial: "idle",
		States: map[string]engine.StateNodeConfig[struct{}, enumEvt]{
			"idle": {Entry: []action.Action[struct{}, enumEvt]{action.Raise[struct{}](enumBack)}},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	var logs []string
	a := engine.NewActor(m, engine.WithLogger(func(s string) { logs = append(logs, s) }))
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], engine.ErrUnnamedEvent.Error()) {
		t.Fatalf("logs = %q, want one ErrUnnamedEvent drop", logs)
	}
}

func TestInvocationUnnamedEventIsDroppedAndLogged(t *testing.T) {
	m, err := engine.CreateMachine(engine.MachineConfig[struct{}, enumEvt]{
		ID:      "invoke_unnamed",
		Initial: "work",
		States: map[string]engine.StateNodeConfig[struct{}, enumEvt]{
			"work": {Invoke: []effect.Invocation[struct{}, enumEvt]{{
				ID:     "job",
				Src:    "job",
				OnDone: func(any) enumEvt { return enumSubmit },
			}}},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	var logs []string
	a := engine.NewActor(m, engine.WithLogger(func(s string) { logs = append(logs, s) }))
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	pending := a.PendingInvocations()
	if len(pending) != 1 {
		t.Fatalf("pending = %+v, want one invocation", pending)
	}
	if !a.ResolveInvocation(pending[0].ID, nil) {
		t.Fatal("ResolveInvocation = false, want true: the outcome was accepted")
	}
	if len(logs) != 1 || !strings.Contains(logs[0], engine.ErrUnnamedEvent.Error()) {
		t.Fatalf("logs = %q, want one ErrUnnamedEvent drop", logs)
	}
}

func TestDescribeLabelsRaiseWithTheMachineEventName(t *testing.T) {
	names := map[enumEvt]string{enumSubmit: "SUBMIT", enumBack: "BACK"}
	describeEntry := func(namer func(enumEvt) string) string {
		m, err := engine.CreateMachine(engine.MachineConfig[struct{}, enumEvt]{
			ID:        "raise_label",
			Initial:   "idle",
			EventName: namer,
			States: map[string]engine.StateNodeConfig[struct{}, enumEvt]{
				"idle": {Entry: []action.Action[struct{}, enumEvt]{action.Raise[struct{}](enumBack)}},
			},
		})
		if err != nil {
			t.Fatalf("CreateMachine: %v", err)
		}
		return m.Describe().States["idle"].Entry[0]
	}
	if got := describeEntry(func(e enumEvt) string { return names[e] }); got != "raise:BACK" {
		t.Errorf("with a namer: label = %q, want raise:BACK", got)
	}
	if got := describeEntry(nil); got != "raise" {
		t.Errorf("without a namer: label = %q, want raise", got)
	}
}
