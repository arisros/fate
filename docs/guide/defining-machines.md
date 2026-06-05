# Defining machines

A machine is described with plain Go structs and built with `CreateMachine`,
which validates it and returns an immutable `*Machine`. The two type parameters
are your context (`Ctx`, the data the machine accumulates) and your event type
(`Evt`, usually a sealed interface).

```go
type Ctx struct{ Attempts int }

type Evt interface{ isEvt() }
type Submit struct{}
type Retry  struct{}
func (Submit) isEvt() {}
func (Retry)  isEvt() {}

m, err := fate.CreateMachine(fate.MachineConfig[Ctx, Evt]{
    ID:      "login",
    Initial: "entering",
    States: map[string]fate.StateNodeConfig[Ctx, Evt]{
        "entering": {On: map[string][]fate.TransitionConfig[Ctx, Evt]{
            "Submit": {{Target: "checking"}},
        }},
        "checking": { /* ... */ },
    },
})
```

`CreateMachine` returns an error (wrapping a sentinel such as `ErrUnknownTarget`
or `ErrNoInitial`) for anything malformed — an initial state that doesn't exist,
a transition whose target can't be resolved, a final state with children. A
machine that builds is structurally sound.

## Events and their names

An event is matched to transitions by its *name*. fate derives the name in this
order:

1. if the event is a string, the string itself;
2. if it has an `EventName() string` method, that;
3. otherwise the concrete type name via reflection.

For typed events, give them an `EventName` so matching never pays for reflection
and the wire name is explicit:

```go
func (Submit) EventName() string { return "Submit" }
```

The keys in an `On` map are these names.

## Transitions

A transition names a `Target` and may carry a `Guard`, a `Cond`, and `Actions`.
The value in `On` is a *slice* of candidates, tried in order; the first whose
guard and condition pass is taken:

```go
"Decision": {
    {Target: "approved", Guard: scoreAtLeast(700)},
    {Target: "review",   Guard: scoreAtLeast(600)},
    {Target: "declined"},  // fallback: no guard
},
```

An empty `Target` makes the transition *internal*: its actions run but the active
state does not change. `Internal: true` on a transition with a target that is a
descendant suppresses exit and re-entry of the source.

## Guards and conditions

A `Guard` is a pure predicate over context and event — `func(Ctx, Evt) bool`.
Compose them with `And`, `Or`, `Not`:

```go
Guard: fate.And(isVerified, fate.Not(isHighRisk)),
```

A guard sees only data. To branch on *which states are currently active* — the
equivalent of XState's `stateIn` — use a `Cond` instead, built with `StateIn` /
`InState` and composed with `CondAllOf`, `CondAnyOf`, `CondNot`:

```go
{Target: "q", Cond: fate.StateIn("review.signed")},
```

When both a `Guard` and a `Cond` are present, the transition fires only if both
pass. Keep guards pure: no clock, no randomness, no I/O. A guard that isn't pure
breaks determinism (see [persistence and determinism](persistence-and-determinism.md)).

## Actions

Actions run as part of a transition or on entering/leaving a state. They are also
pure — they may change context, raise an internal event, or log, but nothing
else.

- `Assign(func(Ctx, Evt) Ctx)` returns a new context. With a struct context the
  idiom is `func(c Ctx, _ Evt) Ctx { c.Attempts++; return c }` — Go's value
  semantics give you a copy to mutate.
- `Raise(evt)` enqueues an internal event, processed before `Send` returns.
- `Log(msg)` emits to the actor's logger.
- `EnqueueActions(func(*Enqueuer))` batches several assigns, raises, and logs.

Entry and exit actions live on the state, transition actions on the transition.
The order when a transition fires is: exit actions (deepest state first) →
transition actions → entry actions (outermost state first).

## The Setup builder

For larger machines, registering guards and actions by name keeps the config
readable and lets transitions share implementations. `NewSetup` gives you that,
in the spirit of XState's `setup`:

```go
s := fate.NewSetup[Ctx, Evt]().
    WithGuard("isHighRisk", func(c Ctx, _ Evt) bool { return c.Risk == "HIGH" }).
    WithAction("clearForm", fate.Assign(func(c Ctx, _ Evt) Ctx { c.Form = nil; return c }))

m, err := s.CreateMachine(fate.MachineConfig[Ctx, Evt]{ /* ... uses s.Guard("isHighRisk"), s.Action("clearForm") ... */ })
```

Referencing a name that was never registered is reported as an error from
`CreateMachine`, so a typo fails at build time rather than silently doing nothing.

## Hierarchy

A state becomes compound by giving it `States` and an `Initial` child. Transitions
declared on the parent apply from any descendant:

```go
"review": {
    Initial: "pending",
    States: map[string]fate.StateNodeConfig[Ctx, Evt]{
        "pending":  {On: ...},
        "approved": {Type: fate.NodeFinal},
    },
    On: map[string][]fate.TransitionConfig[Ctx, Evt]{
        "Cancel": {{Target: "cancelled"}}, // applies whatever review sub-state is active
    },
},
```

Targets resolve relative to the source first (a child of the source), then up the
ancestor chain, then absolutely from the root — so a deeply nested state can name
an ancestor's sibling without writing the full path.

## Parallel regions

Set `Type: NodeParallel` and the children become regions that are all active at
once. A parallel node has no `Initial` — every region is entered:

```go
"active": {
    Type: fate.NodeParallel,
    States: map[string]fate.StateNodeConfig[Ctx, Evt]{
        "main":   {Initial: "form",   States: ...},
        "review": {Initial: "queued", States: ...},
    },
},
```

An event is offered to every region; each region that has a matching transition
takes it. The active configuration now has two leaves, and `Path()` joins them
with `|`.

## Final states and completion

A `NodeFinal` state is terminal for its region. When a compound state's active
child reaches a final state, the parent's `OnDone` transitions fire. When the
top-level child completes, the actor's status becomes `StatusDone` and further
events are ignored. A final state may carry an `Output` function whose result is
captured into the snapshot.

## History

A history pseudo-state remembers where a compound state was when it was last
left. Declare it as a child of the compound whose history you want to remember:

```go
"editing": {
    Initial: "draft",
    States: map[string]fate.StateNodeConfig[Ctx, Evt]{
        "draft":      {On: ...},
        "review":     {On: ...},
        "hist":       {Type: fate.NodeHistory, History: fate.HistoryDeep, Default: "draft"},
    },
},
// elsewhere: re-enter via "editing.hist" to resume the exact sub-state.
```

`HistoryShallow` restores the immediate child; `HistoryDeep` restores the entire
saved subtree, including nested compound and parallel configurations. `Default`
is where it goes the first time, before any history exists. The classic use is
"interrupt and resume": some out-of-band activity pulls the machine away, and on
return deep history puts it back exactly where it was.

## Delayed transitions and invocations

States can also declare `After` (delayed transitions) and `Invoke` (work to run
while active). These are *effects*, and they work differently from everything
above — the engine records them as pending data rather than performing them. They
have their own guide: [effects and adapters](effects-and-adapters.md).
