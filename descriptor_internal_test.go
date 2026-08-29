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
