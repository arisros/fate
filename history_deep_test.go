package fate_test

import (
	"context"
	"testing"

	sc "github.com/arisros/fate"
)

type dhCtx struct{}
type dhEvt interface{ isDhEvt() }

type dhEvtForwardReview struct{}
type dhEvtForwardFinal struct{}
type dhEvtShip struct{}
type dhEvtSuspend struct{}
type dhEvtResume struct{}

func (dhEvtForwardReview) isDhEvt() {}
func (dhEvtForwardFinal) isDhEvt()  {}
func (dhEvtShip) isDhEvt()          {}
func (dhEvtSuspend) isDhEvt()       {}
func (dhEvtResume) isDhEvt()        {}

func (dhEvtForwardReview) EventName() string { return "FORWARD_REVIEW" }
func (dhEvtForwardFinal) EventName() string  { return "FORWARD_FINAL" }
func (dhEvtShip) EventName() string          { return "SHIP" }
func (dhEvtSuspend) EventName() string       { return "SUSPEND" }
func (dhEvtResume) EventName() string        { return "RESUME" }

func newDeepHistoryMachine(t *testing.T, depth sc.History) *sc.Machine[dhCtx, dhEvt] {
	t.Helper()
	m, err := sc.CreateMachine(sc.MachineConfig[dhCtx, dhEvt]{
		ID:      "deep-history-fixture",
		Initial: "editor",
		States: map[string]sc.StateNodeConfig[dhCtx, dhEvt]{
			"editor": {
				Initial: "doc",
				On: map[string][]sc.TransitionConfig[dhCtx, dhEvt]{
					"SUSPEND": {{Target: "suspended"}},
				},
				States: map[string]sc.StateNodeConfig[dhCtx, dhEvt]{
					"doc": {
						Initial: "draft",
						States: map[string]sc.StateNodeConfig[dhCtx, dhEvt]{
							"draft": {
								On: map[string][]sc.TransitionConfig[dhCtx, dhEvt]{
									"FORWARD_REVIEW": {{Target: "review"}},
								},
							},
							"review": {
								On: map[string][]sc.TransitionConfig[dhCtx, dhEvt]{
									"FORWARD_FINAL": {{Target: "final"}},
								},
							},
							"final": {
								Initial: "proofing",
								States: map[string]sc.StateNodeConfig[dhCtx, dhEvt]{
									"proofing": {
										On: map[string][]sc.TransitionConfig[dhCtx, dhEvt]{
											"SHIP": {{Target: "shipped"}},
										},
									},
									"shipped": {},
								},
							},
						},
					},
					"hist_deep": {
						Type:    sc.NodeHistory,
						History: depth,
						Default: "doc",
					},
				},
			},
			"suspended": {
				On: map[string][]sc.TransitionConfig[dhCtx, dhEvt]{
					"RESUME": {{Target: "editor.hist_deep"}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	return m
}

func driveToFinalShipped(t *testing.T, a *sc.Actor[dhCtx, dhEvt]) {
	t.Helper()
	_ = a.Start(context.Background())
	if got := a.Snapshot().Value.Path(); got != "editor.doc.draft" {
		t.Fatalf("initial: got %q want editor.doc.draft", got)
	}
	_ = a.Send(context.Background(), dhEvtForwardReview{})
	_ = a.Send(context.Background(), dhEvtForwardFinal{})
	_ = a.Send(context.Background(), dhEvtShip{})
	if got := a.Snapshot().Value.Path(); got != "editor.doc.final.shipped" {
		t.Fatalf("setup: got %q want editor.doc.final.shipped", got)
	}
}

func TestHistory_DeepRestoresNestedSubtree(t *testing.T) {
	a := sc.NewActor(newDeepHistoryMachine(t, sc.HistoryDeep))
	driveToFinalShipped(t, a)

	_ = a.Send(context.Background(), dhEvtSuspend{})
	if got := a.Snapshot().Value.Path(); got != "suspended" {
		t.Fatalf("after SUSPEND: got %q want suspended", got)
	}

	_ = a.Send(context.Background(), dhEvtResume{})
	if got := a.Snapshot().Value.Path(); got != "editor.doc.final.shipped" {
		t.Errorf("deep restore: got %q want editor.doc.final.shipped", got)
	}
}

func TestHistory_ShallowDoesNotRestoreNestedSubtree(t *testing.T) {
	a := sc.NewActor(newDeepHistoryMachine(t, sc.HistoryShallow))
	driveToFinalShipped(t, a)

	_ = a.Send(context.Background(), dhEvtSuspend{})
	_ = a.Send(context.Background(), dhEvtResume{})
	if got := a.Snapshot().Value.Path(); got == "editor.doc.final.shipped" {
		t.Errorf("shallow regressed to deep behavior: path=%q", got)
	}
	if got := a.Snapshot().Value.Path(); got != "editor.doc.draft" {
		t.Errorf("shallow restore: got %q want editor.doc.draft", got)
	}
}

func TestHistory_DeepRestoresAcrossMultipleCycles(t *testing.T) {
	a := sc.NewActor(newDeepHistoryMachine(t, sc.HistoryDeep))
	driveToFinalShipped(t, a)

	_ = a.Send(context.Background(), dhEvtSuspend{})
	_ = a.Send(context.Background(), dhEvtResume{})
	if got := a.Snapshot().Value.Path(); got != "editor.doc.final.shipped" {
		t.Fatalf("cycle 1 restore: got %q want editor.doc.final.shipped", got)
	}

	_ = a.Send(context.Background(), dhEvtSuspend{})
	_ = a.Send(context.Background(), dhEvtResume{})
	if got := a.Snapshot().Value.Path(); got != "editor.doc.final.shipped" {
		t.Errorf("cycle 2 restore: got %q want editor.doc.final.shipped", got)
	}
}

func TestHistory_DeepWithoutMemoryFallsBackToDefault(t *testing.T) {
	m, err := sc.CreateMachine(sc.MachineConfig[dhCtx, dhEvt]{
		ID:      "deep-default",
		Initial: "suspended",
		States: map[string]sc.StateNodeConfig[dhCtx, dhEvt]{
			"suspended": {
				On: map[string][]sc.TransitionConfig[dhCtx, dhEvt]{
					"RESUME": {{Target: "editor.hist_deep"}},
				},
			},
			"editor": {
				Initial: "doc",
				States: map[string]sc.StateNodeConfig[dhCtx, dhEvt]{
					"doc": {
						Initial: "draft",
						States: map[string]sc.StateNodeConfig[dhCtx, dhEvt]{
							"draft":  {},
							"review": {},
						},
					},
					"hist_deep": {
						Type:    sc.NodeHistory,
						History: sc.HistoryDeep,
						Default: "doc.review",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	a := sc.NewActor(m)
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), dhEvtResume{})
	if got := a.Snapshot().Value.Path(); got != "editor.doc.review" {
		t.Errorf("deep default fallback: got %q want editor.doc.review", got)
	}
}

func TestHistory_DeepFallsBackToInitialWhenNoDefault(t *testing.T) {
	m, err := sc.CreateMachine(sc.MachineConfig[dhCtx, dhEvt]{
		ID:      "deep-initial",
		Initial: "suspended",
		States: map[string]sc.StateNodeConfig[dhCtx, dhEvt]{
			"suspended": {
				On: map[string][]sc.TransitionConfig[dhCtx, dhEvt]{
					"RESUME": {{Target: "editor.hist_deep"}},
				},
			},
			"editor": {
				Initial: "doc",
				States: map[string]sc.StateNodeConfig[dhCtx, dhEvt]{
					"doc": {
						Initial: "draft",
						States: map[string]sc.StateNodeConfig[dhCtx, dhEvt]{
							"draft": {},
						},
					},
					"hist_deep": {Type: sc.NodeHistory, History: sc.HistoryDeep},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	a := sc.NewActor(m)
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), dhEvtResume{})
	if got := a.Snapshot().Value.Path(); got != "editor.doc.draft" {
		t.Errorf("deep initial fallback: got %q want editor.doc.draft", got)
	}
}

func TestHistory_DeepMemoryRoundtripsThroughPersist(t *testing.T) {
	a := sc.NewActor(newDeepHistoryMachine(t, sc.HistoryDeep))
	driveToFinalShipped(t, a)
	_ = a.Send(context.Background(), dhEvtSuspend{})

	persisted, err := a.Persist()
	if err != nil {
		t.Fatalf("Persist: %v", err)
	}
	restored, err := sc.NewActorFromSnapshot(newDeepHistoryMachine(t, sc.HistoryDeep), persisted)
	if err != nil {
		t.Fatalf("NewActorFromSnapshot: %v", err)
	}
	_ = restored.Send(context.Background(), dhEvtResume{})
	if got := restored.Snapshot().Value.Path(); got != "editor.doc.final.shipped" {
		t.Errorf("post-restore RESUME: got %q want editor.doc.final.shipped", got)
	}
}

func TestHistory_DeepMemoryHandlesOlderSnapshotWithoutDeepField(t *testing.T) {
	old := []byte(`{"version":1,"status":"running","value":"suspended","context":{}}`)
	restored, err := sc.NewActorFromSnapshot(newDeepHistoryMachine(t, sc.HistoryDeep), old)
	if err != nil {
		t.Fatalf("NewActorFromSnapshot on legacy snapshot: %v", err)
	}
	_ = restored.Send(context.Background(), dhEvtResume{})
	if got := restored.Snapshot().Value.Path(); got != "editor.doc.draft" {
		t.Errorf("legacy-snapshot RESUME: got %q want editor.doc.draft", got)
	}
}
