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
	want := `"cond_meta":{"fields":[{"path":"$.score","op":"gte","value":60}],"sample":{"score":65}}`
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
		{"bare root", sc.Gates(sc.Field("$").Eq(1)).Build(), `path "$"`},
		{"empty segment", sc.Gates(sc.Field("$..score").Eq(1)).Build(), `path "$..score"`},
		{"bracket path", sc.Gates(sc.Field("$.items[0]").Eq(1)).Build(), `path "$.items[0]"`},
		{"wildcard", sc.Gates(sc.Field("$.items.*").Eq(1)).Build(), `path "$.items.*"`},
		{"space", sc.Gates(sc.Field("$.a b").Eq(1)).Build(), `path "$.a b"`},
		{"newline", sc.Gates(sc.Field("$.a\nb").Eq(1)).Build(), `path "$.a\nb"`},
		{"unknown op", &sc.CondMeta{Fields: []sc.CondField{{Path: "$.score", Op: "like"}}}, `unknown op "like"`},
		{"in without values", sc.Gates(sc.Field("$.status").In()).Build(), "needs a non-empty list"},
		{"in with scalar", &sc.CondMeta{Fields: []sc.CondField{{Path: "$.status", Op: sc.CondIn, Value: "open"}}}, "needs a non-empty list"},
		{"in with bytes", sc.Gates(sc.Field("$.status").In([]byte{1})).Build(), "marshals as a string"},
		{"gt with string", sc.Gates(sc.Field("$.score").Gt("abc")).Build(), `needs a number, got string`},
		{"lte with nil", sc.Gates(sc.Field("$.score").Lte(nil)).Build(), `needs a number, got <nil>`},
		{"truthy with value", &sc.CondMeta{Fields: []sc.CondField{{Path: "$.ok", Op: sc.CondTruthy, Value: true}}}, "takes no value"},
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

func TestCondMeta_OnDoneValidatedAfterRejected(t *testing.T) {
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
	if !errors.Is(err, sc.ErrInvalidConfig) || !strings.Contains(err.Error(), "after 1s candidate 0 has CondMeta") {
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

func TestCondMeta_AcceptedOperands(t *testing.T) {
	meta := sc.Gates(
		sc.Field("$.items.0").Eq(nil),
		sc.Field("$.score").Gte(json.Number("60")),
		sc.Field("$.score").Lt(uint8(100)),
		sc.Field("$.tags").In([]string{"a", "b"}),
		sc.Field("$.codes").In([2]byte{1, 2}),
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
	meta := sc.Gates(sc.Field("$.tags").In(tags)).Sample(`{ "tags" : ["a"] }`)
	m, err := gateMachine(meta)
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	describe := func() string {
		b, err := json.Marshal(m.Describe().States["review"].On["DECIDE"][0].CondMeta)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return string(b)
	}
	want := `{"fields":[{"path":"$.tags","op":"in","value":["a"]}],"sample":{"tags":["a"]}}`
	if got := describe(); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}

	meta.Fields[0].Path = "NOT A PATH"
	meta.Sample = []byte("not json")
	tags[0] = "changed"
	out := m.Describe().States["review"].On["DECIDE"][0].CondMeta
	out.Fields[0].Path = "ALSO NOT"
	out.Fields[0].Value.(json.RawMessage)[2] = 'X'
	out.Sample[2] = 'X'

	if got := describe(); got != want {
		t.Fatalf("machine changed after edits: %s", got)
	}
}

func TestCondMeta_LoadDescriptorRoundTrip(t *testing.T) {
	m, err := gateMachine(sc.Gates(sc.Field("$.score").Gte(60), sc.Field("$.ok").Truthy()).Sample(`{"score":60,"ok":true}`))
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	first, err := json.Marshal(m.Describe())
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := sc.LoadDescriptor(first)
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
