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
// # Packages
//
// The engine is split by concern:
//
//   - [github.com/arisros/fate/engine]: build a Machine and run it as an Actor.
//   - [github.com/arisros/fate/action]: actions, guards and conditions.
//   - [github.com/arisros/fate/effect]: timers and invocations a host drives.
//   - [github.com/arisros/fate/persist]: the active configuration and snapshots.
//   - [github.com/arisros/fate/describe]: the type-erased view tooling reads.
//
// The root package holds [Version] and deprecated aliases for the names that
// lived here before the split; new code imports the packages above.
//
// Built on those: render, diff, snapshot, httphandler and testing. The
// Temporal integration is the separate github.com/arisros/fate/temporal module.
//
// # Design principles
//
//   - Zero dependencies: the root module imports only the standard library.
//     Anything that needs an external dependency (the Temporal integration)
//     lives in a separate module so adopters opt in explicitly.
//   - Determinism: a Machine is immutable once constructed and is safe to
//     share across goroutines. All observable iteration is ordered. Given the
//     same machine and the same event sequence, an Actor produces a
//     byte-identical persisted snapshot. This makes fate safe to drive from
//     deterministic execution environments such as Temporal workflows.
//   - Persistence first: actor state serialises to and restores from JSON via
//     Actor.Persist and NewActorFromSnapshot. The snapshot shape is versioned
//     so it can evolve without breaking stored data.
//
// See the engine package for how to define and drive a machine, and the
// examples directory for runnable machines.
package fate

// Version is the semantic version of the fate library. It is updated by the
// release process and surfaced here so programs can report the engine version
// they were built against.
const Version = "0.9.0" // x-release-please-version
