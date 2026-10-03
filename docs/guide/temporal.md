# Running a machine in Temporal

The engine is dependency-free, so the [Temporal](https://temporal.io) integration
lives in a separate module:

```go
import fatetemporal "github.com/arisros/fate/temporal"
```

```sh
go get github.com/arisros/fate/temporal
```

You only pull in the Temporal SDK if you use this module. The engine module never
does.

## Why a machine fits Temporal

Temporal runs workflow code durably by re-executing it to rebuild state after a
failure, replaying recorded results in place of real ones. This only works if the
code is deterministic — every decision on replay must match the original. A fate
machine is deterministic by construction (see
[persistence and determinism](persistence-and-determinism.md)), so it slots into
a workflow without special care. What the workflow must *not* do is perform time
or I/O directly; it must route them through the SDK. That is exactly the line
fate already draws between the engine and its adapters — so the Temporal adapter
is a thin, generic driver, not a rewrite of your machine.

## WorkflowActor

`WorkflowActor` hosts a `fate.Actor` inside a workflow and drives its pending
effects with Temporal primitives:

```go
func MyWorkflow(ctx workflow.Context) (string, error) {
    m, err := buildMachine()
    if err != nil {
        return "", err
    }
    wa, err := fatetemporal.NewWorkflowActor(ctx, m, fatetemporal.Options{
        ActivityOptions: workflow.ActivityOptions{StartToCloseTimeout: time.Minute},
        SignalName:      "events", // external events arrive on this signal channel
    })
    if err != nil {
        return "", err
    }
    snap, err := wa.Run() // drives the machine to completion
    if err != nil {
        return "", err
    }
    return snap.Value.Path(), nil
}
```

`Run` is the adapter loop from the [effects guide](effects-and-adapters.md),
expressed in Temporal terms:

- each pending **timer** becomes a `workflow.NewTimer`; when it fires inside the
  workflow coroutine, the actor's `FireTimer` is called;
- each pending **invocation** becomes a `workflow.ExecuteActivity`, with `Src` as
  the activity name and `Input` as its argument; completion calls
  `ResolveInvocation`, failure `RejectInvocation`;
- external **events** arrive on a signal channel and become `Send`;
- effects are created, cancelled, and selected in a deterministic order, and
  every actor call happens on the workflow goroutine — so replay reproduces the
  same transitions.

You register the activities named by your invocations' `Src` values like any
other Temporal activities; the adapter just executes them by name.

## Long-running workflows: continue-as-new

A machine that waits for days produces a long event history. Use the actor's
snapshot with continue-as-new to keep histories bounded:

```go
blob, _ := wa.Persist()
return workflow.NewContinueAsNewError(ctx, MyWorkflow, blob)
// the next run rebuilds with NewWorkflowActorFromSnapshot(ctx, m, blob, opts)
```

A resumed actor re-derives its pending timers and invocations from the restored
configuration, so `Run` re-arms them automatically.

## Serialisation constraints

These are ordinary Temporal constraints, surfaced here because they touch the
machine's types:

- An invocation's input and result travel through Temporal's data converter.
  Results are decoded into `interface{}`, so an `OnDone` mapper sees the decoded
  shape — `float64` for numbers, `map[string]interface{}` for objects — exactly
  as any activity caller would.
- Events delivered by signal must be serialisable. A sealed-interface `Evt` needs
  a custom data converter or a concrete envelope, the same constraint `Persist`
  places on `Ctx` and `Evt`.

## Testing

The Temporal test environment drives the whole thing without a real cluster:
advance timers on its mock clock, run registered (or mocked) activities, send
signals, and assert the workflow result. The adapter's own tests do exactly this
and are a good template.
