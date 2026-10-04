package engine_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/arisros/fate/describe"
	"github.com/arisros/fate/engine"
	"github.com/arisros/fate/render"
)

type (
	metaState = engine.StateNodeConfig[struct{}, string]
	metaTrans = engine.TransitionConfig[struct{}, string]
)

func metaConfig(stateMeta, transMeta map[string]any) engine.MachineConfig[struct{}, string] {
	return engine.MachineConfig[struct{}, string]{
		ID: "task", Initial: "signup",
		States: map[string]metaState{
			"signup": {
				Meta: stateMeta,
				On:   map[string][]metaTrans{"SUBMIT": {{Target: "review", Meta: transMeta}}},
			},
			"review": {Initial: "open",
				OnDone: []metaTrans{{Target: "signup", Meta: map[string]any{"auto": true}}},
				States: map[string]metaState{"open": {}},
			},
		},
	}
}

func TestMeta_PublishedByDescribeAndGraph(t *testing.T) {
	stateMeta := map[string]any{"form": "signup_form", "order": 2}
	m, err := engine.CreateMachine(metaConfig(stateMeta, map[string]any{"title": "Submit", "hidden": false}))
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	stateMeta["form"] = "changed after CreateMachine"

	d := m.Describe()
	if got, want := string(d.States["signup"].Meta), `{"form":"signup_form","order":2}`; got != want {
		t.Errorf("state meta %s, want %s", got, want)
	}
	if got, want := string(d.States["signup"].On["SUBMIT"][0].Meta), `{"hidden":false,"title":"Submit"}`; got != want {
		t.Errorf("transition meta %s, want %s", got, want)
	}
	if got, want := string(d.States["review"].OnDone[0].Meta), `{"auto":true}`; got != want {
		t.Errorf("onDone meta %s, want %s", got, want)
	}
	if d.States["review"].Meta != nil {
		t.Errorf("state without Meta published %s", d.States["review"].Meta)
	}

	blob, _ := json.Marshal(d)
	loaded, err := describe.LoadDescriptor(blob)
	if err != nil {
		t.Fatalf("LoadDescriptor: %v", err)
	}
	g := render.GraphJSON(loaded)
	for _, n := range g.Nodes {
		if n.Path == "signup" && string(n.Meta) != `{"form":"signup_form","order":2}` {
			t.Errorf("graph node meta %s", n.Meta)
		}
	}
	for _, e := range g.Edges {
		if e.Event == "SUBMIT" && string(e.Meta) != `{"hidden":false,"title":"Submit"}` {
			t.Errorf("graph edge meta %s", e.Meta)
		}
	}
}

func TestMeta_RejectsWhatCannotBePublished(t *testing.T) {
	bad := map[string]any{"fn": func() {}}
	if _, err := engine.CreateMachine(metaConfig(bad, nil)); !errors.Is(err, engine.ErrInvalidConfig) {
		t.Errorf("state meta: err %v, want ErrInvalidConfig", err)
	}
	if _, err := engine.CreateMachine(metaConfig(nil, bad)); !errors.Is(err, engine.ErrInvalidConfig) {
		t.Errorf("transition meta: err %v, want ErrInvalidConfig", err)
	}
	cfg := metaConfig(nil, nil)
	cfg.States["signup"] = metaState{After: map[time.Duration][]metaTrans{
		time.Hour: {{Target: "review", Meta: map[string]any{"title": "Expire"}}},
	}}
	if _, err := engine.CreateMachine(cfg); !errors.Is(err, engine.ErrInvalidConfig) {
		t.Errorf("after meta: err %v, want ErrInvalidConfig", err)
	}
}
