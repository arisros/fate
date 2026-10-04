# ADR-0008: Step records

- **Status:** Accepted
- **Date:** 2026-10-04
- **Deciders:** Aris Jirat Kurniawan

## Context

An actor reports its state after each call through `Actor.Subscribe`, as a
snapshot. A snapshot says where the machine is. It does not say how it got
there: which event, which transition, which states were left and entered.

Three kinds of host need that.

- A host that maps states onto outside work (a task per state, say) has to
  derive "create", "cancel" and "continue" from what was exited and entered.
- An audit trail needs the event and the transition, not two snapshots to diff.
- An effects adapter cannot see a state that is left and entered again in one
  step. Timer and invocation ids are derived from the state path, so the pending
  lists are identical before and after, and the adapter keeps the old timer or
  activity running instead of restarting it.

## Decision

The actor records a `Step` for every step it takes and delivers it to observers
registered with `Actor.SubscribeSteps`.

```go
type Step struct {
    Seq         uint64
    Cause       StepCause // start, event, raise, done, timer, invoke
    Event       string
    Effect      string    // timer or invocation id
    Transitions []StepTransition
    Exited      []string
    Entered     []string
    Value       persist.StateValue
}
```

- One step per unit the engine already processes on its own: `Start`, an event
  passed to `Send`, each event an action raised, each `OnDone` that fired, a
  delivered timer, a delivered invocation outcome. One `Send` that cascades
  yields several steps, in the order they ran.
- `Exited` and `Entered` are in execution order, not sorted, so a host that
  issues commands from them issues the same commands on every run. A state that
  is left and entered again appears in both.
- `Value` is the configuration after the step. Observers run while the actor is
  locked and must not call it; the value is what they would have read.
- A step in which no transition fired is not recorded. Dropped events stay the
  subject of ADR-0006.
- `Seq` counts from 1 and is stored in the persisted snapshot as `seq`, so it
  continues after a restore. The field is additive: `SnapshotVersion` stays 1
  and a snapshot written before it reads as zero.

Steps are not stored. The engine keeps no journal; a host that wants one appends
the steps it receives to its own store.

## Consequences

- Persisted bytes gain a `seq` field once an actor has taken a step. Two actors
  that took the same steps still persist identically.
- The Temporal adapter can restart a timer or activity whose state appears in
  both `Exited` and `Entered`.
- Replay from a step log is possible for a host but is not provided here.
