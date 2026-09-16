package fate_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	sc "github.com/arisros/fate"
)

type gateCtx struct {
	Score  int    `json:"score"`
	Status string `json:"status"`
}

func gateMachine(meta *sc.CondMeta) (*sc.Machine[gateCtx, string], error) {
	return sc.CreateMachine(sc.MachineConfig[gateCtx, string]{
		ID:      "gate",
		Initial: "review",
		States: map[string]sc.StateNodeConfig[gateCtx, string]{
			"review": {On: map[string][]sc.TransitionConfig[gateCtx, string]{
				"DECIDE": {{
					Target:   "approved",
					Guard:    sc.Guard[gateCtx, string](func(c gateCtx, _ string) bool { return c.Score >= 60 }),
					CondMeta: meta,
				}},
			}},
			"approved": {Type: sc.NodeFinal},
		},
	})
}

func TestCondMeta_BuildersProduceFields(t *testing.T) {
	got := sc.Gates(
		sc.Field("$.score").WithLabel("score passes").Gte(60),
		sc.Field("$.status").Neq("blocked"),
		sc.Field("$.score").Gt(1),
		sc.Field("$.score").Lt(101),
		sc.Field("$.score").Lte(100),
		sc.Field("$.status").Eq("open"),
		sc.Field("$.status").In("open", "held"),
		sc.Field("$.status").Truthy(),
		sc.Field("$.missing").Falsy(),
	).Build()

	want := []sc.CondOp{sc.CondGte, sc.CondNeq, sc.CondGt, sc.CondLt, sc.CondLte, sc.CondEq, sc.CondIn, sc.CondTruthy, sc.CondFalsy}
	if len(got.Fields) != len(want) {
		t.Fatalf("fields: got %d want %d", len(got.Fields), len(want))
	}
	for i, op := range want {
		if got.Fields[i].Op != op {
			t.Errorf("field %d op: got %q want %q", i, got.Fields[i].Op, op)
		}
	}
	if got.Fields[0].Label != "score passes" || got.Fields[1].Label != "" {
		t.Errorf("labels: got %q, %q", got.Fields[0].Label, got.Fields[1].Label)
	}
	if got.Sample != nil {
		t.Errorf("Build must not set a sample, got %s", got.Sample)
	}
	if _, err := gateMachine(got); err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
}

func TestCondMeta_Describe(t *testing.T) {
	meta := sc.Gates(sc.Field("$.score").Gte(60)).Sample(`{"score": 65}`)
	m, err := gateMachine(meta)
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	b, err := json.Marshal(m.Describe().States["review"].On["DECIDE"][0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `"condMeta":{"fields":[{"path":"$.score","op":"gte","value":60}],"sample":{"score":65}}`
	if !strings.Contains(string(b), want) {
		t.Fatalf("descriptor: got %s, want it to contain %s", b, want)
	}
}

func TestCondMeta_Validation(t *testing.T) {
	cases := []struct {
		name string
		meta *sc.CondMeta
		msg  string
	}{
		{"path without root", sc.Gates(sc.Field("score").Eq(1)).Build(), `path "score"`},
		{"empty segment", sc.Gates(sc.Field("$..score").Eq(1)).Build(), `path "$..score"`},
		{"bracket path", sc.Gates(sc.Field("$.items[0]").Eq(1)).Build(), `path "$.items[0]"`},
		{"unknown op", &sc.CondMeta{Fields: []sc.CondField{{Path: "$.score", Op: "like"}}}, `unknown op "like"`},
		{"in without values", sc.Gates(sc.Field("$.status").In()).Build(), "with no values"},
		{"in with scalar", &sc.CondMeta{Fields: []sc.CondField{{Path: "$.status", Op: sc.CondIn, Value: "open"}}}, "with no values"},
		{"unmarshalable value", sc.Gates(sc.Field("$.score").Eq(func() {})).Build(), "value:"},
		{"sample not json", sc.Gates().Sample(`{score`), "not a JSON object"},
		{"sample not object", sc.Gates().Sample(`[1]`), "not a JSON object"},
		{"sample null", sc.Gates().Sample(`null`), "not a JSON object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := gateMachine(tc.meta)
			if !errors.Is(err, sc.ErrInvalidConfig) {
				t.Fatalf("got %v, want ErrInvalidConfig", err)
			}
			if !strings.Contains(err.Error(), tc.msg) || !strings.Contains(err.Error(), `event "DECIDE"`) {
				t.Fatalf("error %q should mention %q and the event", err, tc.msg)
			}
		})
	}
}

func TestCondMeta_ValidatesOnDoneAndAfter(t *testing.T) {
	bad := sc.Gates(sc.Field("nope").Eq(1)).Build()
	_, err := sc.CreateMachine(sc.MachineConfig[gateCtx, string]{
		ID:      "ondone",
		Initial: "flow",
		States: map[string]sc.StateNodeConfig[gateCtx, string]{
			"flow": {
				Initial: "done",
				States:  map[string]sc.StateNodeConfig[gateCtx, string]{"done": {Type: sc.NodeFinal}},
				OnDone:  []sc.TransitionConfig[gateCtx, string]{{Target: "end", CondMeta: bad}},
			},
			"end": {Type: sc.NodeFinal},
		},
	})
	if !errors.Is(err, sc.ErrInvalidConfig) || !strings.Contains(err.Error(), "onDone candidate 0") {
		t.Fatalf("onDone: got %v", err)
	}

	_, err = sc.CreateMachine(sc.MachineConfig[gateCtx, string]{
		ID:      "after",
		Initial: "wait",
		States: map[string]sc.StateNodeConfig[gateCtx, string]{
			"wait": {After: map[time.Duration][]sc.TransitionConfig[gateCtx, string]{
				time.Second: {{Target: "end", CondMeta: bad}},
			}},
			"end": {Type: sc.NodeFinal},
		},
	})
	if !errors.Is(err, sc.ErrInvalidConfig) || !strings.Contains(err.Error(), "after 1s candidate 0") {
		t.Fatalf("after: got %v", err)
	}
}

func TestCondMeta_DoesNotAffectGuard(t *testing.T) {
	m, err := gateMachine(sc.Gates(sc.Field("$.score").Lt(0)).Build())
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	a := sc.NewActor(m)
	if err := a.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := a.Send(t.Context(), "DECIDE"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !a.Snapshot().Matches("review") {
		t.Fatalf("guard (score >= 60) must still block with score 0, got %s", a.Snapshot().Value.Path())
	}
}
