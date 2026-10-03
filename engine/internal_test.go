package engine

import (
	"testing"

	"github.com/arisros/fate/action"
)

// bareAction implements Action without an ImplName. None of the built-in
// actions does, so this pins the fallback in actionName: an action type that
// omits ImplName degrades the descriptor to an empty name rather than
// panicking.
type bareAction[Ctx any, Evt any] struct{}

func (bareAction[Ctx, Evt]) Apply(c Ctx, _ Evt, _ action.Sink[Evt]) Ctx { return c }

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
