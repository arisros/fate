// Command quickstart is the README example: a counter machine driven by events,
// then persisted and restored.
package main

import (
	"context"
	"fmt"

	"github.com/arisros/fate"
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
	m, err := fate.CreateMachine(fate.MachineConfig[Ctx, Evt]{
		ID:      "counter",
		Initial: "active",
		States: map[string]fate.StateNodeConfig[Ctx, Evt]{
			"active": {
				On: map[string][]fate.TransitionConfig[Ctx, Evt]{
					"inc": {{Actions: []fate.Action[Ctx, Evt]{
						fate.Assign(func(c Ctx, _ Evt) Ctx { c.Count++; return c }),
					}}},
					"reset": {{Target: "active", Actions: []fate.Action[Ctx, Evt]{
						fate.Assign(func(c Ctx, _ Evt) Ctx { c.Count = 0; return c }),
					}}},
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}

	a := fate.NewActor(m)
	_ = a.Start(context.Background())
	_ = a.Send(context.Background(), inc{})
	_ = a.Send(context.Background(), inc{})
	fmt.Println("count:", a.Snapshot().Context.Count) // 2

	blob, _ := a.Persist()
	b, _ := fate.NewActorFromSnapshot[Ctx, Evt](m, blob)
	fmt.Println("restored count:", b.Snapshot().Context.Count) // 2
}
