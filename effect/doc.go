// Package effect defines the work an actor asks its host to run: delayed
// transitions ([PendingTimer]) and invocations ([Invocation],
// [PendingInvocation]).
//
// The engine never runs an effect itself. It records the intent, the host
// reads it from engine.Actor, and reports the outcome back by ID.
package effect
