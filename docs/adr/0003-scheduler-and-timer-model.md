# ADR-0003: Clock-agnostic core and the adapter timer model

- **Status:** Accepted
- **Date:** 2026-06-05
- **Deciders:** Aris Kurniawan

## Context

Statecharts support delayed (`after`) transitions: on entering a state a timer
is armed; if the state is still active when the delay elapses a transition
fires; if the state is exited first the timer is cancelled.

The fate engine must remain **deterministic** and **dependency-free**, and the
same machine must run in several very different time environments:

1. An ordinary Go program — real wall-clock delays.
2. Unit tests — exact, instantaneous, deterministic control over time.
3. A Temporal workflow — delays must be **durable and replay-deterministic**,
   using `workflow.NewTimer`, whose future is awaited inside the single workflow
   coroutine; firing a timer from an arbitrary goroutine is not permitted.

The guiding model is the same one XState and Temporal use: the **statechart
logic is pure, and a host/adapter drives it**. XState's actor logic is a pure
transition function that an interpreter runs; Temporal makes durable execution
work by keeping workflow code deterministic and pushing every side effect out to
the SDK. fate follows suit: the engine is the pure logic; timing is a side
effect owned by an adapter (a "fate adapter", analogous to how one writes a
Temporal adapter over XState).

An earlier iteration embedded a `Scheduler` interface (with a default wall-clock
implementation) inside the actor. That put a clock — and goroutines — into the
core, which is exactly what an agnostic engine must avoid, and it did not fit
Temporal's coroutine model.

## Decision

**The core is clock-agnostic. It never reads the wall clock, sleeps, or starts a
goroutine for a timer.** It treats a delayed transition as *data*: on entry it
records a pending timer; on exit it disarms it. It exposes that intent and
accepts firings back, and nothing more:

```go
// Read half: what timers are armed right now (deterministic order, by ID).
func (a *Actor[Ctx, Evt]) PendingTimers() []PendingTimer // {ID TimerID; Delay time.Duration}

// Write half: deliver an elapsed delay back into the machine.
func (a *Actor[Ctx, Evt]) FireTimer(id TimerID)
```

`TimerID` is derived deterministically from the state path, delay, and index, so
the same logical timer has the same ID across runs and across persistence.

**Driving timers is entirely the adapter's job.** Each environment supplies its
own adapter around the actor:

- **Temporal adapter** (`github.com/arisros/fate/temporal`, Phase 1.4): for each
  `PendingTimer`, start a `workflow.NewTimer(ctx, delay)`; when its future fires
  inside the workflow coroutine, call `FireTimer(id)`. Durable and
  replay-deterministic, no goroutines, no mutex contention.
- **In-memory / real-time adapter** (a small optional helper for standalone Go
  use): watch the actor's pending timers and arm `time.AfterFunc`, calling
  `FireTimer` when each elapses. Lives outside the core's hot path; the core has
  no dependency on it.
- **Tests**: drive `PendingTimers()` / `FireTimer()` by hand — deterministic,
  no real sleeping, no flakiness.

The same pull-of-intent pattern will generalise to `invoke`/`spawn` (Phase 1.2):
the core will expose *pending invocations / spawned children as data*, and the
adapter maps them to its world (a Temporal adapter → activities / child
workflows) and feeds results back as events. This keeps every side effect —
time, async work, child actors — out of the deterministic core.

## Consequences

- The engine core stays deterministic and standard-library-only; there is no
  clock or goroutine anywhere in it.
- No timer fires unless an adapter drives it. A bare `Actor` with `after`
  transitions and no adapter will simply hold its pending timers — intended, and
  documented on `StateNodeConfig.After`. Standalone ergonomics are provided by an
  opt-in real-time adapter rather than by baking a clock into the core.
- Phase 1.4 needs no special engine hooks: the Temporal integration is purely an
  adapter over `PendingTimers` / `FireTimer` (and, later, the analogous
  invoke/spawn intent API).
- Persisting **remaining** delay (vs configured delay) across a restart is part
  of the snapshot-shape work; until then `PendingTimer.Delay` is the configured
  delay and the adapter owns elapsed-time accounting.
