package engine

import (
	"sort"
	"strings"

	"github.com/arisros/fate/effect"
	"github.com/arisros/fate/persist"
)

type invokeBinding[Ctx any, Evt any] struct {
	node  *stateNode[Ctx, Evt]
	inv   effect.Invocation[Ctx, Evt]
	input any
}

func makeInvokeID(path []string, localID string) effect.InvokeID {
	var b strings.Builder
	b.WriteString(strings.Join(path, "."))
	b.WriteString("#invoke#")
	b.WriteString(localID)
	return effect.InvokeID(b.String())
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
func (a *Actor[Ctx, Evt]) PendingInvocations() []effect.PendingInvocation {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]effect.PendingInvocation, 0, len(a.pendingInvokes))
	for id, b := range a.pendingInvokes {
		out = append(out, effect.PendingInvocation{ID: id, Src: b.inv.Src, Input: b.input})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ResolveInvocation reports successful completion of the invocation with the given id.
func (a *Actor[Ctx, Evt]) ResolveInvocation(id effect.InvokeID, output any) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, ok := a.settleInvokeLocked(id)
	if !ok {
		return false
	}
	if b.inv.OnDone == nil {
		return true
	}
	a.deliverInvokeEventLocked(b.inv.OnDone(output))
	return true
}

// RejectInvocation reports failure of the invocation with the given id.
func (a *Actor[Ctx, Evt]) RejectInvocation(id effect.InvokeID, err error) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, ok := a.settleInvokeLocked(id)
	if !ok {
		return false
	}
	if b.inv.OnError == nil {
		return true
	}
	a.deliverInvokeEventLocked(b.inv.OnError(err))
	return true
}

func (a *Actor[Ctx, Evt]) settleInvokeLocked(id effect.InvokeID) (invokeBinding[Ctx, Evt], bool) {
	if a.status != persist.StatusRunning {
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
