# ADR-0004: Invoke / spawn as effects-as-data

- **Status:** Accepted
- **Date:** 2026-06-05
- **Deciders:** Aris Kurniawan

## Context

Statecharts run external work tied to a state's lifetime: XState's `invoke`
(declarative, started on entry, stopped on exit, with `onDone`/`onError`) and
`spawn` (imperative, dynamic child actors with references and `sendTo`).

The fate core must stay deterministic and side-effect-free (ADR-0003). It cannot
call an activity, start a goroutine, or run a child actor itself. Yet machines
must express "while in this state, run X and react to its result", and this must
work both standalone and inside a Temporal workflow.

## Decision

Model invoked work exactly like delayed transitions (ADR-0003): **as data the
core records and an adapter executes.**

### One effect, opaque `Src`

A state declares invocations; the core treats `Src` as an opaque name and never
interprets it:

```go
type Invocation[Ctx any, Evt any] struct {
    ID      string                  // unique within the state
    Src     string                  // opaque work name (activity, function, or machine)
    Input   func(ctx Ctx) any       // optional, built from context at entry
    OnDone  func(output any) Evt     // map success → an event the machine handles
    OnError func(err error) Evt      // map failure → an event the machine handles
}
```

On entering the state the core records a pending invocation (deterministic
`InvokeID = statePath#invoke#ID`); on exit it disarms it. The core exposes:

```go
func (a *Actor[Ctx, Evt]) PendingInvocations() []PendingInvocation // {ID, Src, Input}
func (a *Actor[Ctx, Evt]) ResolveInvocation(id InvokeID, output any)
func (a *Actor[Ctx, Evt]) RejectInvocation(id InvokeID, err error)
```

An adapter pulls `PendingInvocations`, runs each `Src`, and calls
`ResolveInvocation` / `RejectInvocation`. The core then maps the result to an
event (`OnDone` / `OnError`) and processes it as a normal internal step — but
only if the owning state is still active, giving correct invoke lifecycle
(results for an already-exited state are ignored).

### `spawn` = `invoke` with a machine `Src`

Because `Src` is opaque, "spawn a child statechart" needs no new core concept:
it is an invocation whose `Src` names a machine. The adapter decides what that
means — a Temporal **child workflow**, or a nested in-process `Actor`, or a
goroutine — and feeds the child's completion back via `ResolveInvocation`. The
core stays identical for an activity call and a spawned sub-machine.

### Deferred: dynamic imperative `spawn()` with refs

XState's imperative `spawn()` returning an `ActorRef` for `sendTo`, dynamic
fan-out, and a parent/child "system" is a larger subsystem. The motivating
consumers don't need it — their external work is activity-shaped, mediated by an
adapter. It is explicitly **out of scope for v0.1**; the declarative
invoke-with-machine-`Src` above covers the spawn use cases we have. Revisit if a
concrete need appears.

### Persistence: derive, don't store

Pending timers and pending invocations are a pure function of the active state
configuration. On restore they are **re-derived** by walking the restored
configuration, not serialised. This keeps the snapshot free of un-marshalable
`any` input payloads, and means a restored actor re-presents its pending effects
to the adapter (which, under Temporal, is deduplicated by workflow history; for
standalone restore, re-presenting is the intended behaviour). The snapshot adds
only `output` and `error` for completed/failed actors.

## Consequences

- One uniform effect surface for time, services, and child machines; the core
  never executes anything.
- The Temporal adapter (Phase 1.4) maps `PendingInvocations` to
  `workflow.ExecuteActivity` (or child workflows) and timers to
  `workflow.NewTimer`, all driven inside the workflow coroutine.
- A task-adapter shape from the proof-of-concept (an event mapper plus a
  responder) maps precisely onto this surface, easing any downstream migration.
- Dynamic `spawn()`/refs remain unavailable until a real need justifies the
  actor-system machinery.
