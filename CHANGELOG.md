# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

While in `v0.x`, minor versions may contain breaking API changes; those are
flagged explicitly under a **Breaking** heading.

## [Unreleased]

## [0.5.0] - 2026-08-30

Root package reduced to the core engine API. Visualization and diff are now
opt-in sub-packages so adopters who don't need them avoid the code surface.
Exit semantics under parallel regions are corrected, and guards and actions
now carry names into the descriptor.

### Added

- **`fate/httphandler` sub-package** — exposes an [Actor] as an HTTP simulator
  API (SSE `/stream`, plus `/send`, `/timer`, `/invoke`, `/reset`, `/undo`,
  `/import`, `/export`, `/timeline`), wire-compatible with fate-studio's
  `/sim/{name}/*` endpoints. One actor per browser session. It has no
  authentication and is intended for development, not public exposure.

- **`Actor.Can(evt) bool`** — reports whether an event would be handled by the
  current configuration, guards included, without mutating the actor. `Send`
  drops an unhandled event on purpose; `Can` is the companion for callers that
  treat one as an error.

- **`Named(name, action)`** — labels an action so it appears under that name in
  a `MachineDescriptor` and in every rendered diagram.

- **`TransitionConfig.GuardName`** — labels a guard the same way. Optional; an
  unnamed guard still renders as `""`.

### Changed

- **Guards and actions are named in `Machine.Describe`.** The built-in actions
  report their kind (`assign`, `log`, `enqueue`, `raise:CANCEL`), and
  `Setup.Action` labels what it hands out. Guards are named by the new
  `TransitionConfig.GuardName`; a func value carries no identity a descriptor
  could recover, so a guard is unnamed unless it is declared. Descriptor output
  therefore changes for any machine that already used actions: names that were
  `""` now carry a value.

- **`Machine.findState` visits children alphabetically**, so `IsKnownState`,
  `IsTerminal` and `IsLegalTransition` cannot disagree with themselves between
  runs when two states at the same depth share a name.

### Fixed

- **Exit sets under parallel regions.** `computeExitSet` walked up from the
  alphabetically first active leaf rather than from the transition's domain, so
  a transition fired inside one region ran another region's `Exit` actions and
  disarmed its timers and invocations, and a transition leaving the parallel
  node exited only one region. The exit set is now every active node below the
  transition's domain, and no result depends on which region sorts first. The
  state value was always correct and is unchanged.

- **A region could exit while still being reported active.** When the LCCA was
  the parallel node itself (a cross-region target, a handler on the parallel
  node targeting its own descendant, an external self-transition on a region, or
  an internal transition declared on the parallel node), states were exited that
  `commitValue` then carried over as active. Their `Exit` actions ran and their
  timers and invocations were disarmed while the snapshot still reported them
  running, leaving an armed timer that could never fire. The exit set now stops
  at the region `commitValue` actually replaces.

- **A region could be re-entered with nothing armed.** `computeEntrySet` had no
  parallel case: it built the single target-to-domain chain, so a transition
  entering a parallel node added that node alone and entered none of its
  regions. Their `Entry` actions never ran and their `After` timers and
  `Invoke` calls were never armed, while `commitValue` reported every region
  active. Leaving a parallel node and returning to it therefore produced an
  active state whose delayed transition could no longer fire. This was masked
  before the exit set was corrected, because the old exit set never disarmed
  those effects on the way out. The entry set now enters every region of a
  parallel node on the chain, keyed to the same domain the exit set uses, so a
  transition that stays inside the node still leaves its siblings untouched.

- **Shallow history under parallel regions.** `recordHistoryLocked` searched
  only the first active leaf, so a region that did not sort first recorded no
  history and fell back to its default on re-entry.

### Breaking

- **`fate/render` sub-package** — the three renderers and their types move out
  of the root package:
  - `fate.RenderASCII` → `render.ASCII`; `fate.RenderOptions` → `render.Options`
  - `fate.RenderTransitions` → `render.Transitions`
  - `fate.RenderMermaid` → `render.Mermaid`; `fate.MermaidOptions` →
    `render.MermaidOptions` (same name, new package)
  - `fate.RenderGraphJSON` → `render.GraphJSON`
  - `fate.Graph`, `fate.GraphNode`, `fate.GraphEdge` → same names in `render`
  - Import: `github.com/arisros/fate/render`

- **`fate/diff` sub-package** — snapshot diffing moves out of the root package:
  - `fate.DiffSnapshots` → `diff.Snapshots`
  - `fate.DiffKind` → `diff.Kind`; constants `DiffKind*` → `Kind*`
  - `fate.DiffEntry` → `diff.Entry`; `fate.SnapshotDiff` → `diff.Result`
  - Import: `github.com/arisros/fate/diff`

- **`FireTimer`, `ResolveInvocation` and `RejectInvocation` return `bool`**,
  reporting whether the effect was accepted: the id was armed and its owning
  state still active. Previously a stale or unknown id was an undetectable
  no-op. Call statements are unaffected, but a signature change is not source
  compatible everywhere: a method value (`var fire func(fate.TimerID) =
  a.FireTimer`, the adapter shape ADR-0003 encourages) and interface
  satisfaction (`interface{ FireTimer(fate.TimerID) }`, including test doubles)
  both stop compiling.

- **`Actor.PersistDeterministic` removed.** It delegated to `Persist` and did
  nothing else, while its documentation implied `Persist` carried a weaker
  guarantee. `Persist` is the deterministic surface and always was: the
  property tests that asserted byte-stability now assert it about `Persist`
  directly. Callers should use `Persist`.

### Migrating from 0.4.0

`v0.5.0` is the first tag that carries the sub-package extraction, so every
consumer still on `v0.4.0` changes imports on upgrade. `go get -u` surfaces this
as compile errors, not as a runtime change: nothing moved silently, and no
behaviour depends on which import path a symbol came from.

Add the imports you need,

```go
import (
    "github.com/arisros/fate"
    "github.com/arisros/fate/render"  // if you render
    "github.com/arisros/fate/diff"    // if you diff snapshots
)
```

then apply the renames:

| `v0.4.0` | `v0.5.0` |
|---|---|
| `fate.RenderASCII` | `render.ASCII` |
| `fate.RenderOptions` | `render.Options` |
| `fate.RenderTransitions` | `render.Transitions` |
| `fate.RenderMermaid` | `render.Mermaid` |
| `fate.MermaidOptions` | `render.MermaidOptions` |
| `fate.RenderGraphJSON` | `render.GraphJSON` |
| `fate.Graph` / `fate.GraphNode` / `fate.GraphEdge` | `render.Graph` / `render.GraphNode` / `render.GraphEdge` |
| `fate.DiffSnapshots` | `diff.Snapshots` |
| `fate.SnapshotDiff` | `diff.Result` |
| `fate.DiffEntry` | `diff.Entry` |
| `fate.DiffKind` | `diff.Kind` |
| `fate.DiffKindStateValue` etc. | `diff.KindStateValue` etc. |
| `actor.PersistDeterministic()` | `actor.Persist()` |

**What did not move.** The engine API is untouched: `Machine`, `Actor`,
`Snapshot`, `ActorStatus`, `Guard`, `Cond`, `Action`, `Setup`,
`MachineDescriptor`, `LoadDescriptor`, the node and history constants, and the
error values all stay in `github.com/arisros/fate`. In particular `Snapshot` and
`ActorStatus` remain in the root package despite the new `fate/snapshot`
sub-package, which is unrelated: it is new API for writing descriptor JSON to
disk (`snapshot.Emit`, `snapshot.EmitDescriptor`), not a relocation of anything
that existed in `v0.4.0`.

Known affected consumer: fate-studio vendors `v0.4.0` and will need the renames
above.

### Known limitations

- **Parallel regions are not fully SCXML.** A transition whose domain is a
  parallel node relocates only the target's region; the others keep running
  rather than exiting and re-entering. Exit and entry agree on that narrower
  domain, so the configuration stays consistent, but SCXML would exit and
  re-enter every region. Adopting it means widening both halves together.
- **Parallel `OnDone` is absent.** A parallel node does not complete when all
  its regions reach a final state, and `settleFinalLocked` still reads only the
  first active leaf, so a region that does not sort first never fires its
  compound's `OnDone`.
- **Exit order is depth-major**, so states from different regions interleave by
  depth. SCXML uses reverse document order.

### Unchanged

All core engine imports (`github.com/arisros/fate`) are unaffected: `Machine`,
`Actor`, `Snapshot`, `Guard`, `Cond`, `Action`, `Setup`, `MachineDescriptor`,
`LoadDescriptor`, etc. remain in the root package.

## [0.4.1] - 2026-06-06

### Changed
- **File merges:** `version.go` absorbed into `doc.go`; `snapshot.go` absorbed
  into `persist.go`; `guards.go` and `cond.go` merged into `guard.go`.
- **File renames:** `ascii_graph.go` → `render_ascii.go`, `mermaid.go` →
  `render_mermaid.go`, `graph.go` → `render_graph.go` (render-cluster naming);
  `scxml.go` → `algorithm.go` (content is SCXML transition algorithms, not
  SCXML parsing); `actions.go` → `action.go` (Go singular-noun convention);
  `after.go` absorbed into and renamed to `timer.go` (timer types + implementation
  unified). Test files track source renames.

No public API changes. No import-path changes.

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
