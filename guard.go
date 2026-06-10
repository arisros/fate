package fate

// Guard is a pure predicate over context and event. Returning true selects
// the transition; returning false skips it. Guards must be pure (no I/O,
// no time, no randomness) — see ADR-002.
type Guard[Ctx any, Evt any] func(ctx Ctx, evt Evt) bool

// AlwaysTrue is the implicit guard for transitions that declare no Guard.
// Exposed for combinator chaining.
func AlwaysTrue[Ctx any, Evt any]() Guard[Ctx, Evt] {
	return func(Ctx, Evt) bool { return true }
}

// And returns a guard that passes only when every supplied guard passes.
// Short-circuits on the first false.
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
// Short-circuits on the first true.
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

// To match against the active state configuration rather than context or event
// data — XState's stateIn guard — use a [Cond] via [TransitionConfig.Cond]
// (see [StateIn] / [InState]). Guards intentionally see only (context, event)
// so they remain pure functions of data.

// Cond is a structural transition condition evaluated against the actor's
// active state configuration, independent of context and event data. It is the
// fate equivalent of XState's stateIn guard.
//
// A [Guard] sees only (context, event); a Cond sees only which states are
// currently active. The two are complementary: set both [TransitionConfig.Guard]
// and [TransitionConfig.Cond] and the transition fires only when both pass.
//
// Build a Cond with [StateIn] / [InState] and compose with [CondNot],
// [CondAllOf], and [CondAnyOf]. Conds hold no mutable state and no reference to
// any actor, so a Cond built once is safe to share across machines and
// goroutines.
type Cond interface {
	// matches reports whether the condition holds for the given active
	// configuration. Unexported so the set of Cond implementations stays
	// closed to this package.
	matches(v StateValue) bool
}

// InState returns a [Cond] that holds when the active configuration includes
// the given dot-separated state path. Matching uses [StateValue.Matches], so a
// prefix such as "menu.settings" matches any deeper active leaf beneath it.
func InState(path string) Cond { return inStateCond{path: path} }

// StateIn is an alias of [InState], named to match XState's stateIn guard for
// readers familiar with that library.
func StateIn(path string) Cond { return InState(path) }

type inStateCond struct{ path string }

func (c inStateCond) matches(v StateValue) bool { return v.Matches(c.path) }

// CondNot returns a [Cond] that holds when c does not.
func CondNot(c Cond) Cond { return notCond{c: c} }

type notCond struct{ c Cond }

func (n notCond) matches(v StateValue) bool { return n.c == nil || !n.c.matches(v) }

// CondAllOf returns a [Cond] that holds only when every supplied condition
// holds. With no arguments it always holds.
func CondAllOf(cs ...Cond) Cond { return allOfCond{cs: cs} }

type allOfCond struct{ cs []Cond }

func (a allOfCond) matches(v StateValue) bool {
	for _, c := range a.cs {
		if c != nil && !c.matches(v) {
			return false
		}
	}
	return true
}

// CondAnyOf returns a [Cond] that holds when at least one supplied condition
// holds. With no arguments it never holds.
func CondAnyOf(cs ...Cond) Cond { return anyOfCond{cs: cs} }

type anyOfCond struct{ cs []Cond }

func (a anyOfCond) matches(v StateValue) bool {
	for _, c := range a.cs {
		if c != nil && c.matches(v) {
			return true
		}
	}
	return false
}

// transitionPasses reports whether a transition's combined Guard and Cond admit
// it for the given context, event, and active configuration. A nil Guard or nil
// Cond is treated as "always passes". Centralised here so every selection site
// (event handling, wildcard, onDone, and after timers) applies identical
// semantics.
func transitionPasses[Ctx any, Evt any](t TransitionConfig[Ctx, Evt], ctx Ctx, evt Evt, value StateValue) bool {
	if t.Guard != nil && !t.Guard(ctx, evt) {
		return false
	}
	if t.Cond != nil && !t.Cond.matches(value) {
		return false
	}
	return true
}
