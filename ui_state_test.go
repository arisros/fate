package fate_test

import (
	"encoding/json"
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
	return string(got)
}

func TestUIState_NoContributor(t *testing.T) {
	if got := uiStateAfter(t, uiMachine(t)); got != "" {
		t.Fatalf("idle: got %s, want nil", got)
	}
}

func TestUIState_AncestorContributes(t *testing.T) {
	if got := uiStateAfter(t, uiMachine(t), "SETUP"); got != `{"title":"intro","level":5}` {
		t.Fatalf("settings.general: got %s", got)
	}
}

func TestUIState_NearestStateWins(t *testing.T) {
	if got := uiStateAfter(t, uiMachine(t), "SETUP", "AUDIO"); got != `{"volume":5,"muted":false}` {
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
	if _, err := m.UIState(s.Value, s.Context); err == nil || !strings.Contains(err.Error(), `ui state of "a"`) {
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
		"tags":     `{"items":{"type":"string"},"type":"array"}`,
		"blob":     `{"type":"string"}`,
		"counts":   `{"additionalProperties":{"type":"integer"},"type":"object"}`,
		"pointer":  `{"type":"boolean"}`,
		"anything": `{}`,
		"when":     `{}`,
		"raw":      `{}`,
		"Untagged": `{"type":"integer"}`,
		"tree":     `{"properties":{"children":{"items":{},"type":"array"},"name":{"type":"string"}},"required":["name"],"type":"object"}`,
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
	if got := string(sc.UIStateOf(func(uiCtx) *int { return nil }).Schema()); got != `{"type":"integer"}` {
		t.Errorf("pointer to int: got %s", got)
	}
}
