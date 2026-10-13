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
	if got := (&Machine[int, string]{}).actionName(bareAction[int, string]{}); got != "" {
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

func TestLocationNamesTheFuncByItsPackage(t *testing.T) {
	cases := []struct{ symbol, file, wantFile, wantFunc string }{
		{"github.com/acme/app/orders.New.func1", "/src/orders/machine.go", "github.com/acme/app/orders/machine.go", "orders.New.func1"},
		{"gopkg.in/yaml%2ev3.(*Node).Decode", "/src/yaml.v3/node.go", "gopkg.in/yaml.v3/node.go", "yaml.v3.(*Node).Decode"},
		{"github.com/acme/app/orders.Pick[go.shape.int]", "/src/orders/pick.go", "github.com/acme/app/orders/pick.go", "orders.Pick[go.shape.int]"},
	}
	for _, c := range cases {
		got := location(c.symbol, c.file, 7)
		if got.File != c.wantFile || got.Func != c.wantFunc {
			t.Errorf("location(%q) = %q %q, want %q %q", c.symbol, got.File, got.Func, c.wantFile, c.wantFunc)
		}
	}

	if got := location("main.isReady", "/src/cmd/server/main.go", 7); got.Func != "main.isReady" {
		t.Errorf("a func in package main is named %q, want main.isReady", got.Func)
	}
}

func TestWrapperLocationSkipsTheEngine(t *testing.T) {
	if got := wrapperLocation(actionPkg + ".Guard[go.shape.int].Call-fm"); got != nil {
		t.Errorf("an engine wrapper was located at %+v", got)
	}
	if got := wrapperLocation("github.com/acme/app/orders.Service.Check-fm"); got == nil || got.Func != "orders.Service.Check" {
		t.Errorf("method value = %+v", got)
	}
}
