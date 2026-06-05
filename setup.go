package fate

import (
	"fmt"
	"sort"
)

// Setup is a type-safe registry of named guards and actions, mirroring
// XState v5's setup({ guards, actions }) ergonomic. Register implementations
// once, then reference them by name while declaring a [MachineConfig] via the
// [Setup.Guard] and [Setup.Action] accessors. This keeps large machine configs
// readable and lets several transitions share one implementation.
//
// Setup is sugar over [CreateMachine]; it adds no semantics the declarative
// config cannot express. A typical use:
//
//	s := fate.NewSetup[Ctx, Evt]().
//		WithGuard("isHighRisk", func(c Ctx, _ Evt) bool { return c.Risk == "HIGH" }).
//		WithAction("clearForm", fate.Assign(func(c Ctx, _ Evt) Ctx { c.Form = nil; return c }))
//
//	m, err := s.CreateMachine(fate.MachineConfig[Ctx, Evt]{
//		ID: "review", Initial: "open",
//		States: map[string]fate.StateNodeConfig[Ctx, Evt]{
//			"open": {On: map[string][]fate.TransitionConfig[Ctx, Evt]{
//				"NEXT": {{Target: "closed", Guard: s.Guard("isHighRisk"),
//					Actions: []fate.Action[Ctx, Evt]{s.Action("clearForm")}}},
//			}},
//			"closed": {Type: fate.NodeFinal},
//		},
//	})
//
// Referencing a name that was never registered is reported as an error from
// [Setup.CreateMachine], so typos surface at construction time rather than
// silently doing nothing.
type Setup[Ctx any, Evt any] struct {
	guards  map[string]Guard[Ctx, Evt]
	actions map[string]Action[Ctx, Evt]
	missing map[string]struct{} // names referenced but not registered
}

// NewSetup returns an empty registry. Register entries with [Setup.WithGuard]
// and [Setup.WithAction] (both chainable).
func NewSetup[Ctx any, Evt any]() *Setup[Ctx, Evt] {
	return &Setup[Ctx, Evt]{
		guards:  map[string]Guard[Ctx, Evt]{},
		actions: map[string]Action[Ctx, Evt]{},
		missing: map[string]struct{}{},
	}
}

// WithGuard registers a guard under name and returns the Setup for chaining.
// Registering the same name twice replaces the earlier guard.
func (s *Setup[Ctx, Evt]) WithGuard(name string, g Guard[Ctx, Evt]) *Setup[Ctx, Evt] {
	s.guards[name] = g
	return s
}

// WithAction registers an action under name and returns the Setup for chaining.
// Registering the same name twice replaces the earlier action.
func (s *Setup[Ctx, Evt]) WithAction(name string, a Action[Ctx, Evt]) *Setup[Ctx, Evt] {
	s.actions[name] = a
	return s
}

// Guard returns the guard registered under name for use in a
// [TransitionConfig]. If no guard is registered under name, Guard records the
// missing reference (so [Setup.CreateMachine] returns an error) and returns a
// guard that never passes, keeping config construction safe to continue.
func (s *Setup[Ctx, Evt]) Guard(name string) Guard[Ctx, Evt] {
	if g, ok := s.guards[name]; ok {
		return g
	}
	s.missing["guard:"+name] = struct{}{}
	return func(Ctx, Evt) bool { return false }
}

// Action returns the action registered under name for use in a
// [TransitionConfig] or a state's Entry/Exit. If no action is registered under
// name, Action records the missing reference (so [Setup.CreateMachine] returns
// an error) and returns a no-op action.
func (s *Setup[Ctx, Evt]) Action(name string) Action[Ctx, Evt] {
	if a, ok := s.actions[name]; ok {
		return a
	}
	s.missing["action:"+name] = struct{}{}
	return assignAction[Ctx, Evt]{fn: nil}
}

// CreateMachine validates and builds the machine, first reporting any guard or
// action names referenced via [Setup.Guard] / [Setup.Action] that were never
// registered. On success it is identical to calling [CreateMachine] directly.
func (s *Setup[Ctx, Evt]) CreateMachine(cfg MachineConfig[Ctx, Evt]) (*Machine[Ctx, Evt], error) {
	if len(s.missing) > 0 {
		names := make([]string, 0, len(s.missing))
		for n := range s.missing {
			names = append(names, n)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("%w: unregistered references %v", ErrInvalidConfig, names)
	}
	return CreateMachine(cfg)
}
