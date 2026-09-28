package fate_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	fate "github.com/arisros/fate"
)

type kindEvt string

type enumEvt int64

const (
	enumSubmit enumEvt = iota + 1
	enumBack
)

type namedKindEvt string

func (namedKindEvt) EventName() string { return "FROM_METHOD" }

func twoStep[Evt any](t *testing.T, on string, namer func(Evt) string) *fate.Machine[struct{}, Evt] {
	t.Helper()
	m, err := fate.CreateMachine(fate.MachineConfig[struct{}, Evt]{
		ID:        "two_step",
		Initial:   "form",
		EventName: namer,
		States: map[string]fate.StateNodeConfig[struct{}, Evt]{
			"form": {On: map[string][]fate.TransitionConfig[struct{}, Evt]{on: {{Target: "done"}}}},
			"done": {},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	return m
}

func started[Evt any](t *testing.T, m *fate.Machine[struct{}, Evt]) *fate.Actor[struct{}, Evt] {
	t.Helper()
	a := fate.NewActor(m)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return a
}

func TestNamedStringEventDispatchesOnItsValue(t *testing.T) {
	a := started(t, twoStep[kindEvt](t, "SUBMIT", nil))
	if !a.Can("SUBMIT") {
		t.Fatal(`Can("SUBMIT") = false, want true`)
	}
	if err := a.Send(context.Background(), "SUBMIT"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := a.Snapshot().Value.Path(); got != "done" {
		t.Fatalf("value = %q, want done", got)
	}
}

func TestPointerToNamedStringEventDispatchesOnItsValue(t *testing.T) {
	a := started(t, twoStep[*kindEvt](t, "SUBMIT", nil))
	evt := kindEvt("SUBMIT")
	if err := a.Send(context.Background(), &evt); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := a.Snapshot().Value.Path(); got != "done" {
		t.Fatalf("value = %q, want done", got)
	}
}

func TestEventNameMethodWinsOverStringValue(t *testing.T) {
	a := started(t, twoStep[namedKindEvt](t, "FROM_METHOD", nil))
	if err := a.Send(context.Background(), "ignored"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := a.Snapshot().Value.Path(); got != "done" {
		t.Fatalf("value = %q, want done", got)
	}
}

// An int enum used to collapse every value to its type name, so no transition
// keyed by value could ever fire. It must now fail loudly instead.
func TestIntEnumWithoutNamerIsUnnamed(t *testing.T) {
	a := started(t, twoStep[enumEvt](t, "enumEvt", nil))
	if a.Can(enumSubmit) {
		t.Fatal("Can = true for an unnamed event, want false")
	}
	err := a.Send(context.Background(), enumSubmit)
	if !errors.Is(err, fate.ErrUnnamedEvent) {
		t.Fatalf("Send err = %v, want ErrUnnamedEvent", err)
	}
	if !strings.Contains(err.Error(), "enumEvt") {
		t.Errorf("error %q does not name the event type", err)
	}
	if got := a.Snapshot().Value.Path(); got != "form" {
		t.Fatalf("value = %q, want form (unchanged)", got)
	}
}

func TestMachineEventNameNamesIntEnum(t *testing.T) {
	names := map[enumEvt]string{enumSubmit: "SUBMIT", enumBack: "BACK"}
	namer := func(e enumEvt) string { return names[e] }
	a := started(t, twoStep(t, "SUBMIT", namer))
	if a.Can(enumBack) {
		t.Fatal("Can(BACK) = true, want false")
	}
	if err := a.Send(context.Background(), enumSubmit); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := a.Snapshot().Value.Path(); got != "done" {
		t.Fatalf("value = %q, want done", got)
	}
}

func TestMachineEventNameOverridesDefaultRules(t *testing.T) {
	a := started(t, twoStep(t, "UPPER", func(e kindEvt) string { return strings.ToUpper(string(e)) }))
	if err := a.Send(context.Background(), "upper"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := a.Snapshot().Value.Path(); got != "done" {
		t.Fatalf("value = %q, want done", got)
	}
}

func TestMachineEventNameEmptyIsUnnamed(t *testing.T) {
	a := started(t, twoStep(t, "SUBMIT", func(enumEvt) string { return "" }))
	if err := a.Send(context.Background(), enumSubmit); !errors.Is(err, fate.ErrUnnamedEvent) {
		t.Fatalf("Send err = %v, want ErrUnnamedEvent", err)
	}
}

func TestRaisedUnnamedEventIsDroppedAndLogged(t *testing.T) {
	m, err := fate.CreateMachine(fate.MachineConfig[struct{}, enumEvt]{
		ID:      "raise_unnamed",
		Initial: "idle",
		States: map[string]fate.StateNodeConfig[struct{}, enumEvt]{
			"idle": {Entry: []fate.Action[struct{}, enumEvt]{fate.Raise[struct{}](enumBack)}},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	var logs []string
	a := fate.NewActor(m, fate.WithLogger(func(s string) { logs = append(logs, s) }))
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], fate.ErrUnnamedEvent.Error()) {
		t.Fatalf("logs = %q, want one ErrUnnamedEvent drop", logs)
	}
}
