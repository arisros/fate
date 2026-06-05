# Getting started

## Install

```sh
go get github.com/arisros/fate
```

The engine has no dependencies beyond the standard library. The optional Temporal
integration is a separate module (`go get github.com/arisros/fate/temporal`).

## A first machine

This counter has one state and mutates its context on each event:

```go
package main

import (
    "context"
    "fmt"

    "github.com/arisros/fate"
)

type Ctx struct{ Count int }

type Evt interface{ isEvt() }
type Inc struct{}
type Reset struct{}

func (Inc) isEvt()   {}
func (Reset) isEvt() {}

func main() {
    m, err := fate.CreateMachine(fate.MachineConfig[Ctx, Evt]{
        ID:      "counter",
        Initial: "active",
        States: map[string]fate.StateNodeConfig[Ctx, Evt]{
            "active": {On: map[string][]fate.TransitionConfig[Ctx, Evt]{
                "Inc":   {{Actions: []fate.Action[Ctx, Evt]{fate.Assign(func(c Ctx, _ Evt) Ctx { c.Count++; return c })}}},
                "Reset": {{Target: "active", Actions: []fate.Action[Ctx, Evt]{fate.Assign(func(c Ctx, _ Evt) Ctx { c.Count = 0; return c })}}},
            }},
        },
    })
    if err != nil {
        panic(err)
    }

    a := fate.NewActor(m)
    _ = a.Start(context.Background())
    _ = a.Send(context.Background(), Inc{})
    _ = a.Send(context.Background(), Inc{})

    fmt.Println(a.Snapshot().Context.Count) // 2
}
```

## The shape of the API

- `CreateMachine` validates a config and returns an immutable `*Machine`.
- `NewActor` makes a running instance; `Start` runs the initial entry; `Send`
  dispatches an event.
- `Snapshot` reads the current state; `Subscribe` observes changes; `Persist` and
  `NewActorFromSnapshot` round-trip to JSON.

## Where to go next

- [Defining machines](defining-machines.md) — hierarchy, parallel regions,
  guards, actions, history.
- [Effects and adapters](effects-and-adapters.md) — delays and invocations.
- [Persistence and determinism](persistence-and-determinism.md) — why the engine
  is replay-safe.
- [Temporal](temporal.md) — durable execution.

The `examples/` directory has runnable programs (a traffic light, a real-time
timer driver) and the package's testable `Example` functions double as
documentation on [pkg.go.dev](https://pkg.go.dev/github.com/arisros/fate).
