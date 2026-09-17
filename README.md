<p align="center">
  <img src="docs/public/logo.svg" width="96" height="96" alt="fate">
</p>

<h1 align="center">fate</h1>

<p align="center">A statechart engine for Go.</p>

<p align="center">
  <a href="https://pkg.go.dev/github.com/arisros/fate"><img src="https://pkg.go.dev/badge/github.com/arisros/fate.svg" alt="Go Reference"></a>
  <a href="https://github.com/arisros/fate/actions/workflows/ci.yml"><img src="https://github.com/arisros/fate/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://goreportcard.com/report/github.com/arisros/fate"><img src="https://goreportcard.com/badge/github.com/arisros/fate" alt="Go Report Card"></a>
  <a href="./LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="MIT license"></a>
</p>

<p align="center">
  <a href="https://fate.arisjirat.com">Website</a> ·
  <a href="https://fate.arisjirat.com/guide/getting-started">Documentation</a> ·
  <a href="https://pkg.go.dev/github.com/arisros/fate">API reference</a> ·
  <a href="https://fate-studio.arisjirat.com">Studio</a>
</p>

---

fate implements [Harel statecharts](https://en.wikipedia.org/wiki/State_diagram#Harel_statechart):
hierarchical states, parallel regions, and deep/shallow history. Its semantics
follow SCXML and [XState v5](https://stately.ai/docs), expressed in Go with
generics for the context and event types.

The engine imports only the standard library. It computes state and never
performs side effects: delays and invocations are surfaced as data for an
adapter to carry out, which keeps a machine deterministic and safe to run inside
a durable runtime such as [Temporal](https://temporal.io).

> **Status:** pre-release (`v0.x`). The API may change between minor versions
> until `v1.0.0`. See [CHANGELOG.md](./CHANGELOG.md) and the
> [versioning policy](https://fate.arisjirat.com/versioning).

## Contents

- [Install](#install)
- [Quickstart](#quickstart)
- [Feature map](#feature-map)
- [Documentation](#documentation)
- [Tooling](#tooling)
- [Contributing](#contributing)
- [License](#license)

## Install

```sh
go get github.com/arisros/fate
```

The Temporal integration is a separate module, so the engine's dependency graph
stays empty unless you ask for it:

```sh
go get github.com/arisros/fate/temporal
```

Requires Go 1.24 or later.

## Quickstart

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
		Context: Ctx{},
		States: map[string]fate.StateNodeConfig[Ctx, Evt]{
			"active": {
				On: map[string][]fate.TransitionConfig[Ctx, Evt]{
					"Inc": {{Actions: []fate.Action[Ctx, Evt]{
						fate.Assign(func(c Ctx, _ Evt) Ctx { c.Count++; return c }),
					}}},
					"Reset": {{Target: "active", Actions: []fate.Action[Ctx, Evt]{
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
	_ = a.Send(context.Background(), Inc{})
	_ = a.Send(context.Background(), Inc{})

	fmt.Println(a.Snapshot().Context.Count) // 2

	// Persist and restore. The restored actor is identical.
	blob, _ := a.Persist()
	b, _ := fate.NewActorFromSnapshot[Ctx, Evt](m, blob)
	fmt.Println(b.Snapshot().Context.Count) // 2
}
```

Runnable programs for hierarchical, parallel, history, delayed, and
invoked-actor machines live in [`examples/`](./examples). The package's testable
examples are also on
[pkg.go.dev](https://pkg.go.dev/github.com/arisros/fate#pkg-examples).

## Feature map

| Statechart concept | fate |
| --- | --- |
| Atomic / compound / parallel / final state | `NodeAtomic`, `NodeCompound`, `NodeParallel`, `NodeFinal` |
| History (shallow / deep) | `NodeHistory` with `HistoryShallow` / `HistoryDeep` |
| Guarded transition | `TransitionConfig.Guard`, with `And` / `Or` / `Not` / `StateIn` |
| Entry, exit, and transition actions | `Assign`, `Raise`, `Log`, `EnqueueActions` |
| Delayed (`after`) transitions | `StateNodeConfig.After`, driven by `PendingTimers` / `FireTimer` |
| Invoked work and spawned children | `StateNodeConfig.Invoke`, driven by `PendingInvocations` / `ResolveInvocation` |
| Snapshot persistence | `Actor.Persist`, `NewActorFromSnapshot` |
| Visualisation | `RenderASCII`, `RenderMermaid`, `RenderGraphJSON` |

## Documentation

The full documentation is at **[fate.arisjirat.com](https://fate.arisjirat.com)**.
It is built from the [`docs/`](./docs) directory in this repository, so the same
pages are readable on GitHub:

| Page | What it covers |
| --- | --- |
| [Concepts](./docs/concepts.md) | What a statechart is, and why the engine computes state while adapters perform effects |
| [Defining machines](./docs/guide/defining-machines.md) | States, transitions, guards, actions, hierarchy, parallel regions, history |
| [Persistence and determinism](./docs/guide/persistence-and-determinism.md) | How an actor serialises to JSON, and the rules that keep replay exact |
| [Effects and adapters](./docs/guide/effects-and-adapters.md) | Delayed transitions and invocations as data, and how an adapter drives them |
| [Temporal](./docs/guide/temporal.md) | Running a machine inside a Temporal workflow |
| [Architecture Decision Records](./docs/adr) | The significant design choices, in the order they were made |

Per-symbol reference lives on
[pkg.go.dev](https://pkg.go.dev/github.com/arisros/fate).

## Tooling

### The `fate` CLI

Renders, inspects, and diffs statecharts from JSON descriptors and snapshots.

```sh
go install github.com/arisros/fate/cmd/fate@latest
```

```
fate render   <descriptor.json|->        ASCII state diagram
fate mermaid  <descriptor.json|->        Mermaid stateDiagram-v2 source
fate graph    <descriptor.json|->        resolved node/edge graph (JSON)
fate snap     <snapshot.json|->          inspect a persisted snapshot
fate diff     <left.json> <right.json>   structural snapshot diff
```

See the [CLI reference](./docs/cli.md) for the input formats.

### fate-studio

[fate-studio](https://github.com/arisros/fate-studio) is a self-hosted chart
viewer and live simulator that renders and drives any fate machine in the
browser, hosted at
[fate-studio.arisjirat.com](https://fate-studio.arisjirat.com). It lives in its
own repository so that the engine keeps no dependencies.

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for the development workflow, and
[SECURITY.md](./SECURITY.md) for reporting vulnerabilities. `make all` runs the
race tests and the linter.

## License

[MIT](./LICENSE) © Aris Kurniawan
