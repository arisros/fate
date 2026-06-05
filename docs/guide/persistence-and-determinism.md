# Persistence and determinism

## Persisting and restoring

An actor's entire state serialises to JSON and back:

```go
blob, err := actor.Persist()                       // []byte of JSON
restored, err := fate.NewActorFromSnapshot[Ctx, Evt](machine, blob)
```

The snapshot holds the active configuration, the context, the lifecycle status,
the history memory, any internally queued events, and — for a completed or failed
actor — its output or error. The machine is not stored; you pass it back in,
because it is immutable and rebuilt from code.

The guarantee is round-trip transparency: a restored actor, given the same future
events, behaves exactly as the original would have, and re-serialises to the same
bytes. You can persist after every event and resume from the last snapshot with
no observable difference.

### What is not stored, and why

Pending timers and pending invocations are *not* written to the snapshot. They
are a pure function of the active configuration — a state either declares an
`After`/`Invoke` or it doesn't — so on restore they are re-derived by walking the
restored configuration. This keeps the snapshot free of un-serialisable values
(an invocation's input can be any Go value) and means a resumed actor re-presents
its pending effects to whatever adapter is driving it. Under Temporal that
re-presentation is deduplicated by the workflow's own history; for a plain
restore it is the behaviour you want.

### A note on context and event types

`Ctx` and `Evt` are your types, so they must be JSON-serialisable for `Persist`
to work — exported fields, no unsupported types. A sealed-interface `Evt` whose
concrete type can't be recovered from JSON alone (the usual case) needs a custom
codec or a concrete envelope if you intend to persist queued events of that type;
most machines drain their internal queue within a single `Send`, so this rarely
bites.

## Determinism

Determinism means: the same machine driven by the same sequence of operations
always reaches the same active configuration and produces the same persisted
bytes. fate holds to this by construction.

- A `Machine` is immutable after `CreateMachine`. Nothing about it changes at
  runtime.
- The engine performs no I/O, reads no clock, and uses no randomness. Guards and
  actions are pure by contract.
- Every place the engine iterates a map in a way that could affect observable
  output sorts the keys first, so map iteration order never leaks in.
- `Persist` produces byte-identical output for identical actor state.

The library's own test suite asserts this with property-based tests: a machine
driven by a random sequence of operations twice produces identical snapshots, and
a snapshot persisted mid-run and restored continues identically to one that never
persisted.

### The rules you must follow

The engine can't be non-deterministic, but *your* guards and actions can, if you
let them. Two rules keep your side clean:

1. **No clock, randomness, or I/O in guards and actions.** A guard that calls
   `time.Now()` or `rand.Intn` will reach different conclusions on a replay. If a
   decision depends on time or a random value, model it as an event or an
   invocation result that an adapter supplies — then it is recorded and replays
   the same.
2. **Treat context as a value.** `Assign` returns a new context; build it from
   the inputs you were given, not from ambient state.

### Why it matters for durable execution

Durable runtimes such as Temporal rebuild a workflow's state by re-executing its
code and replaying recorded results. That replay must reproduce every decision
exactly, or the runtime detects a non-determinism error and fails the workflow. A
fate machine satisfies this requirement for free: there is nothing inside the
engine that can diverge between the original run and the replay. The only thing
you have to get right is keeping your own guards and actions pure — which the API
already nudges you toward, because a guard is handed only `(Ctx, Evt)` and has
nowhere to reach for the clock. The [Temporal guide](temporal.md) builds on this.
