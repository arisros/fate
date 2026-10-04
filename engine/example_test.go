package engine_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/arisros/fate/action"
	"github.com/arisros/fate/describe"
	"github.com/arisros/fate/engine"
)

// Example builds a one-state counter machine, drives it with two events, and
// reads the accumulated context.
func Example() {
	type Ctx struct{ Count int }

	m, err := engine.CreateMachine(engine.MachineConfig[Ctx, string]{
		ID:      "counter",
		Initial: "active",
		States: map[string]engine.StateNodeConfig[Ctx, string]{
			"active": {On: map[string][]engine.TransitionConfig[Ctx, string]{
				"INC": {{Actions: []action.Action[Ctx, string]{
					action.Assign(func(c Ctx, _ string) Ctx { c.Count++; return c }),
				}}},
			}},
		},
	})
	if err != nil {
		panic(err)
	}

	a := engine.NewActor(m)
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), "INC")
	_ = a.Send(context.Background(), "INC")

	fmt.Println(a.Snapshot().Context.Count)
	// Output: 2
}

// Example_persistence shows that an actor round-trips through a JSON snapshot:
// the restored actor continues from exactly where the original left off.
func Example_persistence() {
	type Ctx struct{ Count int }

	build := func() *engine.Machine[Ctx, string] {
		m, _ := engine.CreateMachine(engine.MachineConfig[Ctx, string]{
			ID:      "counter",
			Initial: "active",
			States: map[string]engine.StateNodeConfig[Ctx, string]{
				"active": {On: map[string][]engine.TransitionConfig[Ctx, string]{
					"INC": {{Actions: []action.Action[Ctx, string]{
						action.Assign(func(c Ctx, _ string) Ctx { c.Count++; return c }),
					}}},
				}},
			},
		})
		return m
	}

	a := engine.NewActor(build())
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), "INC")

	blob, _ := a.Persist()
	restored, _ := engine.NewActorFromSnapshot[Ctx, string](build(), blob)
	_ = restored.Send(context.Background(), "INC")

	fmt.Println(restored.Snapshot().Context.Count)
	// Output: 2
}

// Example_delayedTransition shows the clock-agnostic timer model: the engine
// records a pending "after" timer but never fires it. A driver (here, the test
// itself; in production the fate/temporal adapter) decides the delay elapsed and
// calls FireTimer.
func Example_delayedTransition() {
	m, _ := engine.CreateMachine(engine.MachineConfig[struct{}, string]{
		ID:      "blink",
		Initial: "off",
		States: map[string]engine.StateNodeConfig[struct{}, string]{
			"off": {After: map[time.Duration][]engine.TransitionConfig[struct{}, string]{
				time.Hour: {{Target: "on"}},
			}},
			"on": {Type: engine.NodeFinal},
		},
	})

	a := engine.NewActor(m)
	_ = a.Start(context.Background())
	fmt.Println(a.Snapshot().Value.Path())

	// A driver pulls the pending timer and fires it once the delay elapses.
	a.FireTimer(a.PendingTimers()[0].ID)
	fmt.Println(a.Snapshot().Value.Path())
	// Output:
	// off
	// on
}

// Example_tooling annotates a guard with Gates and a state with UIStateOf, then
// reads both the way a viewer would: the gate from the descriptor, the view
// model from the live snapshot.
func Example_tooling() {
	type Ctx struct {
		Score int `json:"score"`
	}
	type ReviewView struct {
		Score  int  `json:"score"`
		Passes bool `json:"passes"`
	}

	m, err := engine.CreateMachine(engine.MachineConfig[Ctx, string]{
		ID:      "review",
		Initial: "pending",
		Context: Ctx{Score: 72},
		States: map[string]engine.StateNodeConfig[Ctx, string]{
			"pending": {
				UIState: describe.UIStateOf(func(c Ctx) ReviewView {
					return ReviewView{Score: c.Score, Passes: c.Score >= 60}
				}),
				On: map[string][]engine.TransitionConfig[Ctx, string]{
					"DECIDE": {{
						Target:   "approved",
						Guard:    func(c Ctx, _ string) bool { return c.Score >= 60 },
						CondMeta: action.Gates(action.Field("$.score").Gte(60)).Sample(`{"score":60}`),
					}},
				},
			},
			"approved": {Type: engine.NodeFinal},
		},
	})
	if err != nil {
		panic(err)
	}

	gate, _ := json.Marshal(m.Describe().States["pending"].On["DECIDE"][0].CondMeta)
	fmt.Println(string(gate))

	a := engine.NewActor(m)
	_ = a.Start(context.Background())
	s := a.Snapshot()
	views, _ := m.UIState(s.Value, s.Context)
	fmt.Println(string(views["pending"]))
	// Output:
	// {"fields":[{"path":"$.score","op":"gte","value":60}],"sample":{"score":60}}
	// {"score":72,"passes":true}
}

// ExampleActor_NextEvents lists the events a task accepts in its current state
// and previews one of them without sending it.
func ExampleActor_NextEvents() {
	type Ctx struct{ Score int }
	type state = engine.StateNodeConfig[Ctx, string]
	type transition = engine.TransitionConfig[Ctx, string]

	m, err := engine.CreateMachine(engine.MachineConfig[Ctx, string]{
		ID:      "review",
		Initial: "open",
		Context: Ctx{Score: 40},
		States: map[string]state{
			"open": {On: map[string][]transition{
				"APPROVE": {{Target: "approved", Guard: func(c Ctx, _ string) bool { return c.Score >= 60 }}},
				"REJECT":  {{Target: "rejected"}},
			}},
			"approved": {Type: engine.NodeFinal},
			"rejected": {Type: engine.NodeFinal},
		},
	})
	if err != nil {
		panic(err)
	}

	a := engine.NewActor(m)
	_ = a.Start(context.Background())

	fmt.Println(a.NextEvents())
	fmt.Println(a.Enabled(func(name string) (string, bool) { return name, true }))

	next, _ := a.Preview("REJECT")
	fmt.Println(next.Value.Path(), next.Status)
	fmt.Println(a.Snapshot().Value.Path())
	// Output:
	// [APPROVE REJECT]
	// [REJECT]
	// rejected done
	// open
}

// ExampleActor_SubscribeSteps records what each step of an actor did: which
// transition fired and which states were left and entered.
func ExampleActor_SubscribeSteps() {
	type state = engine.StateNodeConfig[struct{}, string]
	type transition = engine.TransitionConfig[struct{}, string]

	m, err := engine.CreateMachine(engine.MachineConfig[struct{}, string]{
		ID:      "task",
		Initial: "draft",
		States: map[string]state{
			"draft":  {On: map[string][]transition{"SUBMIT": {{Target: "review"}}}},
			"review": {On: map[string][]transition{"RETURN": {{Target: "review"}}}},
		},
	})
	if err != nil {
		panic(err)
	}

	a := engine.NewActor(m)
	a.SubscribeSteps(func(s engine.Step) {
		fmt.Println(s.Seq, s.Cause, s.Event, s.Exited, s.Entered)
	})
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), "SUBMIT")
	_ = a.Send(context.Background(), "RETURN")
	// Output:
	// 1 start  [] [draft]
	// 2 event SUBMIT [draft] [review]
	// 3 event RETURN [review] [review]
}

// Example_meta attaches host data to a state and a transition and reads it back
// from the descriptor, where a form builder or a viewer would find it.
func Example_meta() {
	type state = engine.StateNodeConfig[struct{}, string]
	type transition = engine.TransitionConfig[struct{}, string]

	m, err := engine.CreateMachine(engine.MachineConfig[struct{}, string]{
		ID:      "task",
		Initial: "signup",
		States: map[string]state{
			"signup": {
				Meta: map[string]any{"form": "signup_form"},
				On: map[string][]transition{
					"SUBMIT": {{Target: "done", Meta: map[string]any{"title": "Submit", "order": 1}}},
				},
			},
			"done": {Type: engine.NodeFinal},
		},
	})
	if err != nil {
		panic(err)
	}

	signup := m.Describe().States["signup"]
	fmt.Println(string(signup.Meta))
	fmt.Println(string(signup.On["SUBMIT"][0].Meta))
	// Output:
	// {"form":"signup_form"}
	// {"order":1,"title":"Submit"}
}

// ExampleMachine_Lint finds a state nothing enters and a state nothing leaves.
func ExampleMachine_Lint() {
	type state = engine.StateNodeConfig[struct{}, string]
	type transition = engine.TransitionConfig[struct{}, string]

	m, err := engine.CreateMachine(engine.MachineConfig[struct{}, string]{
		ID:      "task",
		Initial: "draft",
		States: map[string]state{
			"draft":    {On: map[string][]transition{"SUBMIT": {{Target: "review"}}}},
			"review":   {},
			"returned": {On: map[string][]transition{"SUBMIT": {{Target: "review"}}}},
		},
	})
	if err != nil {
		panic(err)
	}

	for _, f := range m.Lint() {
		fmt.Println(f.State, f.Kind)
	}
	// Output:
	// returned unreachable
	// review dead_end
}
