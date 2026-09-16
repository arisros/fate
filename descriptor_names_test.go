package fate_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	fate "github.com/arisros/fate"
	"github.com/arisros/fate/render"
)

// Tests for the names guards and actions carry into a MachineDescriptor, and
// through it into every rendered diagram. Before these existed the descriptor
// reported "" for every action and every guard, so a generated diagram showed
// topology and nothing else.

type dnCtx struct{ Hits int }

// dnChecker exists to produce two guards that differ in behaviour but share a
// code pointer, which is the shape that defeats pointer-based name inference.
type dnChecker struct{ min int }

func (c dnChecker) Check(dnCtx, string) bool { return c.min < 100 }

func dnAssign() fate.Action[dnCtx, string] {
	return fate.Assign(func(c dnCtx, _ string) dnCtx { c.Hits++; return c })
}

// transitionDescriptor builds the descriptor for the machine and returns its
// single GO transition, which is where these tests read names from.
func transitionDescriptor(t *testing.T, m *fate.Machine[dnCtx, string]) fate.TransitionDescriptor {
	t.Helper()
	d := m.Describe()
	ts, ok := d.States["idle"].On["GO"]
	if !ok || len(ts) != 1 {
		t.Fatalf("expected exactly one GO transition on idle, got %#v", d.States["idle"].On)
	}
	return ts[0]
}

func machineWithActions(t *testing.T, actions ...fate.Action[dnCtx, string]) *fate.Machine[dnCtx, string] {
	t.Helper()
	m, err := fate.CreateMachine(fate.MachineConfig[dnCtx, string]{
		ID:      "names",
		Initial: "idle",
		States: map[string]fate.StateNodeConfig[dnCtx, string]{
			"idle": {On: map[string][]fate.TransitionConfig[dnCtx, string]{
				"GO": {{Target: "open", Actions: actions}},
			}},
			"open": {},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return m
}

func TestBuiltinActionsReportTheirKind(t *testing.T) {
	tests := []struct {
		name   string
		action fate.Action[dnCtx, string]
		want   string
	}{
		{"assign", dnAssign(), "assign"},
		{"raise names its event", fate.Raise[dnCtx, string]("CANCEL"), "raise:CANCEL"},
		{"raise with an unnameable event", fate.Raise[dnCtx, string](""), "raise"},
		{"log omits the message", fate.Log[dnCtx, string]("something happened"), "log"},
		{"named wrapper", fate.Named("lockApplication", dnAssign()), "lockApplication"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := transitionDescriptor(t, machineWithActions(t, tc.action)).Actions
			if len(got) != 1 || got[0] != tc.want {
				t.Errorf("action names = %#v, want [%q]", got, tc.want)
			}
		})
	}
}

func TestEntryAndExitActionsAreNamed(t *testing.T) {
	m, err := fate.CreateMachine(fate.MachineConfig[dnCtx, string]{
		ID:      "names",
		Initial: "idle",
		States: map[string]fate.StateNodeConfig[dnCtx, string]{
			"idle": {
				Entry: []fate.Action[dnCtx, string]{fate.Named("arm", dnAssign())},
				Exit:  []fate.Action[dnCtx, string]{fate.Named("disarm", dnAssign())},
			},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	sd := m.Describe().States["idle"]
	if len(sd.Entry) != 1 || sd.Entry[0] != "arm" {
		t.Errorf("entry = %#v, want [\"arm\"]", sd.Entry)
	}
	if len(sd.Exit) != 1 || sd.Exit[0] != "disarm" {
		t.Errorf("exit = %#v, want [\"disarm\"]", sd.Exit)
	}
}

// Naming must be a label, never a behaviour change. This is the guarantee that
// lets Setup.Action wrap every action it hands out.
func TestNamedDoesNotChangeBehaviour(t *testing.T) {
	run := func(t *testing.T, action fate.Action[dnCtx, string]) dnCtx {
		t.Helper()
		a := fate.NewActor(machineWithActions(t, action))
		if err := a.Start(context.Background()); err != nil {
			t.Fatalf("start: %v", err)
		}
		if err := a.Send(context.Background(), "GO"); err != nil {
			t.Fatalf("send: %v", err)
		}
		return a.Snapshot().Context
	}

	bare := run(t, dnAssign())
	wrapped := run(t, fate.Named("bump", dnAssign()))
	if bare != wrapped {
		t.Errorf("wrapped action produced %+v, bare produced %+v", wrapped, bare)
	}
	if wrapped.Hits != 1 {
		t.Errorf("Hits = %d, want 1: the wrapped action did not run", wrapped.Hits)
	}
}

func TestNamedToleratesANilAction(t *testing.T) {
	a := fate.NewActor(machineWithActions(t, fate.Named[dnCtx, string]("placeholder", nil)))
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := a.Send(context.Background(), "GO"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got := a.Snapshot().Value.Path(); got != "open" {
		t.Fatalf("state = %q, want %q: a named no-op must still transition", got, "open")
	}
}

// setupMachine wires one guard and one action by name, which is the path a
// large machine actually uses.
func setupMachine(t *testing.T, s *fate.Setup[dnCtx, string]) *fate.Machine[dnCtx, string] {
	t.Helper()
	m, err := s.CreateMachine(fate.MachineConfig[dnCtx, string]{
		ID:      "names",
		Initial: "idle",
		States: map[string]fate.StateNodeConfig[dnCtx, string]{
			"idle": {On: map[string][]fate.TransitionConfig[dnCtx, string]{
				"GO": {{
					Target:    "open",
					Guard:     s.Guard("isReady"),
					GuardName: "isReady",
					Actions:   []fate.Action[dnCtx, string]{s.Action("clearForm")},
				}},
			}},
			"open": {},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return m
}

func TestSetupNamesReachTheDescriptor(t *testing.T) {
	s := fate.NewSetup[dnCtx, string]().
		WithGuard("isReady", func(dnCtx, string) bool { return true }).
		WithAction("clearForm", dnAssign())

	td := transitionDescriptor(t, setupMachine(t, s))
	if td.Guard != "isReady" {
		t.Errorf("guard = %q, want %q", td.Guard, "isReady")
	}
	if len(td.Actions) != 1 || td.Actions[0] != "clearForm" {
		t.Errorf("actions = %#v, want [%q]", td.Actions, "clearForm")
	}
}

func TestSetupActionStillRuns(t *testing.T) {
	s := fate.NewSetup[dnCtx, string]().
		WithGuard("isReady", func(dnCtx, string) bool { return true }).
		WithAction("clearForm", dnAssign())

	a := fate.NewActor(setupMachine(t, s))
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := a.Send(context.Background(), "GO"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got := a.Snapshot().Context.Hits; got != 1 {
		t.Errorf("Hits = %d, want 1: the named action from Setup did not run", got)
	}
	if got := a.Snapshot().Value.Path(); got != "open" {
		t.Errorf("state = %q, want %q: the named guard from Setup did not pass", got, "open")
	}
}

// Guard names are declared, not inferred. An earlier version matched guards to
// registered names by implementation pointer, which is not a per-closure
// identity: a method value's pointer is per-method, not per-receiver, so an
// unregistered guard sharing a code pointer with a registered one inherited its
// name. A diagram then asserted a condition that was not on that edge, which is
// worse than the blank it replaced, because a blank is visibly absent.
func TestGuardNamesAreNotInferredFromImplementation(t *testing.T) {
	a, b := dnChecker{min: 18}, dnChecker{min: 65}
	if reflect.ValueOf(fate.Guard[dnCtx, string](a.Check)).Pointer() !=
		reflect.ValueOf(fate.Guard[dnCtx, string](b.Check)).Pointer() {
		t.Skip("method values no longer share a code pointer; the mislabelling shape is gone")
	}

	m, err := fate.CreateMachine(fate.MachineConfig[dnCtx, string]{
		ID:      "names",
		Initial: "idle",
		States: map[string]fate.StateNodeConfig[dnCtx, string]{
			"idle": {On: map[string][]fate.TransitionConfig[dnCtx, string]{
				"GO":    {{Target: "open", Guard: a.Check, GuardName: "isAdult"}},
				"OTHER": {{Target: "open", Guard: b.Check}},
			}},
			"open": {},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	on := m.Describe().States["idle"].On
	if got := on["GO"][0].Guard; got != "isAdult" {
		t.Errorf("declared guard = %q, want %q", got, "isAdult")
	}
	if got := on["OTHER"][0].Guard; got != "" {
		t.Errorf("undeclared guard = %q, want \"\": it shares a code pointer with isAdult but is a different guard", got)
	}
}

func TestUnnamedGuardsFallBackToEmpty(t *testing.T) {
	t.Run("guard written inline, no Setup", func(t *testing.T) {
		m, err := fate.CreateMachine(fate.MachineConfig[dnCtx, string]{
			ID:      "names",
			Initial: "idle",
			States: map[string]fate.StateNodeConfig[dnCtx, string]{
				"idle": {On: map[string][]fate.TransitionConfig[dnCtx, string]{
					"GO": {{Target: "open", Guard: func(dnCtx, string) bool { return true }}},
				}},
				"open": {},
			},
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if got := transitionDescriptor(t, m).Guard; got != "" {
			t.Errorf("guard = %q, want \"\"", got)
		}
	})

	t.Run("no guard at all", func(t *testing.T) {
		if got := transitionDescriptor(t, machineWithActions(t)).Guard; got != "" {
			t.Errorf("guard = %q, want \"\"", got)
		}
	})

}

// A GuardName on a transition with no guard is meaningless but must not crash
// or invent an edge condition.
func TestGuardNameWithoutAGuardIsIgnored(t *testing.T) {
	m, err := fate.CreateMachine(fate.MachineConfig[dnCtx, string]{
		ID:      "names",
		Initial: "idle",
		States: map[string]fate.StateNodeConfig[dnCtx, string]{
			"idle": {On: map[string][]fate.TransitionConfig[dnCtx, string]{
				"GO": {{Target: "open", GuardName: "isReady"}},
			}},
			"open": {},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	a := fate.NewActor(m)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := a.Send(context.Background(), "GO"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got := a.Snapshot().Value.Path(); got != "open" {
		t.Errorf("state = %q, want %q: a name must not act as a guard", got, "open")
	}
}

// The point of all of the above: a rendered diagram now says what the machine
// does, not merely where it can go.
func TestRenderedDiagramsCarryTheNames(t *testing.T) {
	s := fate.NewSetup[dnCtx, string]().
		WithGuard("isReady", func(dnCtx, string) bool { return true }).
		WithAction("clearForm", dnAssign())
	d := setupMachine(t, s).Describe()

	t.Run("mermaid", func(t *testing.T) {
		out := render.Mermaid(d, render.MermaidOptions{})
		for _, want := range []string{"isReady", "clearForm"} {
			if !strings.Contains(out, want) {
				t.Errorf("mermaid output missing %q:\n%s", want, out)
			}
		}
	})

	// ASCII draws the state tree, which carries no guards or actions; the
	// per-state transition listing is where they surface.
	t.Run("ascii transitions", func(t *testing.T) {
		out := render.Transitions(d, "idle")
		for _, want := range []string{"isReady", "clearForm"} {
			if !strings.Contains(out, want) {
				t.Errorf("transition listing missing %q:\n%s", want, out)
			}
		}
	})
}

func TestEnqueueActionIsNamed(t *testing.T) {
	action := fate.EnqueueActions(func(enq *fate.Enqueuer[dnCtx, string]) {
		enq.Assign(func(c dnCtx, _ string) dnCtx { c.Hits++; return c })
	})
	got := transitionDescriptor(t, machineWithActions(t, action)).Actions
	if len(got) != 1 || got[0] != "enqueue" {
		t.Errorf("action names = %#v, want [\"enqueue\"]", got)
	}
}

// A nil entry in an Actions slice is tolerated at describe time; it renders as
// an empty name rather than panicking.
func TestNilActionDescribesAsEmpty(t *testing.T) {
	got := transitionDescriptor(t, machineWithActions(t, nil)).Actions
	if len(got) != 1 || got[0] != "" {
		t.Errorf("action names = %#v, want [\"\"]", got)
	}
}

// An unregistered name still yields a usable, labelled no-op, so config
// construction can continue far enough for CreateMachine to report every
// missing reference at once rather than failing at the first.
func TestSetupActionForUnregisteredNameIsANamedNoop(t *testing.T) {
	s := fate.NewSetup[dnCtx, string]().WithAction("clearForm", dnAssign())
	action := s.Action("neverRegistered")

	m, err := fate.CreateMachine(fate.MachineConfig[dnCtx, string]{
		ID:      "names",
		Initial: "idle",
		States: map[string]fate.StateNodeConfig[dnCtx, string]{
			"idle": {On: map[string][]fate.TransitionConfig[dnCtx, string]{
				"GO": {{Target: "open", Actions: []fate.Action[dnCtx, string]{action}}},
			}},
			"open": {},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := transitionDescriptor(t, m).Actions; len(got) != 1 || got[0] != "neverRegistered" {
		t.Errorf("action names = %#v, want [\"neverRegistered\"]", got)
	}

	a := fate.NewActor(m)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := a.Send(context.Background(), "GO"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got := a.Snapshot().Context.Hits; got != 0 {
		t.Errorf("Hits = %d, want 0: an unregistered action must do nothing", got)
	}

	// The reference is still recorded, so building through the Setup fails.
	if _, err := s.CreateMachine(fate.MachineConfig[dnCtx, string]{
		ID: "names", Initial: "idle",
		States: map[string]fate.StateNodeConfig[dnCtx, string]{"idle": {}},
	}); err == nil {
		t.Error("Setup.CreateMachine returned nil error, want a missing-reference error")
	}
}

// A Setup must surface an invalid config the same way CreateMachine does,
// rather than swallowing the error while attaching its name registry.
func TestSetupCreateMachinePropagatesConfigErrors(t *testing.T) {
	s := fate.NewSetup[dnCtx, string]()
	_, err := s.CreateMachine(fate.MachineConfig[dnCtx, string]{
		ID:      "names",
		Initial: "doesNotExist",
		States: map[string]fate.StateNodeConfig[dnCtx, string]{
			"idle": {},
		},
	})
	if err == nil {
		t.Fatal("CreateMachine returned nil error, want a config error for an unknown initial state")
	}
}
