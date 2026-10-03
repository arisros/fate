// Package persist defines the serialisable state of a running actor: the
// active configuration ([StateValue]) and the [Snapshot] an actor reports.
//
// Storing and restoring an actor are engine.Actor.Persist and
// engine.NewActorFromSnapshot, because they read the actor's private state.
package persist
