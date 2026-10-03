// Package describe defines what tooling reads from a machine without knowing
// its context or event types: the JSON [MachineDescriptor] that
// engine.Machine.Describe returns and [LoadDescriptor] reads back, and the
// per-state view model ([UIState]) whose schema the descriptor publishes.
package describe
