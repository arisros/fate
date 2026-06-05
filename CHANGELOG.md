# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

While in `v0.x`, minor versions may contain breaking API changes; those are
flagged explicitly under a **Breaking** heading.

## [Unreleased]

## [0.4.0] - 2026-06-05

The engine is now a focused, dependency-free library: the studio moved to its own
repository, and there is a documentation set covering the model and its use.

### Added
- A documentation set under `docs/` — concepts, and guides for defining machines,
  persistence and determinism, effects and adapters, and Temporal.

### Breaking
- The studio moved to its own repository and module,
  [github.com/arisros/fate-studio](https://github.com/arisros/fate-studio). The
  `fate/studio` package, the demo machines, and the `fate-studio` server are no
  longer part of this module, so the engine no longer pulls in `net/http`.
- The `fate` CLI is now engine-only and file-based: `render` / `mermaid` /
  `graph` / `snap` / `diff` operate on descriptor and snapshot JSON, with no
  built-in demo machines.

## [0.3.0] - 2026-06-05

Studio: timer/invoke visualization, a redesigned welcome page, a Sentry-inspired
visual refresh, and the CLI rename.

### Breaking
- Renamed the CLI binaries to match the product: `scs` → **`fate`**, `scs-web` →
  **`fate-studio`**. The server env var is now `FATE_STUDIO_ADDR` (was
  `SCS_WEB_ADDR`); the Docker image target is `fate-studio`.

### Added
- **Timer / invocation visualization in the simulator.** The live snapshot now
  carries pending delayed (`after`) timers and invocations; a "Pending effects"
  panel lets you fire a timer or resolve/reject an invocation (with JSON output),
  driving the machine exactly as an adapter would. New endpoints `/sim/{m}/timer`
  and `/sim/{m}/invoke`; `LiveInstance` gains `PendingTimers`/`FireTimer`/
  `PendingInvocations`/`ResolveInvocation`/`RejectInvocation`.
- New `timeout` (after-timer) and `fetch` (invocation) demo machines.
- A redesigned **welcome page** (hero, machine-card gallery) and a Sentry-inspired
  visual refresh (violet-midnight ink, electric-lime keyword accent, button-cap
  styling) — self-contained, no external fonts or build step.
- A `counter` demo machine whose transitions mutate context (INC/DEC/RESET), so
  the studio's context panel shows `{"count": N}` updating live.

### Fixed
- Studio context panel rendered `[object Object]`: the SSE snapshot's `context`
  arrives already JSON-parsed, but the client re-`JSON.parse`d it. Now handled
  as a value.
- Studio nodes clipped their bottom action rows: locked header/row heights to the
  JS box math, added nowrap + ellipsis, enlarged the node box.

## [0.2.0] - 2026-06-05

The studio release: a viewer/simulator and the `fate` / `fate-studio` binaries.

### Added
- `fate/studio` — an embeddable, dependency-free HTTP statechart studio: a chart
  viewer and live, Server-Sent-Events simulator for any fate machine. Endpoints
  for the machine list, static diagram, JSON descriptor, resolved canvas graph,
  per-state inspection, and a per-browser-session simulator (send, undo, reset,
  import/export, timeline). Carries forward the proof-of-concept's resilience
  fixes (elk fallback layout, NaN guards, content-versioned asset cache-busting).
- `fate` CLI (list / view / describe / snap / diff) and `fate-studio` server, serving
  a set of generic demo machines (traffic light, media player, build pipeline,
  deep-history document editor).
- A multi-stage, distroless `Dockerfile` for `fate-studio`, and a GoReleaser config
  building the `fate` / `fate-studio` binaries on release (validated in CI).
- Studio endpoint coverage via `httptest`; the studio package ships in the root
  module and keeps it standard-library only.

## [0.1.0] - 2026-06-05

First public release: the statechart engine and its Temporal adapter.

### Added
- Project bootstrap: zero-dependency engine module (`github.com/arisros/fate`)
  and separate Temporal integration module (`github.com/arisros/fate/temporal`).
- ADR-0001 (provenance & license) and ADR-0002 (public API design).
- Core engine harvested from the proof-of-concept: hierarchy, parallel regions,
  deep/shallow history, guards, actions, final states, JSON persist/restore.
- `Setup` builder for registering named guards and actions (XState-style).
- `Cond` / `StateIn` / `InState` — structural conditions over the active state
  configuration, complementary to data `Guard`s.
- Delayed (`after`) transitions with a clock-agnostic core: the engine records
  pending timers as data and exposes `Actor.PendingTimers` / `Actor.FireTimer`;
  an adapter (Temporal, or an opt-in real-time helper) owns all timing. The core
  never reads the wall clock or starts a goroutine.
- `invoke` as effects-as-data: a state's `Invoke` records pending work the core
  never runs; an adapter pulls `Actor.PendingInvocations` and reports outcomes
  via `Actor.ResolveInvocation` / `Actor.RejectInvocation`. A spawned child
  machine is just an invocation whose `Src` names a machine (ADR-0004).
- Final-state `Output` captured into the snapshot's `output`; `error` persisted.
- Snapshot restore re-derives pending timers and invocations from the active
  configuration (not stored), keeping snapshots free of un-marshalable payloads.
- Property-based tests (seeded `math/rand`, no third-party dependency):
  determinism (same ops → byte-identical snapshot), persist/restore
  transparency, and persist stability. Coverage gate at ≥85% (currently ~87%).
- Tooling & packaging: GitHub Actions CI (Go matrix × root and `temporal/`
  modules — `go vet`, `go test -race`, golangci-lint, a coverage gate, and a
  zero-dependency assertion on the engine), a tag-driven release workflow,
  `golangci-lint` config enforcing godoc on every exported symbol, `Makefile`,
  `CODEOWNERS`, a PR template, runnable `examples/` (quickstart, trafficlight,
  realtime-timer), and testable `Example` functions.

- Temporal integration module `github.com/arisros/fate/temporal`: a
  `WorkflowActor` that hosts a `fate.Actor` inside a Temporal workflow and drives
  its pending effects — `after` timers → `workflow.NewTimer`, invocations →
  `workflow.ExecuteActivity`, events → a signal channel — all inside the workflow
  coroutine via a deterministic selector loop. Supports continue-as-new via
  `Persist` / `NewWorkflowActorFromSnapshot`. Validated end-to-end against
  Temporal's test environment (ADR-0005). The root engine module stays
  zero-dependency.

### Fixed
- `Start` now enters the full initial configuration of parallel states: entry
  actions, delayed transitions, and invocations declared on the initial state of
  each parallel region are no longer skipped at startup. (Found by the
  persist/restore property test: a restored actor re-derived a region's timer
  that a freshly-started actor had never armed.)
