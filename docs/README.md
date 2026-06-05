# fate documentation

fate is a statechart engine for Go. These pages explain the ideas behind it and
how to use it well.

If you just want to get something running, the [README quickstart](../README.md)
is the fastest path. Come back here when you want to understand *why* the API is
shaped the way it is — especially before driving machines in a durable runtime
like Temporal.

## Guides

- [Concepts](concepts.md) — what a statechart is, and the one idea that shapes
  the whole library: the engine computes state, adapters perform effects.
- [Defining machines](guide/defining-machines.md) — states, transitions, guards,
  actions, hierarchy, parallel regions, and history.
- [Persistence and determinism](guide/persistence-and-determinism.md) — how an
  actor serialises to JSON, what "deterministic" buys you, and the rules that
  keep it that way.
- [Effects and adapters](guide/effects-and-adapters.md) — delayed transitions and
  invocations as data, and how an adapter drives them.
- [Temporal](guide/temporal.md) — running a machine inside a Temporal workflow.

## Reference

- Full API reference on [pkg.go.dev](https://pkg.go.dev/github.com/arisros/fate).
- [Architecture Decision Records](adr/) — the significant design choices and the
  reasoning behind them, in the order they were made.

## The studio

The visual chart viewer and live simulator is a separate project,
[fate-studio](https://github.com/arisros/fate-studio). It is kept out of this
repository on purpose: the engine has no dependencies, and the studio needs a web
server. The engine ships a small CLI (`cmd/fate`) for rendering and diffing
machines from JSON on the command line.
