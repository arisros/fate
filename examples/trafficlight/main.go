// Command trafficlight is the canonical compound-state example: red → green →
// yellow → red, with a pedestrian "walk" sub-state inside red.
package main

import (
	"context"
	"fmt"

	"github.com/arisros/fate/engine"
	"github.com/arisros/fate/render"
)

func main() {
	m, err := engine.CreateMachine(engine.MachineConfig[struct{}, string]{
		ID:      "traffic",
		Initial: "red",
		States: map[string]engine.StateNodeConfig[struct{}, string]{
			"red": {
				Initial: "stop",
				States: map[string]engine.StateNodeConfig[struct{}, string]{
					"stop": {On: map[string][]engine.TransitionConfig[struct{}, string]{
						"WALK": {{Target: "walk"}},
					}},
					"walk": {On: map[string][]engine.TransitionConfig[struct{}, string]{
						"WAIT": {{Target: "stop"}},
					}},
				},
				On: map[string][]engine.TransitionConfig[struct{}, string]{
					"NEXT": {{Target: "green"}},
				},
			},
			"green":  {On: map[string][]engine.TransitionConfig[struct{}, string]{"NEXT": {{Target: "yellow"}}}},
			"yellow": {On: map[string][]engine.TransitionConfig[struct{}, string]{"NEXT": {{Target: "red"}}}},
		},
	})
	if err != nil {
		panic(err)
	}

	a := engine.NewActor(m)
	_ = a.Start(context.Background())
	fmt.Println(a.Snapshot().Value.Path()) // red.stop

	for _, ev := range []string{"WALK", "WAIT", "NEXT", "NEXT", "NEXT"} {
		_ = a.Send(context.Background(), ev)
		fmt.Println(ev, "->", a.Snapshot().Value.Path())
	}

	fmt.Println("\nMermaid diagram:")
	fmt.Println(render.Mermaid(m.Describe(), render.MermaidOptions{}))
}
