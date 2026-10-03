package action_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/arisros/fate/action"
	"github.com/arisros/fate/describe"
	"github.com/arisros/fate/engine"
)

type gateCtx struct {
	Score  int    `json:"score"`
	Status string `json:"status"`
}

func gateMachine(meta *action.CondMeta) (*engine.Machine[gateCtx, string], error) {
	return engine.CreateMachine(engine.MachineConfig[gateCtx, string]{
		ID:      "gate",
		Initial: "review",
		States: map[string]engine.StateNodeConfig[gateCtx, string]{
			"review": {On: map[string][]engine.TransitionConfig[gateCtx, string]{
				"DECIDE": {{
					Target:   "approved",
					Guard:    action.Guard[gateCtx, string](func(c gateCtx, _ string) bool { return c.Score >= 60 }),
					CondMeta: meta,
				}},
			}},
			"approved": {Type: engine.NodeFinal},
		},
	})
}

func TestCondMeta_BuildersProduceFields(t *testing.T) {
	got := action.Gates(
		action.Field("$.score").WithLabel("score passes").Gte(60),
		action.Field("$.status").Neq("blocked"),
		action.Field("$.score").Gt(1),
		action.Field("$.score").Lt(101),
		action.Field("$.score").Lte(100),
		action.Field("$.status").Eq("open"),
		action.Field("$.status").In("open", "held"),
		action.Field("$.status").Truthy(),
		action.Field("$.missing").Falsy(),
	).Build()

	want := []action.CondOp{action.CondGte, action.CondNeq, action.CondGt, action.CondLt, action.CondLte, action.CondEq, action.CondIn, action.CondTruthy, action.CondFalsy}
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
	meta := action.Gates(action.Field("$.score").Gte(60)).Sample(`{"score": 65}`)
	m, err := gateMachine(meta)
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	b, err := json.Marshal(m.Describe().States["review"].On["DECIDE"][0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `"cond_meta":{"fields":[{"path":"$.score","op":"gte","value":60}],"sample":{"score":65}}`
	if !strings.Contains(string(b), want) {
		t.Fatalf("descriptor: got %s, want it to contain %s", b, want)
	}
}

func TestCondMeta_Validation(t *testing.T) {
	cases := []struct {
		name string
		meta *action.CondMeta
		msg  string
	}{
		{"path without root", action.Gates(action.Field("score").Eq(1)).Build(), `path "score"`},
		{"bare root", action.Gates(action.Field("$").Eq(1)).Build(), `path "$"`},
		{"empty segment", action.Gates(action.Field("$..score").Eq(1)).Build(), `path "$..score"`},
		{"bracket path", action.Gates(action.Field("$.items[0]").Eq(1)).Build(), `path "$.items[0]"`},
		{"wildcard", action.Gates(action.Field("$.items.*").Eq(1)).Build(), `path "$.items.*"`},
		{"space", action.Gates(action.Field("$.a b").Eq(1)).Build(), `path "$.a b"`},
		{"newline", action.Gates(action.Field("$.a\nb").Eq(1)).Build(), `path "$.a\nb"`},
		{"unknown op", &action.CondMeta{Fields: []action.CondField{{Path: "$.score", Op: "like"}}}, `unknown op "like"`},
		{"in without values", action.Gates(action.Field("$.status").In()).Build(), "needs a non-empty list"},
		{"in with scalar", &action.CondMeta{Fields: []action.CondField{{Path: "$.status", Op: action.CondIn, Value: "open"}}}, "needs a non-empty list"},
		{"in with bytes", action.Gates(action.Field("$.status").In([]byte{1})).Build(), "marshals as a string"},
		{"gt with string", action.Gates(action.Field("$.score").Gt("abc")).Build(), `needs a number, got string`},
		{"lte with nil", action.Gates(action.Field("$.score").Lte(nil)).Build(), `needs a number, got <nil>`},
		{"truthy with value", &action.CondMeta{Fields: []action.CondField{{Path: "$.ok", Op: action.CondTruthy, Value: true}}}, "takes no value"},
		{"unmarshalable value", action.Gates(action.Field("$.score").Eq(func() {})).Build(), "value:"},
		{"sample not json", action.Gates().Sample(`{score`), "not a JSON object"},
		{"sample not object", action.Gates().Sample(`[1]`), "not a JSON object"},
		{"sample null", action.Gates().Sample(`null`), "not a JSON object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := gateMachine(tc.meta)
			if !errors.Is(err, engine.ErrInvalidConfig) {
				t.Fatalf("got %v, want ErrInvalidConfig", err)
			}
			if !strings.Contains(err.Error(), tc.msg) || !strings.Contains(err.Error(), `event "DECIDE"`) {
				t.Fatalf("error %q should mention %q and the event", err, tc.msg)
			}
		})
	}
}

func TestCondMeta_OnDoneValidatedAfterRejected(t *testing.T) {
	bad := action.Gates(action.Field("nope").Eq(1)).Build()
	_, err := engine.CreateMachine(engine.MachineConfig[gateCtx, string]{
		ID:      "ondone",
		Initial: "flow",
		States: map[string]engine.StateNodeConfig[gateCtx, string]{
			"flow": {
				Initial: "done",
				States:  map[string]engine.StateNodeConfig[gateCtx, string]{"done": {Type: engine.NodeFinal}},
				OnDone:  []engine.TransitionConfig[gateCtx, string]{{Target: "end", CondMeta: bad}},
			},
			"end": {Type: engine.NodeFinal},
		},
	})
	if !errors.Is(err, engine.ErrInvalidConfig) || !strings.Contains(err.Error(), "onDone candidate 0") {
		t.Fatalf("onDone: got %v", err)
	}

	_, err = engine.CreateMachine(engine.MachineConfig[gateCtx, string]{
		ID:      "after",
		Initial: "wait",
		States: map[string]engine.StateNodeConfig[gateCtx, string]{
			"wait": {After: map[time.Duration][]engine.TransitionConfig[gateCtx, string]{
				time.Second: {{Target: "end", CondMeta: bad}},
			}},
			"end": {Type: engine.NodeFinal},
		},
	})
	if !errors.Is(err, engine.ErrInvalidConfig) || !strings.Contains(err.Error(), "after 1s candidate 0 has CondMeta") {
		t.Fatalf("after: got %v", err)
	}
}

func TestCondMeta_DoesNotAffectGuard(t *testing.T) {
	m, err := gateMachine(action.Gates(action.Field("$.score").Lt(0)).Build())
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	a := engine.NewActor(m)
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

func TestCondMeta_AcceptedOperands(t *testing.T) {
	meta := action.Gates(
		action.Field("$.items.0").Eq(nil),
		action.Field("$.score").Gte(json.Number("60")),
		action.Field("$.score").Lt(uint8(100)),
		action.Field("$.tags").In([]string{"a", "b"}),
		action.Field("$.codes").In([2]byte{1, 2}),
	).Build()
	m, err := gateMachine(meta)
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	b, _ := json.Marshal(m.Describe().States["review"].On["DECIDE"][0].CondMeta)
	want := `{"fields":[{"path":"$.items.0","op":"eq","value":null},{"path":"$.score","op":"gte","value":60},{"path":"$.score","op":"lt","value":100},{"path":"$.tags","op":"in","value":["a","b"]},{"path":"$.codes","op":"in","value":[1,2]}]}`
	if string(b) != want {
		t.Fatalf("got  %s\nwant %s", b, want)
	}
}

func TestCondMeta_MachineOwnsItsCopy(t *testing.T) {
	tags := []string{"a"}
	meta := action.Gates(action.Field("$.tags").In(tags)).Sample(`{ "tags" : ["a"] }`)
	m, err := gateMachine(meta)
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	descriptorJSON := func() string {
		b, err := json.Marshal(m.Describe().States["review"].On["DECIDE"][0].CondMeta)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return string(b)
	}
	want := `{"fields":[{"path":"$.tags","op":"in","value":["a"]}],"sample":{"tags":["a"]}}`
	if got := descriptorJSON(); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}

	meta.Fields[0].Path = "NOT A PATH"
	meta.Sample = []byte("not json")
	tags[0] = "changed"
	out := m.Describe().States["review"].On["DECIDE"][0].CondMeta
	out.Fields[0].Path = "ALSO NOT"
	out.Fields[0].Value.(json.RawMessage)[2] = 'X'
	out.Sample[2] = 'X'

	if got := descriptorJSON(); got != want {
		t.Fatalf("machine changed after edits: %s", got)
	}
}

func TestCondMeta_LoadDescriptorRoundTrip(t *testing.T) {
	m, err := gateMachine(action.Gates(action.Field("$.score").Gte(60), action.Field("$.ok").Truthy()).Sample(`{"score":60,"ok":true}`))
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	first, err := json.Marshal(m.Describe())
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := describe.LoadDescriptor(first)
	if err != nil {
		t.Fatalf("LoadDescriptor: %v", err)
	}
	second, err := json.Marshal(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("round trip changed the descriptor:\n%s\n%s", first, second)
	}
	if v := loaded.States["review"].On["DECIDE"][0].CondMeta.Fields[0].Value; v != float64(60) {
		t.Fatalf("loaded operand = %#v, want float64(60)", v)
	}
}
