package fate

import (
	"sort"
	"strings"
)

// InvokeID identifies one armed invocation instance for as long as its state is active.
type InvokeID string

// Invocation declares external work a state runs while it is active — XState's invoke.
type Invocation[Ctx any, Evt any] struct {
	ID      string
	Src     string
	Input   func(ctx Ctx) any
	OnDone  func(output any) Evt
	OnError func(err error) Evt
}

// PendingInvocation is what an adapter reads from [Actor.PendingInvocations] to learn which work to run.
type PendingInvocation struct {
	ID    InvokeID
	Src   string
	Input any
}

type invokeBinding[Ctx any, Evt any] struct {
	node  *stateNode[Ctx, Evt]
	inv   Invocation[Ctx, Evt]
	input any
}

func makeInvokeID(path []string, localID string) InvokeID {
	var b strings.Builder
	b.WriteString(strings.Join(path, "."))
	b.WriteString("#invoke#")
	b.WriteString(localID)
	return InvokeID(b.String())
}

func (a *Actor[Ctx, Evt]) armInvokesLocked(n *stateNode[Ctx, Evt]) {
	for _, inv := range n.invokes {
		id := makeInvokeID(n.path, inv.ID)
		var input any
		if inv.Input != nil {
			input = inv.Input(a.ctx)
		}
		a.pendingInvokes[id] = invokeBinding[Ctx, Evt]{node: n, inv: inv, input: input}
	}
}

func (a *Actor[Ctx, Evt]) cancelInvokesLocked(n *stateNode[Ctx, Evt]) {
	for _, inv := range n.invokes {
		delete(a.pendingInvokes, makeInvokeID(n.path, inv.ID))
	}
}

// PendingInvocations returns the actor's currently-armed invocations, in deterministic order (by ID).
func (a *Actor[Ctx, Evt]) PendingInvocations() []PendingInvocation {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]PendingInvocation, 0, len(a.pendingInvokes))
	for id, b := range a.pendingInvokes {
		out = append(out, PendingInvocation{ID: id, Src: b.inv.Src, Input: b.input})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ResolveInvocation reports successful completion of the invocation with the given id.
func (a *Actor[Ctx, Evt]) ResolveInvocation(id InvokeID, output any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, ok := a.settleInvokeLocked(id)
	if !ok || b.inv.OnDone == nil {
		return
	}
	a.deliverInvokeEventLocked(b.inv.OnDone(output))
}

// RejectInvocation reports failure of the invocation with the given id.
func (a *Actor[Ctx, Evt]) RejectInvocation(id InvokeID, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, ok := a.settleInvokeLocked(id)
	if !ok || b.inv.OnError == nil {
		return
	}
	a.deliverInvokeEventLocked(b.inv.OnError(err))
}

func (a *Actor[Ctx, Evt]) settleInvokeLocked(id InvokeID) (invokeBinding[Ctx, Evt], bool) {
	if a.status != StatusRunning {
		return invokeBinding[Ctx, Evt]{}, false
	}
	b, ok := a.pendingInvokes[id]
	if !ok {
		return invokeBinding[Ctx, Evt]{}, false
	}
	delete(a.pendingInvokes, id)
	if _, active := extractValueAt[Ctx, Evt](a.machine.root, a.value, b.node); !active {
		return invokeBinding[Ctx, Evt]{}, false
	}
	return b, true
}

func (a *Actor[Ctx, Evt]) deliverInvokeEventLocked(evt Evt) {
	a.handleEventLocked(evt)
	a.drainQueueLocked()
	a.settleFinalLocked(evt)
	a.notifyLocked()
}
