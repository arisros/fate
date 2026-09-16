package fate

import (
	"fmt"
	"sort"
)

// Setup is a type-safe registry of named guards and actions, mirroring XState v5's setup({ guards, actions }) ergonomic.
type Setup[Ctx any, Evt any] struct {
	guards  map[string]Guard[Ctx, Evt]
	actions map[string]Action[Ctx, Evt]
	missing map[string]struct{}
}

// NewSetup returns an empty registry.
func NewSetup[Ctx any, Evt any]() *Setup[Ctx, Evt] {
	return &Setup[Ctx, Evt]{
		guards:  map[string]Guard[Ctx, Evt]{},
		actions: map[string]Action[Ctx, Evt]{},
		missing: map[string]struct{}{},
	}
}

// WithGuard registers a guard under name and returns the Setup for chaining.
func (s *Setup[Ctx, Evt]) WithGuard(name string, g Guard[Ctx, Evt]) *Setup[Ctx, Evt] {
	s.guards[name] = g
	return s
}

// WithAction registers an action under name and returns the Setup for chaining.
func (s *Setup[Ctx, Evt]) WithAction(name string, a Action[Ctx, Evt]) *Setup[Ctx, Evt] {
	s.actions[name] = a
	return s
}

// Guard returns the guard registered under name for use in a [TransitionConfig].
func (s *Setup[Ctx, Evt]) Guard(name string) Guard[Ctx, Evt] {
	if g, ok := s.guards[name]; ok {
		return g
	}
	s.missing["guard:"+name] = struct{}{}
	return func(Ctx, Evt) bool { return false }
}

// Action returns the action registered under name for use in a [TransitionConfig] or a state's Entry/Exit.
func (s *Setup[Ctx, Evt]) Action(name string) Action[Ctx, Evt] {
	if a, ok := s.actions[name]; ok {
		return Named(name, a)
	}
	s.missing["action:"+name] = struct{}{}
	return Named[Ctx, Evt](name, nil)
}

// CreateMachine validates and builds the machine, first reporting any guard or action names referenced via [Setup.Guard] / [Setup.Action] that were never registered.
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
