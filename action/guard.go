package action

import "github.com/arisros/fate/persist"

// Guard is a pure predicate over context and event.
type Guard[Ctx any, Evt any] func(ctx Ctx, evt Evt) bool

// AlwaysTrue is the implicit guard for transitions that declare no Guard.
func AlwaysTrue[Ctx any, Evt any]() Guard[Ctx, Evt] {
	return func(Ctx, Evt) bool { return true }
}

// And returns a guard that passes only when every supplied guard passes.
func And[Ctx any, Evt any](gs ...Guard[Ctx, Evt]) Guard[Ctx, Evt] {
	return func(c Ctx, e Evt) bool {
		for _, g := range gs {
			if g == nil {
				continue
			}
			if !g(c, e) {
				return false
			}
		}
		return true
	}
}

// Or returns a guard that passes when any supplied guard passes.
func Or[Ctx any, Evt any](gs ...Guard[Ctx, Evt]) Guard[Ctx, Evt] {
	return func(c Ctx, e Evt) bool {
		for _, g := range gs {
			if g == nil {
				continue
			}
			if g(c, e) {
				return true
			}
		}
		return false
	}
}

// Not negates a guard.
func Not[Ctx any, Evt any](g Guard[Ctx, Evt]) Guard[Ctx, Evt] {
	return func(c Ctx, e Evt) bool { return !g(c, e) }
}

// Cond is a structural transition condition evaluated against the actor's active state configuration, independent of context and event data.
type Cond interface {
	Matches(v persist.StateValue) bool
}

// InState returns a [Cond] that holds when the active configuration includes the given dot-separated state path.
func InState(path string) Cond { return inStateCond{path: path} }

// StateIn is an alias of [InState], named to match XState's stateIn guard for readers familiar with that library.
func StateIn(path string) Cond { return InState(path) }

type inStateCond struct{ path string }

// Matches implements [Cond].
func (c inStateCond) Matches(v persist.StateValue) bool { return v.Matches(c.path) }

// CondNot returns a [Cond] that holds when c does not.
func CondNot(c Cond) Cond { return notCond{c: c} }

type notCond struct{ c Cond }

// Matches implements [Cond].
func (n notCond) Matches(v persist.StateValue) bool { return n.c == nil || !n.c.Matches(v) }

// CondAllOf returns a [Cond] that holds only when every supplied condition holds.
func CondAllOf(cs ...Cond) Cond { return allOfCond{cs: cs} }

type allOfCond struct{ cs []Cond }

// Matches implements [Cond].
func (a allOfCond) Matches(v persist.StateValue) bool {
	for _, c := range a.cs {
		if c != nil && !c.Matches(v) {
			return false
		}
	}
	return true
}

// CondAnyOf returns a [Cond] that holds when at least one supplied condition holds.
func CondAnyOf(cs ...Cond) Cond { return anyOfCond{cs: cs} }

type anyOfCond struct{ cs []Cond }

// Matches implements [Cond].
func (a anyOfCond) Matches(v persist.StateValue) bool {
	for _, c := range a.cs {
		if c != nil && c.Matches(v) {
			return true
		}
	}
	return false
}
