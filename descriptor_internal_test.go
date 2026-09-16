package fate

import "testing"

type bareAction[Ctx any, Evt any] struct{}

func (bareAction[Ctx, Evt]) apply(c Ctx, _ Evt, _ actionSink[Ctx, Evt]) Ctx { return c }

func TestActionNameFallsBackForAnActionWithoutImplName(t *testing.T) {
	if got := actionName[int, string](bareAction[int, string]{}); got != "" {
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
