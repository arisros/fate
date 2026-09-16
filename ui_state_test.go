package fate_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	sc "github.com/arisros/fate"
	"github.com/arisros/fate/render"
)

type uiCtx struct {
	Volume int
	Muted  bool
	Title  string
}

type playerView struct {
	Title string `json:"title"`
	Level int    `json:"level,omitempty"`
}

type audioView struct {
	Volume int  `json:"volume"`
	Muted  bool `json:"muted"`
}

func uiMachine(t *testing.T) *sc.Machine[uiCtx, string] {
	t.Helper()
	m, err := sc.CreateMachine(sc.MachineConfig[uiCtx, string]{
		ID:      "player",
		Initial: "idle",
		Context: uiCtx{Volume: 5, Title: "intro"},
		States: map[string]sc.StateNodeConfig[uiCtx, string]{
			"idle": {On: map[string][]sc.TransitionConfig[uiCtx, string]{
				"PLAY":  {{Target: "playing"}},
				"SETUP": {{Target: "settings"}},
			}},
			"settings": {
				UIState: sc.UIStateOf(func(c uiCtx) playerView { return playerView{Title: c.Title, Level: c.Volume} }),
				Initial: "general",
				States: map[string]sc.StateNodeConfig[uiCtx, string]{
					"general": {},
					"audio": {
						UIState: sc.UIStateOf(func(c uiCtx) audioView { return audioView{Volume: c.Volume} }),
					},
				},
				On: map[string][]sc.TransitionConfig[uiCtx, string]{
					"AUDIO": {{Target: "settings.audio"}},
				},
			},
			"playing": {
				Type: sc.NodeParallel,
				States: map[string]sc.StateNodeConfig[uiCtx, string]{
					"video": {
						UIState: sc.UIStateOf(func(c uiCtx) playerView { return playerView{Title: c.Title} }),
						Initial: "shown",
						States:  map[string]sc.StateNodeConfig[uiCtx, string]{"shown": {}},
					},
					"audio": {
						Initial: "on",
						States: map[string]sc.StateNodeConfig[uiCtx, string]{
							"on": {UIState: sc.UIStateOf(func(c uiCtx) audioView { return audioView{Volume: c.Volume, Muted: c.Muted} })},
						},
					},
					"captions": {Initial: "off", States: map[string]sc.StateNodeConfig[uiCtx, string]{"off": {}}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	return m
}

func uiStateAfter(t *testing.T, m *sc.Machine[uiCtx, string], events ...string) string {
	t.Helper()
	a := sc.NewActor(m)
	if err := a.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	for _, e := range events {
		if err := a.Send(t.Context(), e); err != nil {
			t.Fatalf("Send %s: %v", e, err)
		}
	}
	s := a.Snapshot()
	got, err := m.UIState(s.Value, s.Context)
	if err != nil {
		t.Fatalf("UIState: %v", err)
	}
	if got == nil {
		return ""
	}
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func TestUIState_NoContributor(t *testing.T) {
	if got := uiStateAfter(t, uiMachine(t)); got != "" {
		t.Fatalf("idle: got %s, want nil", got)
	}
}

func TestUIState_AncestorContributes(t *testing.T) {
	if got := uiStateAfter(t, uiMachine(t), "SETUP"); got != `{"settings":{"title":"intro","level":5}}` {
		t.Fatalf("settings.general: got %s", got)
	}
}

func TestUIState_NearestStateWins(t *testing.T) {
	if got := uiStateAfter(t, uiMachine(t), "SETUP", "AUDIO"); got != `{"settings.audio":{"volume":5,"muted":false}}` {
		t.Fatalf("settings.audio: got %s", got)
	}
}

func TestUIState_ParallelMergesByPath(t *testing.T) {
	got := uiStateAfter(t, uiMachine(t), "PLAY")
	want := `{"playing.audio.on":{"volume":5,"muted":false},"playing.video":{"title":"intro"}}`
	if got != want {
		t.Fatalf("playing: got %s want %s", got, want)
	}
}

func TestUIState_MarshalError(t *testing.T) {
	m, err := sc.CreateMachine(sc.MachineConfig[uiCtx, string]{
		ID:      "broken",
		Initial: "a",
		States: map[string]sc.StateNodeConfig[uiCtx, string]{
			"a": {UIState: sc.UIStateOf(func(uiCtx) chan int { return make(chan int) })},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	a := sc.NewActor(m)
	if err := a.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	s := a.Snapshot()
	if _, err := m.UIState(s.Value, s.Context); err == nil || !strings.Contains(err.Error(), `ui state of "a":`) {
		t.Fatalf("got %v, want an error naming state a", err)
	}
}

func TestUIState_SchemaInDescriptorAndGraph(t *testing.T) {
	m := uiMachine(t)
	want := `{"properties":{"muted":{"type":"boolean"},"volume":{"type":"integer"}},"required":["muted","volume"],"type":"object"}`

	d := m.Describe()
	if got := string(d.States["settings"].States["audio"].UIStateSchema); got != want {
		t.Fatalf("descriptor schema: got %s want %s", got, want)
	}
	if d.States["idle"].UIStateSchema != nil {
		t.Fatalf("idle should have no schema")
	}

	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal descriptor: %v", err)
	}
	loaded, err := sc.LoadDescriptor(b)
	if err != nil {
		t.Fatalf("LoadDescriptor: %v", err)
	}
	for _, n := range render.GraphJSON(loaded).Nodes {
		if n.Path == "settings.audio" {
			if string(n.UIStateSchema) != want {
				t.Fatalf("graph schema: got %s want %s", n.UIStateSchema, want)
			}
			return
		}
	}
	t.Fatal("graph has no settings.audio node")
}

type schemaTree struct {
	Name     string        `json:"name"`
	Children []*schemaTree `json:"children,omitempty"`
}

type schemaBase struct {
	ID string `json:"id"`
}

type schemaAll struct {
	schemaBase
	Ratio    float64           `json:"ratio"`
	Tags     []string          `json:"tags"`
	Blob     []byte            `json:"blob"`
	Counts   map[string]uint16 `json:"counts"`
	Pointer  *bool             `json:"pointer"`
	Anything any               `json:"anything"`
	When     time.Time         `json:"when"`
	Raw      json.RawMessage   `json:"raw"`
	Tree     schemaTree        `json:"tree"`
	Optional string            `json:"optional,omitzero"`
	Skipped  string            `json:"-"`
	Untagged int
	hidden   int
}

func TestUIStateOf_Schema(t *testing.T) {
	var s map[string]any
	if err := json.Unmarshal(sc.UIStateOf(func(uiCtx) schemaAll { return schemaAll{hidden: 1} }).Schema(), &s); err != nil {
		t.Fatalf("schema is not JSON: %v", err)
	}
	props := s["properties"].(map[string]any)
	prop := func(name string) string {
		b, _ := json.Marshal(props[name])
		return string(b)
	}
	checks := map[string]string{
		"id":       `{"type":"string"}`,
		"ratio":    `{"type":"number"}`,
		"tags":     `{"items":{"type":"string"},"type":["array","null"]}`,
		"blob":     `{"type":["string","null"]}`,
		"counts":   `{"additionalProperties":{"type":"integer"},"type":["object","null"]}`,
		"pointer":  `{"type":["boolean","null"]}`,
		"anything": `{}`,
		"when":     `{}`,
		"raw":      `{}`,
		"Untagged": `{"type":"integer"}`,
		"tree":     `{"properties":{"children":{"items":{},"type":["array","null"]},"name":{"type":"string"}},"required":["name"],"type":"object"}`,
	}
	for name, want := range checks {
		if got := prop(name); got != want {
			t.Errorf("%s: got %s want %s", name, got, want)
		}
	}
	for _, absent := range []string{"Skipped", "hidden", "schemaBase"} {
		if _, ok := props[absent]; ok {
			t.Errorf("%s should not be in the schema", absent)
		}
	}
	req, _ := json.Marshal(s["required"])
	if want := `["Untagged","anything","blob","counts","id","pointer","ratio","raw","tags","tree","when"]`; string(req) != want {
		t.Errorf("required: got %s want %s", req, want)
	}
}

type textKey struct{}

func (textKey) MarshalText() ([]byte, error) { return []byte("k"), nil }

func TestUIStateOf_SchemaTextMarshalerAndScalar(t *testing.T) {
	if got := string(sc.UIStateOf(func(uiCtx) textKey { return textKey{} }).Schema()); got != `{"type":"string"}` {
		t.Errorf("text marshaler: got %s", got)
	}
	if got := string(sc.UIStateOf(func(uiCtx) *int { return nil }).Schema()); got != `{"type":["integer","null"]}` {
		t.Errorf("pointer to int: got %s", got)
	}
}

func TestUIState_SharedAncestorContributesOnce(t *testing.T) {
	m, err := sc.CreateMachine(sc.MachineConfig[uiCtx, string]{
		ID:      "shared",
		Initial: "p",
		States: map[string]sc.StateNodeConfig[uiCtx, string]{
			"p": {
				Type:    sc.NodeParallel,
				UIState: sc.UIStateOf(func(c uiCtx) int { return c.Volume }),
				States: map[string]sc.StateNodeConfig[uiCtx, string]{
					"a": {Initial: "x", States: map[string]sc.StateNodeConfig[uiCtx, string]{"x": {}}},
					"b": {Initial: "y", States: map[string]sc.StateNodeConfig[uiCtx, string]{"y": {}}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	if got := uiStateAfter(t, m); got != `{"p":0}` {
		t.Fatalf("got %s, want one entry for p", got)
	}
}

func TestUIState_PanicIsReturned(t *testing.T) {
	m, err := sc.CreateMachine(sc.MachineConfig[uiCtx, string]{
		ID:      "panics",
		Initial: "a",
		States: map[string]sc.StateNodeConfig[uiCtx, string]{
			"a": {UIState: sc.UIStateOf(func(uiCtx) int { panic("boom") })},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	a := sc.NewActor(m)
	if err := a.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	s := a.Snapshot()
	if _, err := m.UIState(s.Value, s.Context); err == nil || !strings.Contains(err.Error(), `ui state of "a" panicked: boom`) {
		t.Fatalf("got %v", err)
	}
}

func TestUIState_ZeroValueRejected(t *testing.T) {
	_, err := sc.CreateMachine(sc.MachineConfig[uiCtx, string]{
		ID:      "zero",
		Initial: "a",
		States:  map[string]sc.StateNodeConfig[uiCtx, string]{"a": {UIState: &sc.UIState[uiCtx]{}}},
	})
	if !errors.Is(err, sc.ErrInvalidConfig) || !strings.Contains(err.Error(), "UIStateOf") {
		t.Fatalf("got %v", err)
	}
	var nilState *sc.UIState[uiCtx]
	if nilState.Schema() != nil {
		t.Fatal("nil UIState should have no schema")
	}
}

type (
	selfMap map[string]selfMap
	selfPtr *selfPtr
	selfArr []selfArr
)

func TestUIStateOf_RecursiveNonStructTypes(t *testing.T) {
	cases := map[string]string{
		"map":     string(sc.UIStateOf(func(uiCtx) selfMap { return nil }).Schema()),
		"pointer": string(sc.UIStateOf(func(uiCtx) selfPtr { return nil }).Schema()),
		"slice":   string(sc.UIStateOf(func(uiCtx) selfArr { return nil }).Schema()),
	}
	want := map[string]string{
		"map":     `{"additionalProperties":{},"type":["object","null"]}`,
		"pointer": `{}`,
		"slice":   `{"items":{},"type":["array","null"]}`,
	}
	for k, got := range cases {
		if got != want[k] {
			t.Errorf("%s: got %s want %s", k, got, want[k])
		}
	}
}

type conflictA struct{ X string }
type conflictB struct{ X string }
type conflictC struct {
	X int `json:"X"`
}

type pointerText struct{ N int }

func (*pointerText) MarshalText() ([]byte, error) { return []byte("t"), nil }

type schemaRules struct {
	conflictA
	conflictB
	Y     int
	Outer struct{ conflictC } `json:"outer"`
	Won   struct {
		X string
		conflictC
	} `json:"won"`
	Quoted int         `json:"quoted,string"`
	Num    json.Number `json:"num"`
	Bytes  [2]byte     `json:"bytes"`
	Text   pointerText `json:"text"`
}

func TestUIStateOf_SchemaMatchesEncodingJSON(t *testing.T) {
	var s struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(sc.UIStateOf(func(uiCtx) schemaRules { return schemaRules{} }).Schema(), &s); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(schemaRules{})
	var emitted map[string]json.RawMessage
	_ = json.Unmarshal(out, &emitted)
	for k := range emitted {
		if _, ok := s.Properties[k]; !ok {
			t.Errorf("encoding/json emits %q but the schema lacks it", k)
		}
	}
	for k := range s.Properties {
		if _, ok := emitted[k]; !ok {
			t.Errorf("schema has %q but encoding/json does not emit it (%s)", k, out)
		}
	}
	want := map[string]string{
		"Y":      `{"type":"integer"}`,
		"outer":  `{"properties":{"X":{"type":"integer"}},"required":["X"],"type":"object"}`,
		"won":    `{"properties":{"X":{"type":"string"}},"required":["X"],"type":"object"}`,
		"quoted": `{"type":"string"}`,
		"num":    `{"type":"number"}`,
		"bytes":  `{"items":{"type":"integer"},"type":"array"}`,
		"text":   `{"properties":{"N":{"type":"integer"}},"required":["N"],"type":"object"}`,
	}
	for k, w := range want {
		if got := string(s.Properties[k]); got != w {
			t.Errorf("%s: got %s want %s", k, got, w)
		}
	}
	if got := strings.Join(s.Required, ","); got != "Y,bytes,num,outer,quoted,text,won" {
		t.Errorf("required: got %s", got)
	}

	ptr := sc.UIStateOf(func(uiCtx) *pointerText { return nil }).Schema()
	if string(ptr) != `{"type":["string","null"]}` {
		t.Errorf("pointer root uses MarshalText: got %s", ptr)
	}
}
