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
