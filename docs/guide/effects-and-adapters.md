# Effects and adapters

Most of a machine — states, transitions, guards, actions — is pure computation.
Two features are different: waiting for a delay (`After`) and running work
(`Invoke`). Both want something to *happen* in the outside world, which the
engine refuses to do. fate resolves this by modelling them as **pending effects**:
the engine records the intent as data and exposes it; an **adapter** reads the
intent, performs it, and reports the result back.

This guide explains both effects from both sides — how you declare them on a
machine, and how an adapter drives them. If you only ever run machines inside
Temporal, the [Temporal adapter](temporal.md) does all the driving for you and
you can treat this as background.

## Delayed transitions (`After`)

A state declares transitions keyed by a delay:

```go
"waiting": {
    On:    map[string][]fate.TransitionConfig[Ctx, Evt]{"Cancel": {{Target: "idle"}}},
    After: map[time.Duration][]fate.TransitionConfig[Ctx, Evt]{
        30 * time.Second: {{Target: "expired"}},
    },
},
```

Entering `waiting` *arms* a timer; leaving it (via `Cancel`, say) disarms it. But
the engine never fires the timer itself — it has no clock. Instead:

```go
for _, t := range actor.PendingTimers() {  // {ID, Delay}
    // an adapter decides the delay has elapsed, then:
    actor.FireTimer(t.ID)                   // fires the matching after-transition
}
```

`PendingTimers` is the read side, `FireTimer` the write side. A timer fired for a
state that has since been left is a safe no-op.

## Invocations (`Invoke`)

A state declares work to run while it is active:

```go
"charging": {
    Invoke: []fate.Invocation[Ctx, Evt]{{
        ID:      "charge",
        Src:     "charge-card",                 // an opaque name the adapter understands
        Input:   func(c Ctx) any { return c.Amount },
        OnDone:  func(out any) Evt { return Charged{ref: out} },
        OnError: func(err error) Evt { return ChargeFailed{} },
    }},
    On: map[string][]fate.TransitionConfig[Ctx, Evt]{
        "Charged":      {{Target: "done"}},
        "ChargeFailed": {{Target: "retry"}},
    },
},
```

The engine treats `Src` as an opaque label — it does not know or care what
`"charge-card"` is. Entering `charging` records a pending invocation; leaving it
cancels it. The adapter runs the work and reports the outcome:

```go
for _, inv := range actor.PendingInvocations() {  // {ID, Src, Input}
    result, err := run(inv.Src, inv.Input)         // however the adapter performs work
    if err != nil {
        actor.RejectInvocation(inv.ID, err)        // fires OnError → an event
    } else {
        actor.ResolveInvocation(inv.ID, result)    // fires OnDone → an event
    }
}
```

`OnDone` and `OnError` map the outcome to one of *your* events, which the machine
then handles through its ordinary `On` transitions — so the result re-enters the
machine through the same path as any other event. The engine only delivers the
outcome if the invocation's state is still active, which gives correct
lifecycle: a result that arrives after the state was left is ignored.

### Spawning a child machine is just an invocation

There is no separate "spawn" concept. Because `Src` is opaque, "run a child
statechart" is an invocation whose `Src` happens to name a machine; the adapter
decides what that means — a nested actor, a child workflow, a goroutine — and
feeds the child's completion back through `ResolveInvocation`. The engine is
identical for an HTTP call and a sub-machine.

## What an adapter is

An adapter is the loop that connects pending effects to the real world. It does
three things, repeatedly, until the machine completes:

1. Read `PendingTimers` and `PendingInvocations`.
2. Start the real work for anything newly pending; cancel anything no longer
   pending.
3. When a timer elapses or work finishes, call `FireTimer` /
   `ResolveInvocation` / `RejectInvocation` — which advances the machine and may
   change the pending sets, so the loop reconciles again.

The crucial discipline is that all of this — every call back into the actor —
happens on one goroutine, in a defined order. That is what lets the same machine
run under a real clock, a virtual clock, or Temporal's replay-safe clock without
changing a line of the machine.

### Three adapters, one machine

- **Tests** are the simplest adapter: read `PendingTimers()` / `PendingInvocations()`
  and call `FireTimer` / `ResolveInvocation` by hand. No clock, no I/O, fully
  deterministic — ideal for asserting exactly what a machine does.
- **A real-time adapter** (for an ordinary program) arms an OS timer for each
  pending timer and calls out to a real service for each invocation, then reports
  back from the callback.
- **The Temporal adapter** maps timers to `workflow.NewTimer` and invocations to
  `workflow.ExecuteActivity`, driven inside the workflow coroutine. The result is
  durable: the machine can wait for days, survive worker restarts, and replay
  deterministically. See [Temporal](temporal.md).

The pattern generalises: any new kind of effect is "expose pending intent, accept
a result", and any new runtime is one more adapter.
