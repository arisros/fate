package fate

import "testing"

// bareAction implements Action without an ImplName. No such type exists in the
// package today, and one cannot be written outside it either, because Action's
// only method is unexported. The fallback in actionName is therefore
// unreachable through the public API, and this internal test is what keeps it
// honest: if a future action type forgets ImplName, the descriptor degrades to
// an empty name rather than panicking.
type bareAction[Ctx any, Evt any] struct{}

func (bareAction[Ctx, Evt]) apply(c Ctx, _ Evt, _ actionSink[Ctx, Evt]) Ctx { return c }

func TestActionNameFallsBackForAnActionWithoutImplName(t *testing.T) {
	if got := actionName[int, string](bareAction[int, string]{}); got != "" {
		t.Errorf("actionName = %q, want \"\"", got)
	}
}

// exitDomain narrows a parallel LCCA to the region containing the target. Every
// LCCA the engine produces is a proper ancestor of the target, so the fallback
// for a target outside the domain is unreachable through the public API. This
// pins the contract anyway: an unrecognised shape keeps the domain it was given
// rather than returning nil and collapsing the exit set to nothing.
func TestExitDomainKeepsAParallelDomainWhenTargetIsOutsideIt(t *testing.T) {
	parallel := &stateNode[int, string]{typ: NodeParallel}
	outside := &stateNode[int, string]{typ: NodeAtomic}

	if got := exitDomain(parallel, outside); got != parallel {
		t.Errorf("exitDomain returned %v, want the parallel node unchanged", got)
	}
	if got := exitDomain[int, string](nil, outside); got != nil {
		t.Errorf("exitDomain(nil, ...) = %v, want nil", got)
	}
}
