# Concepts

## What a statechart is

A finite-state machine has one active state at a time and a flat list of
transitions. That works until it doesn't: real systems accumulate modes,
sub-modes, and concurrent activities, and a flat machine answers with an
explosion of states and duplicated transitions.

A statechart, as introduced by David Harel, adds three things that keep the
model honest as it grows:

- **Hierarchy.** A state can contain sub-states. A transition on the parent
  applies no matter which child is active, so you write it once.
- **Parallel regions.** A state can hold several independent regions that are all
  active at the same time, each with its own current state. This replaces the
  cross-product of states you would otherwise need.
- **History.** Re-entering a compound state can return to the sub-state it was in
  when it was last left, rather than starting over.

fate implements all three, plus guards, entry/exit and transition actions,
final states, delayed transitions, and invoked work. It is a statechart engine —
the name is not an acronym.

## The active configuration

Because of hierarchy and parallelism, "the current state" is not a single value.
At any moment a machine has an *active configuration*: the set of states that are
active together. fate represents it as a `StateValue` — a tree that is a bare
string for an atomic state, or a map of region name to sub-value for a compound
or parallel state. Its `Path()` renders the leaves as dotted paths, joined by
`|` across parallel regions (for example `review.bpkb.checking | review.head_vd`).

## Machine and Actor

The library separates the *definition* of a machine from a *running instance* of
it:

- A `Machine` is immutable and validated once at construction. It is safe to
  share across goroutines and to back many running instances. It holds no
  runtime state.
- An `Actor` is one running instance. It holds the active configuration and the
  context, processes events, and serialises to and from JSON.

This split is what makes the engine cheap to embed: build the machine once at
start-up, spin up an actor per workflow or per request.

## The one idea: the engine computes, adapters perform

This is the principle that shapes the entire API, and the one worth internalising
before anything else.

**The engine never performs a side effect.** It does not read the clock, sleep,
start a goroutine, make a network call, or touch the filesystem. Given a machine
and a sequence of events, it computes the next state — and nothing else.

That sounds limiting, because real machines clearly *do* things: they wait for a
timeout, call a service, spawn a child. fate handles this by treating those as
**data, not actions**. When a state wants to wait, the engine records a *pending
timer*. When a state wants to call a service, it records a *pending invocation*.
It exposes that intent and waits to be told the outcome:

```
state enters "loading"
    │
    ├─ engine: record pending invocation {id, src: "charge-card", input}
    │
adapter reads PendingInvocations(), runs the work, and reports back:
    │
    └─ engine: ResolveInvocation(id, result)  →  fires the OnDone transition
```

The component that reads those pending effects and performs them is an
**adapter**. Different adapters perform the same intent in different worlds:

- a real-time adapter arms an OS timer and calls an HTTP endpoint;
- a Temporal adapter maps the timer to `workflow.NewTimer` and the invocation to
  `workflow.ExecuteActivity`, so the work is durable and survives restarts;
- a test drives both by hand, with no clock and no I/O at all.

The engine is identical in every case. This is why the same machine can run in a
unit test in microseconds and in a Temporal workflow for three weeks, and produce
the same transitions. The [effects and adapters](guide/effects-and-adapters.md)
guide makes this concrete.

## Why determinism matters here

Keeping side effects out of the engine has a second payoff: determinism. The same
machine and the same events always produce the same active configuration and the
same serialised snapshot, byte for byte. Nothing inside the engine can vary
between runs, because nothing inside it reads the outside world.

Durable execution engines such as Temporal *require* this: they re-run workflow
code to rebuild state after a failure, and that re-run must reproduce the original
decisions exactly. A machine that called `time.Now()` in a guard would break on
replay. fate cannot, because the guard has no way to call it. See
[persistence and determinism](guide/persistence-and-determinism.md).
