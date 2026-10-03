// Package engine builds and runs statecharts.
//
// [CreateMachine] validates a [MachineConfig] into an immutable [Machine], and
// [Setup] does the same with guards and actions registered by name. An [Actor]
// runs a machine: [Actor.Start] it, feed it events with [Actor.Send], read it
// with [Actor.Snapshot], and store or restore it with [Actor.Persist] and
// [NewActorFromSnapshot].
//
// The engine performs no I/O and reads no clock. Delayed transitions and
// invocations are exposed as data ([Actor.PendingTimers],
// [Actor.PendingInvocations]) for a host to drive ([Actor.FireTimer],
// [Actor.ResolveInvocation], [Actor.RejectInvocation]).
//
// The other packages hold the vocabulary a machine is written in: action for
// actions, guards and conditions, effect for timers and invocations, persist
// for the active configuration and snapshots, and describe for the type-erased
// view that tooling reads.
package engine
