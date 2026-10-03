package main

import (
	"context"
	"fmt"
	"time"

	"github.com/arisros/fate/engine"
	"github.com/arisros/fate/persist"
)

func main() {
	m, err := engine.CreateMachine(engine.MachineConfig[struct{}, string]{
		ID:      "countdown",
		Initial: "three",
		States: map[string]engine.StateNodeConfig[struct{}, string]{
			"three": {After: after("two")},
			"two":   {After: after("one")},
			"one":   {After: after("liftoff")},
			"liftoff": {
				Type: engine.NodeFinal,
			},
		},
	})
	if err != nil {
		panic(err)
	}

	a := engine.NewActor(m)
	_ = a.Start(context.Background())
	fmt.Println(a.Snapshot().Value.Path())

	for a.Snapshot().Status == persist.StatusRunning {
		pending := a.PendingTimers()
		if len(pending) == 0 {
			break
		}
		time.Sleep(pending[0].Delay)
		a.FireTimer(pending[0].ID)
		fmt.Println(a.Snapshot().Value.Path())
	}
}

func after(target string) map[time.Duration][]engine.TransitionConfig[struct{}, string] {
	return map[time.Duration][]engine.TransitionConfig[struct{}, string]{
		100 * time.Millisecond: {{Target: target}},
	}
}
