// Package fate is a statechart engine for Go.
//
// fate implements Harel statecharts: hierarchical (nested) states, parallel
// regions, and deep/shallow history — not a flat finite automaton. It is
// inspired by the semantics of SCXML and XState v5, expressed idiomatically in
// Go with strong typing via generics over a user-defined context (Ctx) and
// event (Evt) type.
//
// # Why "statechart", not "state machine"
//
// A classic finite-state machine has one active state at a time and no nesting.
// A statechart adds hierarchy (a state can contain sub-states), orthogonality
// (independent parallel regions that are all active at once), and history
// (re-entering a compound state can restore the sub-state it was last in).
// These features collapse the combinatorial state explosion that makes flat
// machines unmanageable for real workflows. fate is a statechart engine; the
// name is not an acronym.
//
// # Design principles
//
//   - Zero dependencies: the root module imports only the standard library.
//     Anything that needs an external dependency (the Temporal integration)
//     lives in a separate module so adopters opt in explicitly.
//   - Determinism: a [Machine] is immutable once constructed and is safe to
//     share across goroutines. All observable iteration is ordered. Given the
//     same machine and the same event sequence, an [Actor] produces a
//     byte-identical persisted snapshot. This makes fate safe to drive from
//     deterministic execution environments such as Temporal workflows.
//   - Persistence first: actor state serialises to and restores from JSON via
//     [Actor.Persist] and [NewActorFromSnapshot]. The snapshot shape is
//     versioned so it can evolve without breaking stored data.
//
// # Two ways to define a machine
//
// Define a machine directly with [CreateMachine] and the declarative
// [MachineConfig] / [StateNodeConfig] / [TransitionConfig] structs, or use the
// type-safe [Setup] builder to register named guards, actions and actors once
// and reference them by name from the config. Both produce the same immutable
// [Machine].
//
// # Driving a machine
//
// Construct an [Actor] from a [Machine], [Actor.Start] it, and feed it events
// with [Actor.Send]. Read the current state with [Actor.Snapshot], observe
// changes with [Actor.Subscribe], and persist/restore with [Actor.Persist] and
// [NewActorFromSnapshot]. To drive a machine inside a Temporal workflow, use
// the WorkflowActor from the github.com/arisros/fate/temporal module instead of
// a bare [Actor].
//
// See the examples directory and the package examples for runnable machines.
package fate
