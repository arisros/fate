// Package action holds what a transition runs and what gates it.
//
// Actions ([Assign], [Raise], [Log], [Named], [EnqueueActions]) update the
// context and queue events. Guards ([Guard], [And], [Or], [Not]) decide on
// context and event data. Conditions ([Cond], [InState]) decide on the active
// state configuration. [CondMeta] documents a guard for tooling without
// affecting whether the transition fires.
package action
