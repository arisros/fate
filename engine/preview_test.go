package engine_test

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/arisros/fate/action"
	"github.com/arisros/fate/engine"
)

type previewCtx struct {
	Notes map[string]string
	Seen  int
}

type (
	previewState = engine.StateNodeConfig[previewCtx, string]
	previewTrans = engine.TransitionConfig[previewCtx, string]
)

func previewActor(t *testing.T) *engine.Actor[previewCtx, string] {
	t.Helper()
	note := action.Assign(func(c previewCtx, e string) previewCtx {
		c.Notes[e] = "seen"
		c.Seen++
		return c
	})
	m, err := engine.CreateMachine(engine.MachineConfig[previewCtx, string]{
		ID: "task", Initial: "work",
		Context: previewCtx{Notes: map[string]string{}},
		States: map[string]previewState{
			"work": {
				Type: engine.NodeParallel,
				On:   map[string][]previewTrans{"CANCEL": {{Target: "cancelled"}}},
				States: map[string]previewState{
					"form": {Initial: "editing", States: map[string]previewState{
						"editing": {On: map[string][]previewTrans{
							"SUBMIT": {{Target: "submitted", Actions: []action.Action[previewCtx, string]{note}}},
							"*":      {{Actions: []action.Action[previewCtx, string]{note}}},
						}},
						"submitted": {},
					}},
					"docs": {Initial: "missing", States: map[string]previewState{
						"missing":  {On: map[string][]previewTrans{"UPLOAD": {{Target: "uploaded"}}}},
						"uploaded": {},
					}},
				},
			},
			"cancelled": {Type: engine.NodeFinal},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	a := engine.NewActor(m)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return a
}

func TestNextEvents_UnionOfActiveStatesAndAncestors(t *testing.T) {
	a := previewActor(t)
	if got, want := a.NextEvents(), []string{"CANCEL", "SUBMIT", "UPLOAD"}; !slices.Equal(got, want) {
		t.Errorf("NextEvents %v, want %v", got, want)
	}
	_ = a.Send(context.Background(), "SUBMIT")
	if got, want := a.NextEvents(), []string{"CANCEL", "UPLOAD"}; !slices.Equal(got, want) {
		t.Errorf("after SUBMIT: NextEvents %v, want %v", got, want)
	}
	_ = a.Send(context.Background(), "CANCEL")
	if got := a.NextEvents(); got != nil {
		t.Errorf("done actor: NextEvents %v, want none", got)
	}
}

func TestPreview_ReturnsTheNextSnapshotAndLeavesTheActorAlone(t *testing.T) {
	a := previewActor(t)
	before, err := a.Persist()
	if err != nil {
		t.Fatalf("Persist: %v", err)
	}

	next, err := a.Preview("SUBMIT")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if got, want := next.Value.Path(), "work.docs.missing | work.form.submitted"; got != want {
		t.Errorf("previewed value %q, want %q", got, want)
	}
	if next.Context.Seen != 1 || next.Context.Notes["SUBMIT"] != "seen" {
		t.Errorf("previewed context %+v, want the action applied", next.Context)
	}

	after, _ := a.Persist()
	if string(before) != string(after) {
		t.Errorf("actor changed:\n before %s\n after  %s", before, after)
	}
	if len(a.Snapshot().Context.Notes) != 0 {
		t.Errorf("an action wrote through the actor's context map: %v", a.Snapshot().Context.Notes)
	}

	_ = a.Send(context.Background(), "SUBMIT")
	if got := a.Snapshot().Value.Path(); got != next.Value.Path() {
		t.Errorf("Send reached %q, Preview said %q", got, next.Value.Path())
	}
}

func TestPreview_UnhandledEventKeepsTheSnapshot(t *testing.T) {
	a := previewActor(t)
	_ = a.Send(context.Background(), "SUBMIT")
	next, err := a.Preview("NOPE")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if got, want := next.Value.Path(), a.Snapshot().Value.Path(); got != want {
		t.Errorf("value %q, want %q", got, want)
	}
}

func TestPreview_ReportsWhatSendWould(t *testing.T) {
	a := previewActor(t)
	a.Stop()
	if _, err := a.Preview("SUBMIT"); !errors.Is(err, engine.ErrActorStopped) {
		t.Errorf("stopped actor: err %v, want ErrActorStopped", err)
	}

	m, err := engine.CreateMachine(engine.MachineConfig[chan int, string]{
		ID: "bad", Initial: "a", Context: make(chan int),
		States: map[string]engine.StateNodeConfig[chan int, string]{"a": {}},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	b := engine.NewActor(m)
	_ = b.Start(context.Background())
	if _, err := b.Preview("X"); err == nil {
		t.Error("unmarshalable context: Preview returned no error")
	}
}

func TestEnabled_EvaluatesGuardsAndSkipsUnknownNames(t *testing.T) {
	type state = engine.StateNodeConfig[int, string]
	type trans = engine.TransitionConfig[int, string]
	m, err := engine.CreateMachine(engine.MachineConfig[int, string]{
		ID: "review", Initial: "open", Context: 40,
		States: map[string]state{
			"open": {On: map[string][]trans{
				"APPROVE":  {{Target: "closed", Guard: func(score int, _ string) bool { return score >= 60 }}},
				"REJECT":   {{Target: "closed"}},
				"ESCALATE": {{Target: "closed"}},
			}},
			"closed": {},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	a := engine.NewActor(m)
	byName := func(name string) (string, bool) { return name, name != "ESCALATE" }
	if got := a.Enabled(byName); got != nil {
		t.Errorf("not started: Enabled %v, want none", got)
	}
	_ = a.Start(context.Background())
	if got, want := a.Enabled(byName), []string{"REJECT"}; !slices.Equal(got, want) {
		t.Errorf("Enabled %v, want %v", got, want)
	}
}

func cloneMachine(t *testing.T, clone func(map[string]any) map[string]any) *engine.Machine[map[string]any, string] {
	t.Helper()
	type state = engine.StateNodeConfig[map[string]any, string]
	type trans = engine.TransitionConfig[map[string]any, string]
	due := func(c map[string]any, _ string) bool {
		at, ok := c["due"].(time.Time)
		return ok && !at.IsZero()
	}
	mark := action.Assign(func(c map[string]any, e string) map[string]any { c["last"] = e; return c })
	m, err := engine.CreateMachine(engine.MachineConfig[map[string]any, string]{
		ID: "task", Initial: "open",
		Context:      map[string]any{"due": time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)},
		CloneContext: clone,
		States: map[string]state{
			"open": {
				Type: engine.NodeCompound, Initial: "draft",
				On: map[string][]trans{"CLOSE": {{Target: "closed", Guard: due, Actions: []action.Action[map[string]any, string]{mark}}}},
				States: map[string]state{
					"draft":  {On: map[string][]trans{"EDIT": {{Target: "edited"}}}},
					"edited": {},
					"hist":   {Type: engine.NodeHistory, History: engine.HistoryDeep},
				},
			},
			"closed": {On: map[string][]trans{"REOPEN": {{Target: "open.hist"}}}},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	return m
}

func TestPreview_CloneContextKeepsGoTypes(t *testing.T) {
	a := engine.NewActor(cloneMachine(t, maps.Clone[map[string]any]))
	_ = a.Start(context.Background())

	next, err := a.Preview("CLOSE")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if got := next.Value.Path(); got != "closed" {
		t.Errorf("previewed value %q, want %q: the guard saw a time.Time", got, "closed")
	}
	if _, wrote := a.Snapshot().Context["last"]; wrote {
		t.Error("the previewed action wrote into the actor's context")
	}
	if got := a.Snapshot().Value.Path(); got != "open.draft" {
		t.Errorf("actor moved to %q", got)
	}
}

func TestPreview_WithoutCloneContextLosesGoTypes(t *testing.T) {
	a := engine.NewActor(cloneMachine(t, nil))
	_ = a.Start(context.Background())
	next, err := a.Preview("CLOSE")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if got := next.Value.Path(); got != "open.draft" {
		t.Errorf("previewed value %q: the JSON copy is documented to turn time.Time into a string", got)
	}
}

func TestPreview_CloneContextCarriesHistory(t *testing.T) {
	a := engine.NewActor(cloneMachine(t, maps.Clone[map[string]any]))
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), "EDIT")
	_ = a.Send(context.Background(), "CLOSE")

	next, err := a.Preview("REOPEN")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if got := next.Value.Path(); got != "open.edited" {
		t.Errorf("previewed value %q, want deep history restored to %q", got, "open.edited")
	}
	_ = a.Send(context.Background(), "REOPEN")
	if got := a.Snapshot().Value.Path(); got != next.Value.Path() {
		t.Errorf("Send reached %q, Preview said %q", got, next.Value.Path())
	}
}

func TestNewActor_CloneContextSeparatesActors(t *testing.T) {
	m := cloneMachine(t, maps.Clone[map[string]any])
	a, b := engine.NewActor(m), engine.NewActor(m)
	_ = a.Start(context.Background())
	_ = b.Start(context.Background())
	_ = a.Send(context.Background(), "CLOSE")
	if _, shared := b.Snapshot().Context["last"]; shared {
		t.Error("two actors of one machine share a context map")
	}
}
