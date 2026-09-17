# The fate CLI

`cmd/fate` is a small command-line tool that renders, inspects, and diffs
statecharts. It works on JSON files rather than on Go code, so it is not tied to
any particular set of machines.

```sh
go install github.com/arisros/fate/cmd/fate@latest
```

Released binaries for Linux and macOS are also attached to each
[GitHub release](https://github.com/arisros/fate/releases).

## Input formats

The tool reads two kinds of JSON:

- A **machine descriptor** — the JSON of `fate.MachineDescriptor`, as produced
  by `Machine.Describe` and marshalled, or served by a studio at
  `/m/{name}/describe`.
- A **persisted snapshot** — the JSON written by `Actor.Persist`.

A path of `-` reads from standard input, so the tool composes with anything that
can print a descriptor.

## Commands

| Command | Input | Output |
| --- | --- | --- |
| `fate render <descriptor.json\|->` | descriptor | ASCII state diagram |
| `fate mermaid <descriptor.json\|->` | descriptor | Mermaid `stateDiagram-v2` source |
| `fate graph <descriptor.json\|->` | descriptor | resolved node/edge graph as JSON |
| `fate snap <snapshot.json\|->` | snapshot | a readable view of a persisted snapshot |
| `fate diff <left.json> <right.json>` | two snapshots | structural diff |

## Examples

Render a descriptor produced by your own program:

```sh
go run ./cmd/describe-my-machine | fate render -
```

Generate Mermaid source and embed it in a document:

```sh
fate mermaid machine.json > docs/machine.mmd
```

Compare two persisted snapshots, for example before and after an event:

```sh
fate diff before.json after.json
```

## Related

- [Effects and adapters](./guide/effects-and-adapters.md) — what a snapshot
  contains beyond the active configuration.
- [Persistence and determinism](./guide/persistence-and-determinism.md) — the
  snapshot format and its guarantees.
- [fate-studio](https://fate-studio.arisjirat.com) — the same rendering, in the
  browser, with a live simulator.
