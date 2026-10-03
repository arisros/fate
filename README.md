# fate

**A statechart engine for Go.**

`fate` implements [Harel statecharts](https://en.wikipedia.org/wiki/State_diagram#Harel_statechart):
hierarchical states, parallel regions, and deep/shallow history — not a flat
finite automaton. It is inspired by the semantics of SCXML and
[XState v5](https://stately.ai/docs), expressed idiomatically in Go with strong
typing via generics.

```go
import (
	"github.com/arisros/fate/action"
	"github.com/arisros/fate/engine"
)
```

- **Zero dependencies.** The engine imports only the standard library. The
  optional [Temporal](https://temporal.io) integration lives in a separate
  module so you never pull in the Temporal SDK unless you ask for it.
- **Deterministic & persistable.** A `Machine` is immutable and shareable; an
  `Actor`'s state serialises to JSON and restores byte-for-byte. Safe to drive
  from deterministic environments such as Temporal workflows.
- **Hierarchy, parallelism, history, guards, actions, delayed transitions, and
  invoked/spawned child actors** — the full statechart feature set.

> **Status:** pre-release (`v0.x`). The API may change between minor versions
> until `v1.0.0`. See [CHANGELOG.md](./CHANGELOG.md).

## Install

```sh
go get github.com/arisros/fate
```

Temporal integration (optional, separate module):

```sh
go get github.com/arisros/fate/temporal
```

## Quickstart

```go
package main

import (
	"context"
	"fmt"

	"github.com/arisros/fate/action"
	"github.com/arisros/fate/engine"
)

type Ctx struct{ Count int }

type Evt interface{ isEvt() }
type Inc struct{}
type Reset struct{}

func (Inc) isEvt()   {}
func (Reset) isEvt() {}

func main() {
	m, err := engine.CreateMachine(engine.MachineConfig[Ctx, Evt]{
		ID:      "counter",
		Initial: "active",
		Context: Ctx{},
		States: map[string]engine.StateNodeConfig[Ctx, Evt]{
			"active": {
				On: map[string][]engine.TransitionConfig[Ctx, Evt]{
					"Inc": {{Actions: []action.Action[Ctx, Evt]{
						action.Assign(func(c Ctx, _ Evt) Ctx { c.Count++; return c }),
					}}},
					"Reset": {{Target: "active", Actions: []action.Action[Ctx, Evt]{
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
	_ = a.Send(context.Background(), Inc{})
	_ = a.Send(context.Background(), Inc{})

	fmt.Println(a.Snapshot().Context.Count) // 2

	// Persist and restore — the restored actor is identical.
	blob, _ := a.Persist()
	b, _ := engine.NewActorFromSnapshot[Ctx, Evt](m, blob)
	fmt.Println(b.Snapshot().Context.Count) // 2
}
```

See [`examples/`](./examples) for hierarchical, parallel, history, delayed, and
invoked-actor machines, and the package
[examples](https://pkg.go.dev/github.com/arisros/fate/engine#pkg-examples) on pkg.go.dev.

## Packages

| Package | What it holds |
|---|---|
| `engine` | `CreateMachine`, `Setup`, `Machine`, `Actor`: build a machine and run it |
| `action` | Actions (`Assign`, `Raise`, `Log`), guards (`And`, `Or`, `Not`), conditions (`InState`) |
| `effect` | Timers and invocations a host drives (`PendingTimer`, `Invocation`) |
| `persist` | The active configuration (`StateValue`) and `Snapshot` |
| `describe` | The JSON `MachineDescriptor` and `UIState` view models for tooling |
| `render`, `diff`, `snapshot`, `httphandler`, `testing` | Optional helpers built on the engine |

## Concepts

| Statechart concept | fate |
|---|---|
| Atomic / compound / parallel / final state | `NodeAtomic` / `NodeCompound` / `NodeParallel` / `NodeFinal` |
| History (shallow / deep) | `NodeHistory` with `HistoryShallow` / `HistoryDeep` |
| Guarded transition | `TransitionConfig.Guard` (+ `And`/`Or`/`Not`/`StateIn` combinators) |
| Entry/exit & transition actions | `Assign`, `Raise`, `Log`, `EnqueueActions` |
| Delayed (`after`) transitions | `StateNodeConfig.After`, driven by an adapter via `PendingTimers`/`FireTimer` |
| Invoked work / spawned child machines | `StateNodeConfig.Invoke`, driven via `PendingInvocations`/`ResolveInvocation` |
| Snapshot persistence | `Actor.Persist` / `NewActorFromSnapshot` |
| Visualisation | `RenderASCII`, `RenderMermaid`, `RenderGraphJSON` |

## Tooling

- **`fate`** — a small CLI to render (ASCII / Mermaid / graph), inspect, and diff
  statecharts from JSON descriptors and snapshots. Install with
  `go install github.com/arisros/fate/cmd/fate@latest`.
- **[fate-studio](https://github.com/arisros/fate-studio)** — a separate project:
  an embeddable, self-hosted chart viewer and live simulator that renders and
  drives any fate machine in the browser. It lives in its own repository so the
  engine stays dependency-free.

## Documentation

- [Documentation index](./docs/README.md) — concepts and guides
- [Concepts](./docs/concepts.md) · [Defining machines](./docs/guide/defining-machines.md) ·
  [Persistence & determinism](./docs/guide/persistence-and-determinism.md) ·
  [Effects & adapters](./docs/guide/effects-and-adapters.md) ·
  [Temporal](./docs/guide/temporal.md)
- [Architecture Decision Records](./docs/adr)
- Per-symbol reference on [pkg.go.dev](https://pkg.go.dev/github.com/arisros/fate/engine)

## License

[MIT](./LICENSE) © Aris Kurniawan
