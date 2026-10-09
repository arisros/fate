package engine

import (
	"testing"

	"github.com/arisros/fate/action"
)

type bareAction[Ctx any, Evt any] struct{}

func (bareAction[Ctx, Evt]) Apply(c Ctx, _ Evt, _ action.Sink[Evt]) Ctx { return c }

func TestActionNameFallsBackForAnActionWithoutImplName(t *testing.T) {
	if got := (&Machine[int, string]{}).actionName(bareAction[int, string]{}); got != "" {
		t.Errorf("actionName = %q, want \"\"", got)
	}
}

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
