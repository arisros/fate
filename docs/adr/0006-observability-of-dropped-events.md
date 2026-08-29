# ADR-0006: Observability of dropped events and stale effects

- **Status:** Accepted
- **Date:** 2026-08-30
- **Deciders:** Aris Kurniawan

## Context

A statechart drops an event that no active state handles. This is correct: an
event nothing cares about is not an error, and XState v5 behaves the same way.
`Actor.Send` therefore returned `nil` whether a transition fired or not.

The same silence extended to the effect surface. `FireTimer`,
`ResolveInvocation` and `RejectInvocation` returned nothing at all, so an id
that was unknown, already settled, or owned by a state the machine had since
left was indistinguishable from one that was delivered.

That is a problem for two kinds of caller.

An **adapter** reconciling effects against the outside world needs to know
whether a result landed. A Temporal workflow can complete an activity whose
state the machine left while the activity was in flight; a real-time adapter can
fire an OS timer a moment after an exit cancelled it. Both are ordinary races
that the engine already resolves correctly by ignoring the late outcome, but the
adapter cannot see which happened, so it cannot log, retry, or report.

A **caller migrating from a stricter engine** needs to know whether an event was
acted on. The motivating case is LORA's `fp.StateMachine`, whose `Transition`
returns `ErrFSMIllegalTransition` from a state that does not declare the
transition. That error propagates out of a Temporal task handler and fails the
task. Under fate the same submission would be silently ignored and the task
would report success, which is a behavioural regression rather than a port.

Three options were considered.

1. **Make `Send` return a sentinel error** when no transition fires. This gives
   the strict caller exactly what it wants, but it breaks every fire-and-forget
   caller: they must now distinguish "no transition" from a real failure, and the
   common, correct case of an ignored event starts looking like an error.
2. **Have the caller diff snapshots** around each `Send`. Possible today, but it
   is verbose, allocates, and answers the wrong question: a transition can fire
   with no target and no context change, which is handled but produces an
   identical snapshot.
3. **Add a separate query.** Keeps `Send` as it is and gives the strict caller a
   precise answer.

## Decision

Keep `Send`'s semantics and add the observability surface alongside it.

### `Actor.Can`

```go
func (a *Actor[Ctx, Evt]) Can(evt Evt) bool
```

`Can` reports whether at least one transition selects for the event in the
current configuration, with guards evaluated against the current context. It
does not mutate the actor.

The answer is exact rather than an approximation, because guards are pure by
contract (ADR-0003) and so can be evaluated twice without consequence. It
resolves through the same `selectTransitions` path `Send` uses, so the two
cannot drift.

Two boundaries are part of the contract. An actor that is not running reports
`false` for every event, since a stopped or completed actor handles none. And
`Can` answers about transition *selection*: a selected transition whose target
fails to resolve reports `true` while changing no state, which is the same
configuration error `Send` absorbs.

The strict caller composes the two:

```go
if !actor.Can(evt) {
    return fmt.Errorf("%w: %s in %s", ErrUnhandledEvent, name, snap.Value.Path())
}
_ = actor.Send(ctx, evt)
```

### Effect methods report acceptance

`FireTimer`, `ResolveInvocation` and `RejectInvocation` now return `bool`.
True means the effect was **accepted**: the id was armed and its owning state
still active. False means it was unknown, already settled, or belongs to a state
the machine has left.

Acceptance is deliberately not the same as delivery. An accepted invocation with
no `OnDone` or `OnError` mapper reports true and produces no event, because the
invocation was still consumed. An adapter cares whether its report was taken,
not whether the machine chose to model the outcome.

Adding a result to a function that returned nothing is source-compatible: Go
permits discarding a result in a call statement, so existing call sites are
unaffected.

## Consequences

- `Send` keeps a single meaning, and an ignored event stays an ordinary outcome
  rather than an error the caller must filter.
- Callers that need strictness pay one extra guard evaluation per event. For the
  machines this is aimed at, guards are cheap comparisons over context.
- The engine gains no new state and no new configuration. `Can` is derived
  entirely from the existing selection algorithm.
- Adapters can distinguish a delivered effect from a late one, which makes a
  race that was previously invisible loggable.
- `Can` and `Send` must keep using the same selection path. A future change that
  gives `Send` its own resolution logic would let the two disagree, which is the
  one way this decision can rot.
