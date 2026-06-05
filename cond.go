package fate

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
