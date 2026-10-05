package engine

import (
	"sort"
	"strings"

	"github.com/arisros/fate/effect"
	"github.com/arisros/fate/persist"
)

// invokeBinding records an armed invocation so a later resolve/reject can map
// the outcome to an event and confirm the owning state is still active.
type invokeBinding[Ctx any, Evt any] struct {
	node  *stateNode[Ctx, Evt]
	inv   effect.Invocation[Ctx, Evt]
	input any
}

// makeInvokeID derives the deterministic [effect.InvokeID] for an invocation declared
// at the given state path with the given local ID.
func makeInvokeID(path []string, localID string) effect.InvokeID {
	var b strings.Builder
	b.WriteString(strings.Join(path, "."))
	b.WriteString("#invoke#")
	b.WriteString(localID)
	return effect.InvokeID(b.String())
}

// armInvokesLocked records every invocation declared on n as pending, capturing
// each input from the current context. Called when n is entered. The actor
// mutex must be held.
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

// cancelInvokesLocked disarms every invocation declared on n. Called when n is
// exited. The actor mutex must be held.
func (a *Actor[Ctx, Evt]) cancelInvokesLocked(n *stateNode[Ctx, Evt]) {
	for _, inv := range n.invokes {
		delete(a.pendingInvokes, makeInvokeID(n.path, inv.ID))
	}
}

// PendingInvocations returns the actor's currently-armed invocations, in
// deterministic order (by ID). It is the read half of the invoke effect: an
// adapter runs each Src and reports the outcome via ResolveInvocation /
// RejectInvocation. The core never runs an invocation itself. See ADR-0004.
func (a *Actor[Ctx, Evt]) PendingInvocations() []effect.PendingInvocation {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]effect.PendingInvocation, 0, len(a.pendingInvokes))
	for id, b := range a.pendingInvokes {
		out = append(out, effect.PendingInvocation{ID: id, Src: b.inv.Src, Input: b.input, State: dotPath(b.node)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ResolveInvocation reports successful completion of the invocation with the
// given id. If it is still armed (its state still active) and declares OnDone,
// the mapped event is processed as an internal step. Resolving an unknown or
// already-settled id is a safe no-op.
// The reported bool is true when the invocation was still armed and its owning
// state still active, so the outcome was accepted. It is false when the id was
// unknown, already settled, or belongs to a state the machine has since left.
// Note that true means accepted, not that an event was delivered: an accepted
// invocation with no OnDone mapper settles without producing one. Adapters use
// this to tell a delivered result from a late one; callers that do not care may
// discard it.
func (a *Actor[Ctx, Evt]) ResolveInvocation(id effect.InvokeID, output any) bool {
	defer a.deliver()
	a.mu.Lock()
	defer a.mu.Unlock()
	b, ok := a.settleInvokeLocked(id)
	if !ok {
		return false
	}
	if b.inv.OnDone == nil {
		return true
	}
	a.deliverInvokeEventLocked(id, b.inv.OnDone(output))
	return true
}

// RejectInvocation reports failure of the invocation with the given id. If it is
// still armed and declares OnError, the mapped event is processed as an internal
// step. Rejecting an unknown or already-settled id is a safe no-op.
//
// The reported bool carries the same meaning as in [Actor.ResolveInvocation]:
// true when the invocation was accepted, false when the id was unknown, already
// settled, or owned by a state the machine has since left. An accepted
// invocation with no OnError mapper reports true and delivers no event, which
// is how a failure with no declared handler is silently absorbed.
func (a *Actor[Ctx, Evt]) RejectInvocation(id effect.InvokeID, err error) bool {
	defer a.deliver()
	a.mu.Lock()
	defer a.mu.Unlock()
	b, ok := a.settleInvokeLocked(id)
	if !ok {
		return false
	}
	if b.inv.OnError == nil {
		return true
	}
	a.deliverInvokeEventLocked(id, b.inv.OnError(err))
	return true
}

// settleInvokeLocked removes an armed invocation and confirms its state is still
// active. Returns (binding, true) when the outcome should be delivered.
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

// deliverInvokeEventLocked processes an invocation outcome event exactly like a
// sent event: handle, drain raised events, settle finals, notify observers.
func (a *Actor[Ctx, Evt]) deliverInvokeEventLocked(id effect.InvokeID, evt Evt) {
	a.handleEventLocked(evt, StepInvoke, string(id))
	a.drainQueueLocked()
	a.settleFinalLocked(evt)
	a.notifyLocked()
}
