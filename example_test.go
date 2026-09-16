package fate_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	fate "github.com/arisros/fate"
)

func Example() {
	type Ctx struct{ Count int }

	m, err := fate.CreateMachine(fate.MachineConfig[Ctx, string]{
		ID:      "counter",
		Initial: "active",
		States: map[string]fate.StateNodeConfig[Ctx, string]{
			"active": {On: map[string][]fate.TransitionConfig[Ctx, string]{
				"INC": {{Actions: []fate.Action[Ctx, string]{
					fate.Assign(func(c Ctx, _ string) Ctx { c.Count++; return c }),
				}}},
			}},
		},
	})
	if err != nil {
		panic(err)
	}

	a := fate.NewActor(m)
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), "INC")
	_ = a.Send(context.Background(), "INC")

	fmt.Println(a.Snapshot().Context.Count)
	// Output: 2
}

func Example_persistence() {
	type Ctx struct{ Count int }

	build := func() *fate.Machine[Ctx, string] {
		m, _ := fate.CreateMachine(fate.MachineConfig[Ctx, string]{
			ID:      "counter",
			Initial: "active",
			States: map[string]fate.StateNodeConfig[Ctx, string]{
				"active": {On: map[string][]fate.TransitionConfig[Ctx, string]{
					"INC": {{Actions: []fate.Action[Ctx, string]{
						fate.Assign(func(c Ctx, _ string) Ctx { c.Count++; return c }),
					}}},
				}},
			},
		})
		return m
	}

	a := fate.NewActor(build())
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), "INC")

	blob, _ := a.Persist()
	restored, _ := fate.NewActorFromSnapshot[Ctx, string](build(), blob)
	_ = restored.Send(context.Background(), "INC")

	fmt.Println(restored.Snapshot().Context.Count)
	// Output: 2
}

func Example_delayedTransition() {
	m, _ := fate.CreateMachine(fate.MachineConfig[struct{}, string]{
		ID:      "blink",
		Initial: "off",
		States: map[string]fate.StateNodeConfig[struct{}, string]{
			"off": {After: map[time.Duration][]fate.TransitionConfig[struct{}, string]{
				time.Hour: {{Target: "on"}},
			}},
			"on": {Type: fate.NodeFinal},
		},
	})

	a := fate.NewActor(m)
	_ = a.Start(context.Background())
	fmt.Println(a.Snapshot().Value.Path())

	a.FireTimer(a.PendingTimers()[0].ID)
	fmt.Println(a.Snapshot().Value.Path())
	// Output:
	// off
	// on
}

func Example_tooling() {
	type Ctx struct {
		Score int `json:"score"`
	}
	type ReviewView struct {
		Score  int  `json:"score"`
		Passes bool `json:"passes"`
	}

	m, err := fate.CreateMachine(fate.MachineConfig[Ctx, string]{
		ID:      "review",
		Initial: "pending",
		Context: Ctx{Score: 72},
		States: map[string]fate.StateNodeConfig[Ctx, string]{
			"pending": {
				UIState: fate.UIStateOf(func(c Ctx) ReviewView {
					return ReviewView{Score: c.Score, Passes: c.Score >= 60}
				}),
				On: map[string][]fate.TransitionConfig[Ctx, string]{
					"DECIDE": {{
						Target:   "approved",
						Guard:    func(c Ctx, _ string) bool { return c.Score >= 60 },
						CondMeta: fate.Gates(fate.Field("$.score").Gte(60)).Sample(`{"score":60}`),
					}},
				},
			},
			"approved": {Type: fate.NodeFinal},
		},
	})
	if err != nil {
		panic(err)
	}

	gate, _ := json.Marshal(m.Describe().States["pending"].On["DECIDE"][0].CondMeta)
	fmt.Println(string(gate))

	a := fate.NewActor(m)
	_ = a.Start(context.Background())
	s := a.Snapshot()
	views, _ := m.UIState(s.Value, s.Context)
	fmt.Println(string(views["pending"]))
	// Output:
	// {"fields":[{"path":"$.score","op":"gte","value":60}],"sample":{"score":60}}
	// {"score":72,"passes":true}
}
