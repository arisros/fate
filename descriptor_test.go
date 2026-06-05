package fate_test

// Descriptor tests: round-trip a representative compound machine through
// Describe() and assert the JSON-shaped output matches what loaders will
// consume.

import (
	"encoding/json"
	"strings"
	"testing"

	sc "github.com/arisros/fate"
)

type descCtx struct {
	N int `json:"n"`
}
type descEvt interface{ isDescEvt() }
type descEvtTick struct{}

func (descEvtTick) isDescEvt()        {}
func (descEvtTick) EventName() string { return "TICK" }

func buildDescriptorFixture(t *testing.T) sc.MachineDescriptor {
	t.Helper()
	m, err := sc.CreateMachine(sc.MachineConfig[descCtx, descEvt]{
		ID:      "descriptor-fixture",
		Initial: "active",
		Context: descCtx{N: 7},
		States: map[string]sc.StateNodeConfig[descCtx, descEvt]{
			"active": {
				Initial: "running",
				States: map[string]sc.StateNodeConfig[descCtx, descEvt]{
					"running": {
						On: map[string][]sc.TransitionConfig[descCtx, descEvt]{
							"TICK": {{Target: "running", Internal: true}},
						},
					},
					"finished": {Type: sc.NodeFinal},
					"hist":     {Type: sc.NodeHistory, History: sc.HistoryDeep, Default: "running"},
				},
				On: map[string][]sc.TransitionConfig[descCtx, descEvt]{
					"TICK": {{Target: "stopped"}},
				},
			},
			"stopped": {},
			"regions": {
				Type: sc.NodeParallel,
				States: map[string]sc.StateNodeConfig[descCtx, descEvt]{
					"a": {Initial: "a1", States: map[string]sc.StateNodeConfig[descCtx, descEvt]{"a1": {}}},
					"b": {Initial: "b1", States: map[string]sc.StateNodeConfig[descCtx, descEvt]{"b1": {}}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	return m.Describe()
}

func TestDescriptor_TopLevelFields(t *testing.T) {
	d := buildDescriptorFixture(t)
	if d.ID != "descriptor-fixture" {
		t.Errorf("ID: got %q", d.ID)
	}
	if d.Initial != "active" {
		t.Errorf("Initial: got %q", d.Initial)
	}
	if string(d.Context) == "" {
		t.Errorf("Context: empty, expected JSON-marshaled {\"n\":7}")
	}
	if _, ok := d.States["active"]; !ok {
		t.Errorf("missing top-level state 'active'")
	}
}

func TestDescriptor_CompoundChildrenAndHistory(t *testing.T) {
	d := buildDescriptorFixture(t)
	active := d.States["active"]
	if active.Type != "compound" {
		t.Errorf("active.Type: got %q want compound", active.Type)
	}
	if active.Initial != "running" {
		t.Errorf("active.Initial: got %q", active.Initial)
	}
	if _, ok := active.States["running"]; !ok {
		t.Errorf("missing active.running")
	}
	hist := active.States["hist"]
	if hist.Type != "history" {
		t.Errorf("hist.Type: got %q", hist.Type)
	}
	if hist.History != "deep" {
		t.Errorf("hist.History: got %q want deep", hist.History)
	}
	if hist.Default != "running" {
		t.Errorf("hist.Default: got %q", hist.Default)
	}
	finished := active.States["finished"]
	if finished.Type != "final" {
		t.Errorf("finished.Type: got %q", finished.Type)
	}
}

func TestDescriptor_ParallelRegions(t *testing.T) {
	d := buildDescriptorFixture(t)
	regions := d.States["regions"]
	if regions.Type != "parallel" {
		t.Errorf("regions.Type: got %q", regions.Type)
	}
	if _, ok := regions.States["a"]; !ok {
		t.Errorf("missing region 'a'")
	}
	if _, ok := regions.States["b"]; !ok {
		t.Errorf("missing region 'b'")
	}
}

func TestDescriptor_OnTransitionsRecorded(t *testing.T) {
	d := buildDescriptorFixture(t)
	active := d.States["active"]
	tickAtActive := active.On["TICK"]
	if len(tickAtActive) != 1 {
		t.Fatalf("active.On[TICK] length: got %d", len(tickAtActive))
	}
	if tickAtActive[0].Target != "stopped" {
		t.Errorf("active.On[TICK].Target: got %q", tickAtActive[0].Target)
	}
	running := active.States["running"]
	tickAtRunning := running.On["TICK"]
	if len(tickAtRunning) != 1 || tickAtRunning[0].Target != "running" || !tickAtRunning[0].Internal {
		t.Errorf("running.On[TICK]: got %+v want internal self-loop on running", tickAtRunning)
	}
}

func TestLoadDescriptor_Roundtrip(t *testing.T) {
	original := buildDescriptorFixture(t)
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	loaded, err := sc.LoadDescriptor(b)
	if err != nil {
		t.Fatalf("LoadDescriptor: %v", err)
	}
	if loaded.ID != original.ID {
		t.Errorf("ID: got %q want %q", loaded.ID, original.ID)
	}
	if loaded.Initial != original.Initial {
		t.Errorf("Initial: got %q want %q", loaded.Initial, original.Initial)
	}
	if len(loaded.States) != len(original.States) {
		t.Errorf("states count: got %d want %d", len(loaded.States), len(original.States))
	}
	for k, v := range original.States {
		lv, ok := loaded.States[k]
		if !ok {
			t.Errorf("missing state %q in loaded descriptor", k)
			continue
		}
		if lv.Type != v.Type {
			t.Errorf("state %q type: got %q want %q", k, lv.Type, v.Type)
		}
	}
}

func TestLoadDescriptor_RejectsMissingID(t *testing.T) {
	_, err := sc.LoadDescriptor([]byte(`{"states":{"a":{"type":"atomic"}}}`))
	if err == nil {
		t.Error("expected error for missing id")
	}
}

func TestLoadDescriptor_RejectsEmptyStates(t *testing.T) {
	_, err := sc.LoadDescriptor([]byte(`{"id":"x","states":{}}`))
	if err == nil {
		t.Error("expected error for empty states map")
	}
}

func TestLoadDescriptor_RejectsUnknownStateType(t *testing.T) {
	_, err := sc.LoadDescriptor([]byte(`{"id":"x","states":{"a":{"type":"mystery"}}}`))
	if err == nil {
		t.Error("expected error for unknown state type")
	}
}

func TestLoadDescriptor_RejectsUnknownNestedStateType(t *testing.T) {
	_, err := sc.LoadDescriptor([]byte(`{"id":"x","states":{"p":{"type":"compound","states":{"c":{"type":"mystery"}}}}}`))
	if err == nil {
		t.Error("expected error for unknown nested state type")
	}
}

func TestLoadDescriptor_RejectsMalformedJSON(t *testing.T) {
	_, err := sc.LoadDescriptor([]byte(`not json`))
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestDescriptor_JSONShapeIsStable(t *testing.T) {
	d := buildDescriptorFixture(t)
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	got := string(b)
	// Loose shape assertions — full golden-file matching is in P7 follow-ups.
	for _, needle := range []string{
		`"id":"descriptor-fixture"`,
		`"initial":"active"`,
		`"context":{"n":7}`,
		`"type":"parallel"`,
		`"type":"history"`,
		`"history":"deep"`,
		`"default":"running"`,
		`"type":"final"`,
		`"target":"stopped"`,
		`"internal":true`,
	} {
		if !strings.Contains(got, needle) {
			t.Errorf("descriptor JSON missing %q\nfull JSON: %s", needle, got)
		}
	}
}
