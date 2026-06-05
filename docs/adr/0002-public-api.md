# ADR-0002: Public API design and package layout

- **Status:** Accepted
- **Date:** 2026-06-04
- **Deciders:** Aris Kurniawan
- **Supersedes:** the internal proof-of-concept's API

## Context

The POC engine works and its topology is proven by ~152 library tests plus five
real-world machines. But its public API was shaped by an internal migration, not
by external consumption: the package was named `statechart` and consumers
aliased it `sc`; a `Setup`-style builder was documented but never implemented;
several symbols are placeholders (`StateIn` always returns false); and delayed
transitions, `invoke`/`spawn`, and the full snapshot shape were deferred.

fate is a fresh, releasable library. This ADR fixes the public API so the rest
of Batch 1 can be built against a stable target.

## Decision

### 1. Package and module names

- Root package is **`fate`**. Callers write `fate.CreateMachine`,
  `fate.NewActor`, `fate.Assign`. The POC's `sc` alias convention is dropped;
  `fate` is already short.
- Module path is **`github.com/arisros/fate`**.
- The Temporal integration is a **separate module**,
  `github.com/arisros/fate/temporal`, package `temporal`. The root module stays
  standard-library-only. (See ADR-0001 for the provenance angle.)

### 2. Two construction paths, one `Machine`

Both of the following produce the same immutable `*Machine[Ctx, Evt]`:

- **Declarative**: `CreateMachine(MachineConfig[Ctx, Evt]{...})` with nested
  `StateNodeConfig` / `TransitionConfig` structs and inline guards/actions.
  This is the POC's proven shape, carried forward.
- **Builder**: `Setup[Ctx, Evt](SetupOptions{...})` registers named guards,
  actions, and actors once, returning a value whose `.CreateMachine(cfg)`
  resolves string references (`Guard: fate.Ref("isHighRisk")`) against the
  registry. This is the XState-parity ergonomic surface and is **net-new** (the
  POC only documented it).

`Setup` is sugar over `CreateMachine`; it must not introduce semantics the
declarative path cannot express.

### 3. Full statechart feature parity in v0.1

The first tagged release implements the complete feature set, including the
pieces the POC deferred:

- Hierarchy (compound), parallel regions, final states, deep/shallow history,
  guards (+ `And`/`Or`/`Not`/`StateIn` — `StateIn` is implemented, not a stub),
  entry/exit and transition actions (`Assign`/`Raise`/`Log`/`EnqueueActions`).
- **Delayed / `after` transitions** via `RaiseAfter`, driven by an injectable
  scheduler so the engine core stays pure; real timers are supplied by the
  caller (the Temporal module wires them to `workflow.NewTimer`).
- **`invoke` / `spawn`** of child actors with `OnDone` / `OnError` and
  deterministic child IDs.

### 4. Snapshot is versioned and forward-compatible

`Actor.Persist()` emits JSON carrying a `version` field. v0.1 ships the full
shape (state value, context, status, history, internal queue, pending timers,
and spawned children). Additive changes keep the same version; a breaking change
increments it and ships a documented migration. A snapshot written by an older
fate must restore under a newer fate. (Full shape specified in a later ADR;
mirrors the POC's ADR-003.)

### 5. Documentation and naming discipline

- **Every exported symbol carries a godoc comment**, enforced by the linter.
  This is a release-blocking requirement, not a guideline.
- Exported names get a deliberate naming pass for an external audience; no
  internal jargon leaks into the public surface.
- The engine exposes introspection (`Machine.Describe`) and rendering
  (`RenderASCII` / `RenderMermaid` / `RenderGraphJSON`) so tooling (the studio,
  `fate`) consumes the same public API external users do.

### 6. Determinism is part of the contract

`Machine` is immutable post-construction and safe to share. The engine performs
no I/O, time, or randomness; all ordering that affects observable output is
sorted. This is what lets the same machine + event sequence persist
byte-identically, and what makes the Temporal bridge possible. (Detailed in a
later determinism ADR; mirrors the POC's ADR-002 / ADR-007.)

## Consequences

- A downstream consumer migrating off the proof-of-concept performs a real port,
  not a copy: `sc.*` becomes `fate.*`, and call sites may adopt `Setup`. This is
  accepted cost for a clean public API.
- Implementing `after`/`invoke`/`spawn` and the full snapshot shape is net-new
  engineering beyond harvesting the proof-of-concept.
- The `Setup` builder must be kept a thin layer to avoid two divergent semantic
  paths.
