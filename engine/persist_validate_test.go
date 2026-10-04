package engine_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/arisros/fate/engine"
)

func validateMachine(t *testing.T) *engine.Machine[struct{}, string] {
	t.Helper()
	type state = engine.StateNodeConfig[struct{}, string]
	m, err := engine.CreateMachine(engine.MachineConfig[struct{}, string]{
		ID: "task", Initial: "draft",
		States: map[string]state{
			"draft": {},
			"review": {Type: engine.NodeParallel, States: map[string]state{
				"bpkb": {Initial: "open", States: map[string]state{"open": {}, "closed": {}}},
				"uw":   {Initial: "open", States: map[string]state{"open": {}, "closed": {}}},
			}},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	return m
}

func TestNewActorFromSnapshot_RejectsAValueTheMachineDoesNotHave(t *testing.T) {
	for name, tc := range map[string]struct{ value, want string }{
		"empty value":         {`""`, `no child ""`},
		"unknown top state":   {`"archived"`, `no child "archived"`},
		"unknown region":      {`{"review":{"bpkb":"open","uw":"open","stnk":"open"}}`, `no child "stnk"`},
		"unknown nested leaf": {`{"review":{"bpkb":"waived","uw":"open"}}`, `"review.bpkb" has no child "waived"`},
		"region left out":     {`{"review":{"bpkb":"open"}}`, `missing region "uw"`},
		"parallel as a leaf":  {`{"review":"bpkb"}`, `no regions`},
		"two active children": {`{"draft":"draft","review":{"bpkb":"open","uw":"open"}}`, `2 active children`},
	} {
		blob := []byte(`{"version":1,"status":"running","value":` + tc.value + `,"context":{}}`)
		_, err := engine.NewActorFromSnapshot[struct{}, string](validateMachine(t), blob)
		if !errors.Is(err, engine.ErrSnapshotMismatch) {
			t.Errorf("%s: err %v, want ErrSnapshotMismatch", name, err)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err %q, want it to mention %q", name, err, tc.want)
		}
	}
}

func TestNewActorFromSnapshot_AcceptsEveryShapeTheEngineWrites(t *testing.T) {
	for _, value := range []string{
		`"draft"`,
		`"review"`,
		`{"review":{"bpkb":"closed","uw":"open"}}`,
	} {
		blob := []byte(`{"version":1,"status":"running","value":` + value + `,"context":{}}`)
		if _, err := engine.NewActorFromSnapshot[struct{}, string](validateMachine(t), blob); err != nil {
			t.Errorf("%s: %v", value, err)
		}
	}
}
