# ADR-0005: Temporal integration boundary

- **Status:** Accepted
- **Date:** 2026-06-05
- **Deciders:** Aris Kurniawan

## Context

fate must run inside Temporal workflows durably and deterministically, but the
engine itself must stay dependency-free (ADR-0001/0003/0004). This ADR fixes the
boundary between the two.

## Decision

### Separate module

The Temporal integration is the module `github.com/arisros/fate/temporal`, with
its own `go.mod`. The root engine module imports nothing outside the standard
library; only adopters who host a machine in a workflow pull in the Temporal
SDK. During development the module resolves the engine via a `replace`
directive; a release pins a published engine tag.

### A driver, not a hook

The adapter is a `WorkflowActor[Ctx, Evt]` that *hosts* a plain `fate.Actor` and
drives it from the workflow goroutine. It adds no special interface to the
engine — it consumes the same effects-as-data pull API any adapter uses:

- `PendingTimers` → `workflow.NewTimer` (cancellable child context); on fire,
  `FireTimer`.
- `PendingInvocations` → `workflow.ExecuteActivity` (`Src` = activity name,
  `Input` = argument); on completion, `ResolveInvocation` / `RejectInvocation`.
- external events → an optional signal channel; on receive, `Send`.

`Run` loops: reconcile in-flight Temporal timers/activities against the actor's
current pending sets (start newly-armed, cancel no-longer-pending), then
`Select` on all in-flight futures plus the signal channel. Each fired branch
calls back into the actor on the workflow goroutine, which may change the pending
sets, and the next iteration reconciles again. The loop ends when the actor
reaches a top-level final state.

### Determinism

- All effects are created, cancelled, and added to the selector in sorted-ID
  order (fate's pending lists are already sorted), so replay reproduces the same
  command order even when several futures are ready at once.
- Every actor call happens on the workflow goroutine; the actor is never shared
  across goroutines, so its internal mutex never contends and contributes no
  nondeterminism. The actor reads no wall clock and performs no I/O.
- `continue-as-new` is supported via `Persist` / `NewWorkflowActorFromSnapshot`;
  a resumed actor re-derives its pending effects from the restored configuration.

### Serialization constraints

- An invocation's activity input (`Input`) and result are carried by Temporal's
  data converter. Results are decoded into `interface{}`, so an `OnDone` mapper
  must handle the converter's decoded shape (e.g. `float64` for numbers,
  `map[string]interface{}` for objects), exactly as any Temporal activity caller
  must.
- Events delivered via signals must be Temporal-serializable; a sealed-interface
  `Evt` needs a custom data converter or a concrete envelope, the same
  constraint `Persist` places on `Ctx`/`Evt`.

## Consequences

- The engine never learns about Temporal; the integration is one well-contained
  module, validated end-to-end against Temporal's in-memory test environment
  (timers, activities, signals).
- A task-adapter shape from the proof-of-concept (event mapper plus responder)
  is the same shape, so a downstream migration maps cleanly onto this
  `WorkflowActor`.
- The reconcile-loop pattern generalises: any future effect type (e.g. child
  workflows for spawned machines) is added by mapping a new pending list to a
  Temporal primitive, with no engine change.
