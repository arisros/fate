package fate

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
