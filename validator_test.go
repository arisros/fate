package fate_test

// Stateless validator tests — mirror the LPW Application Status (P12) use
// case where the FSM is consulted as a pure validator (no Actor instance).

import (
	"testing"

	sc "github.com/arisros/fate"
)

// Minimal flat machine modeling LPW application-status — `new` → ... → `live`
// with `rejected`/`cancelled` as terminals. Mirrors the topology described
// in docs/architecture/statechart/fsm-lpw-application-status.md.
type valCtx struct{}
type valEvt interface{ isValEvt() }

type valEvtPreQualify struct{}
type valEvtPreApprove struct{}
type valEvtApprove struct{}
type valEvtGoLive struct{}
type valEvtReject struct{}
type valEvtCancel struct{}

func (valEvtPreQualify) isValEvt()         {}
func (valEvtPreApprove) isValEvt()         {}
func (valEvtApprove) isValEvt()            {}
func (valEvtGoLive) isValEvt()             {}
func (valEvtReject) isValEvt()             {}
func (valEvtCancel) isValEvt()             {}
func (valEvtPreQualify) EventName() string { return "PRE_QUALIFY" }
func (valEvtPreApprove) EventName() string { return "PRE_APPROVE" }
func (valEvtApprove) EventName() string    { return "APPROVE" }
func (valEvtGoLive) EventName() string     { return "GO_LIVE" }
func (valEvtReject) EventName() string     { return "REJECT" }
func (valEvtCancel) EventName() string     { return "CANCEL" }

func newAppStatusMachine(t *testing.T) *sc.Machine[valCtx, valEvt] {
	t.Helper()
	m, err := sc.CreateMachine(sc.MachineConfig[valCtx, valEvt]{
		ID:      "app-status",
		Initial: "new",
		States: map[string]sc.StateNodeConfig[valCtx, valEvt]{
			"new": {
				On: map[string][]sc.TransitionConfig[valCtx, valEvt]{
					"PRE_QUALIFY": {{Target: "pre_qualified"}},
					"CANCEL":      {{Target: "cancelled"}},
				},
			},
			"pre_qualified": {
				On: map[string][]sc.TransitionConfig[valCtx, valEvt]{
					"PRE_APPROVE": {{Target: "pre_approved"}},
					"REJECT":      {{Target: "rejected"}},
				},
			},
			"pre_approved": {
				On: map[string][]sc.TransitionConfig[valCtx, valEvt]{
					"APPROVE": {{Target: "approved"}},
					"REJECT":  {{Target: "rejected"}},
				},
			},
			"approved": {
				On: map[string][]sc.TransitionConfig[valCtx, valEvt]{
					"GO_LIVE": {{Target: "live"}},
				},
			},
			"live":      {Type: sc.NodeFinal},
			"rejected":  {Type: sc.NodeFinal},
			"cancelled": {Type: sc.NodeFinal},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	return m
}

func TestValidator_IsKnownState(t *testing.T) {
	m := newAppStatusMachine(t)

	for _, name := range []string{"new", "pre_qualified", "approved", "live", "rejected", "cancelled"} {
		if !m.IsKnownState(name) {
			t.Errorf("IsKnownState(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"", "unknown", "Live"} { // case-sensitive, no empty
		if m.IsKnownState(name) {
			t.Errorf("IsKnownState(%q) = true, want false", name)
		}
	}
}

func TestValidator_IsTerminal(t *testing.T) {
	m := newAppStatusMachine(t)

	for _, name := range []string{"live", "rejected", "cancelled"} {
		if !m.IsTerminal(name) {
			t.Errorf("IsTerminal(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"new", "approved", "unknown"} {
		if m.IsTerminal(name) {
			t.Errorf("IsTerminal(%q) = true, want false", name)
		}
	}
}

func TestValidator_IsLegalTransition(t *testing.T) {
	m := newAppStatusMachine(t)

	legal := []struct {
		from, evt string
	}{
		{"new", "PRE_QUALIFY"},
		{"new", "CANCEL"},
		{"pre_qualified", "PRE_APPROVE"},
		{"pre_qualified", "REJECT"},
		{"approved", "GO_LIVE"},
	}
	for _, c := range legal {
		if !m.IsLegalTransition(c.from, c.evt) {
			t.Errorf("IsLegalTransition(%q, %q) = false, want true", c.from, c.evt)
		}
	}

	illegal := []struct {
		from, evt string
	}{
		{"new", "APPROVE"},
		{"new", "GO_LIVE"},
		{"live", "PRE_QUALIFY"}, // terminal — no outgoing transitions
		{"unknown", "PRE_QUALIFY"},
		{"pre_qualified", "UNKNOWN_EVENT"},
	}
	for _, c := range illegal {
		if m.IsLegalTransition(c.from, c.evt) {
			t.Errorf("IsLegalTransition(%q, %q) = true, want false", c.from, c.evt)
		}
	}
}

func TestValidator_StatesReturnsAllNames(t *testing.T) {
	m := newAppStatusMachine(t)

	got := m.States()
	want := []string{"approved", "cancelled", "live", "new", "pre_approved", "pre_qualified", "rejected"}
	if len(got) != len(want) {
		t.Fatalf("States() count: got %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("States()[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestValidator_StatesIncludesNestedChildren(t *testing.T) {
	// Compound machine — States() must include the child names too.
	m, err := sc.CreateMachine(sc.MachineConfig[valCtx, valEvt]{
		ID:      "nested",
		Initial: "outer",
		States: map[string]sc.StateNodeConfig[valCtx, valEvt]{
			"outer": {
				Initial: "inner_a",
				States: map[string]sc.StateNodeConfig[valCtx, valEvt]{
					"inner_a": {},
					"inner_b": {},
				},
			},
			"done": {Type: sc.NodeFinal},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}

	got := m.States()
	contains := func(s []string, target string) bool {
		for _, x := range s {
			if x == target {
				return true
			}
		}
		return false
	}
	for _, name := range []string{"outer", "inner_a", "inner_b", "done"} {
		if !contains(got, name) {
			t.Errorf("States() must contain %q; got %v", name, got)
		}
	}
}

// Two states at the same depth may share a local name. The lookup has to pick
// one, and it must pick the same one every run: a search that iterated the
// children map directly would let IsKnownState, IsTerminal and
// IsLegalTransition disagree with themselves between runs of one program.
func TestFindStateIsDeterministicWhenNamesCollide(t *testing.T) {
	m, err := sc.CreateMachine(sc.MachineConfig[struct{}, string]{
		ID:      "collide",
		Initial: "alpha",
		States: map[string]sc.StateNodeConfig[struct{}, string]{
			"alpha": {
				Initial: "shared",
				States: map[string]sc.StateNodeConfig[struct{}, string]{
					// Reached first, because "alpha" sorts before "beta".
					"shared": {},
				},
			},
			"beta": {
				Initial: "shared",
				States: map[string]sc.StateNodeConfig[struct{}, string]{
					"shared": {Type: sc.NodeFinal},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	for i := range 50 {
		if !m.IsKnownState("shared") {
			t.Fatalf("run %d: IsKnownState(shared) = false, want true", i)
		}
		// alpha.shared is atomic, beta.shared is final. A stable search always
		// finds the one under "alpha".
		if m.IsTerminal("shared") {
			t.Fatalf("run %d: IsTerminal(shared) = true, want false: the search reached beta.shared", i)
		}
	}
}
