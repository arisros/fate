// Command quickstart is the README example: a counter machine driven by events,
// then persisted and restored.
package main

import (
	"context"
	"fmt"

	"github.com/arisros/fate/action"
	"github.com/arisros/fate/engine"
)

// Ctx is the machine's typed context.
type Ctx struct{ Count int }

// Evt is a sealed event interface.
type Evt interface{ isEvt() }

type inc struct{}
type reset struct{}

func (inc) isEvt()   {}
func (reset) isEvt() {}

func main() {
	m, err := engine.CreateMachine(engine.MachineConfig[Ctx, Evt]{
		ID:      "counter",
		Initial: "active",
		States: map[string]engine.StateNodeConfig[Ctx, Evt]{
			"active": {
				On: map[string][]engine.TransitionConfig[Ctx, Evt]{
					"inc": {{Actions: []action.Action[Ctx, Evt]{
						action.Assign(func(c Ctx, _ Evt) Ctx { c.Count++; return c }),
					}}},
					"reset": {{Target: "active", Actions: []action.Action[Ctx, Evt]{
						action.Assign(func(c Ctx, _ Evt) Ctx { c.Count = 0; return c }),
					}}},
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}

	a := engine.NewActor(m)
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), inc{})
	_ = a.Send(context.Background(), inc{})
	fmt.Println("count:", a.Snapshot().Context.Count) // 2

	blob, _ := a.Persist()
	b, _ := engine.NewActorFromSnapshot[Ctx, Evt](m, blob)
	fmt.Println("restored count:", b.Snapshot().Context.Count) // 2
}
