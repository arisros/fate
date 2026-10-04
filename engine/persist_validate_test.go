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
				"book": {Initial: "open", States: map[string]state{"open": {}, "closed": {}}},
				"ux":   {Initial: "open", States: map[string]state{"open": {}, "closed": {}}},
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
		"unknown region":      {`{"review":{"book":"open","ux":"open","slip":"open"}}`, `no child "slip"`},
		"unknown nested leaf": {`{"review":{"book":"waived","ux":"open"}}`, `"review.book" has no child "waived"`},
		"region left out":     {`{"review":{"book":"open"}}`, `missing region "ux"`},
		"parallel as a leaf":  {`{"review":"book"}`, `no regions`},
		"two active children": {`{"draft":"draft","review":{"book":"open","ux":"open"}}`, `2 active children`},
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
		`{"review":{"book":"closed","ux":"open"}}`,
	} {
		blob := []byte(`{"version":1,"status":"running","value":` + value + `,"context":{}}`)
		if _, err := engine.NewActorFromSnapshot[struct{}, string](validateMachine(t), blob); err != nil {
			t.Errorf("%s: %v", value, err)
		}
	}
}
