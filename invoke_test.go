package fate_test

import (
	"context"
	"errors"
	"testing"

	fate "github.com/arisros/fate"
)

type ivCtx struct {
	Token  string
	Result string
}

type ivEvt interface{ isIv() }
type ivVerified struct{ ok bool }
type ivFailed struct{}

func (ivVerified) isIv() {}
func (ivFailed) isIv()   {}

func invokeMachine(t *testing.T) *fate.Machine[ivCtx, ivEvt] {
	t.Helper()
	m, err := fate.CreateMachine(fate.MachineConfig[ivCtx, ivEvt]{
		ID:      "verify",
		Initial: "checking",
		Context: ivCtx{Token: "abc"},
		States: map[string]fate.StateNodeConfig[ivCtx, ivEvt]{
			"checking": {
				Invoke: []fate.Invocation[ivCtx, ivEvt]{{
					ID:      "verify",
					Src:     "activity:verifyToken",
					Input:   func(c ivCtx) any { return c.Token },
					OnDone:  func(out any) ivEvt { return ivVerified{ok: out.(bool)} },
					OnError: func(error) ivEvt { return ivFailed{} },
				}},
				On: map[string][]fate.TransitionConfig[ivCtx, ivEvt]{
					"ivVerified": {{
						Target: "approved",
						Guard:  func(_ ivCtx, e ivEvt) bool { return e.(ivVerified).ok },
						Actions: []fate.Action[ivCtx, ivEvt]{
							fate.Assign(func(c ivCtx, _ ivEvt) ivCtx { c.Result = "ok"; return c }),
						},
					}},
					"ivFailed": {{Target: "rejected"}},
				},
			},
			"approved": {Type: fate.NodeFinal},
			"rejected": {Type: fate.NodeFinal},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return m
}

func TestInvokeResolveDrivesOnDone(t *testing.T) {
	a := fate.NewActor(invokeMachine(t))
	_ = a.Start(context.Background())

	pend := a.PendingInvocations()
	if len(pend) != 1 {
		t.Fatalf("want 1 pending invocation, got %d", len(pend))
	}
	if pend[0].Src != "activity:verifyToken" || pend[0].Input.(string) != "abc" {
		t.Fatalf("unexpected pending invocation %+v", pend[0])
	}
	if !a.Snapshot().Matches("checking") {
		t.Fatalf("want checking, got %s", a.Snapshot().Value.Path())
	}
	a.ResolveInvocation(pend[0].ID, true)
	snap := a.Snapshot()
	if !snap.Matches("approved") || snap.Status != fate.StatusDone {
		t.Fatalf("want approved/Done, got %s/%s", snap.Value.Path(), snap.Status)
	}
	if snap.Context.Result != "ok" {
		t.Fatalf("OnDone action should have run, result=%q", snap.Context.Result)
	}
	if n := len(a.PendingInvocations()); n != 0 {
		t.Fatalf("want 0 pending after resolve, got %d", n)
	}
}

func TestInvokeRejectDrivesOnError(t *testing.T) {
	a := fate.NewActor(invokeMachine(t))
	_ = a.Start(context.Background())
	pend := a.PendingInvocations()
	a.RejectInvocation(pend[0].ID, errors.New("boom"))
	if !a.Snapshot().Matches("rejected") {
		t.Fatalf("want rejected, got %s", a.Snapshot().Value.Path())
	}
}

func TestInvokeCancelledOnExitIgnoresLateResult(t *testing.T) {
	m, err := fate.CreateMachine(fate.MachineConfig[ivCtx, ivEvt]{
		ID:      "race",
		Initial: "checking",
		States: map[string]fate.StateNodeConfig[ivCtx, ivEvt]{
			"checking": {
				Invoke: []fate.Invocation[ivCtx, ivEvt]{{
					ID: "verify", Src: "activity:x",
					OnDone: func(any) ivEvt { return ivVerified{ok: true} },
				}},
				On: map[string][]fate.TransitionConfig[ivCtx, ivEvt]{
					"ivFailed": {{Target: "rejected"}},
				},
			},
			"approved": {Type: fate.NodeFinal},
			"rejected": {Type: fate.NodeFinal},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	a := fate.NewActor(m)
	_ = a.Start(context.Background())
	id := a.PendingInvocations()[0].ID
	_ = a.Send(context.Background(), ivFailed{})
	if !a.Snapshot().Matches("rejected") {
		t.Fatalf("want rejected, got %s", a.Snapshot().Value.Path())
	}
	a.ResolveInvocation(id, true)
	if !a.Snapshot().Matches("rejected") {
		t.Fatalf("late resolve must be ignored, got %s", a.Snapshot().Value.Path())
	}
}

func TestInvokeRederivedOnRestore(t *testing.T) {
	a := fate.NewActor(invokeMachine(t))
	_ = a.Start(context.Background())
	blob, err := a.Persist()
	if err != nil {
		t.Fatal(err)
	}
	b, err := fate.NewActorFromSnapshot[ivCtx, ivEvt](invokeMachine(t), blob)
	if err != nil {
		t.Fatal(err)
	}
	pend := b.PendingInvocations()
	if len(pend) != 1 || pend[0].Input.(string) != "abc" {
		t.Fatalf("restore should re-derive the pending invocation, got %+v", pend)
	}
	b.ResolveInvocation(pend[0].ID, true)
	if !b.Snapshot().Matches("approved") {
		t.Fatalf("restored actor should drive to approved, got %s", b.Snapshot().Value.Path())
	}
}

type spCtx struct{ ChildResult int }
type spEvt interface{ isSp() }
type spChildDone struct{ n int }

func (spChildDone) isSp() {}

func TestSpawnIsInvokeWithMachineSrc(t *testing.T) {
	m, err := fate.CreateMachine(fate.MachineConfig[spCtx, spEvt]{
		ID:      "parent",
		Initial: "running",
		States: map[string]fate.StateNodeConfig[spCtx, spEvt]{
			"running": {
				Invoke: []fate.Invocation[spCtx, spEvt]{{
					ID:     "child",
					Src:    "machine:subflow",
					OnDone: func(out any) spEvt { return spChildDone{n: out.(int)} },
				}},
				On: map[string][]fate.TransitionConfig[spCtx, spEvt]{
					"spChildDone": {{
						Target: "done",
						Actions: []fate.Action[spCtx, spEvt]{
							fate.Assign(func(c spCtx, e spEvt) spCtx { c.ChildResult = e.(spChildDone).n; return c }),
						},
					}},
				},
			},
			"done": {
				Type:   fate.NodeFinal,
				Output: func(c spCtx) any { return map[string]int{"child": c.ChildResult} },
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	a := fate.NewActor(m)
	_ = a.Start(context.Background())
	pend := a.PendingInvocations()
	if len(pend) != 1 || pend[0].Src != "machine:subflow" {
		t.Fatalf("want pending child machine invocation, got %+v", pend)
	}
	a.ResolveInvocation(pend[0].ID, 42)
	snap := a.Snapshot()
	if !snap.Matches("done") || snap.Status != fate.StatusDone {
		t.Fatalf("want done/Done, got %s/%s", snap.Value.Path(), snap.Status)
	}
	if string(snap.Output) != `{"child":42}` {
		t.Fatalf("want output {\"child\":42}, got %s", string(snap.Output))
	}
}
