package fate

import (
	"sort"
	"strings"
)

// InvokeID identifies one armed invocation instance for as long as its state is
// active. It is derived deterministically from the owning state's path and the
// invocation's local ID, so the same logical invocation has the same ID across
// runs and across persistence.
type InvokeID string

// Invocation declares external work a state runs while it is active — XState's
// invoke. The fate core treats Src as an opaque name and never executes it: on
// entering the state the core records a pending invocation; on exit it disarms
// it. An adapter discovers pending invocations via [Actor.PendingInvocations],
// runs the work named by Src, and reports the outcome via
// [Actor.ResolveInvocation] or [Actor.RejectInvocation]. The core then maps the
// outcome to an event (OnDone / OnError) and processes it — but only if the
// owning state is still active.
//
// Because Src is opaque, the same mechanism expresses both a service/activity
// call and a spawned child machine: the adapter decides what Src means (a
// Temporal activity, a child workflow, a nested actor). See ADR-0004.
type Invocation[Ctx any, Evt any] struct {
	// ID is unique within its state; combined with the state path it forms the
	// invocation's stable [InvokeID].
	ID string
	// Src is the opaque logical name of the work to run.
	Src string
	// Input, if non-nil, builds the invocation input from the context captured
	// when the state is entered. Exposed to the adapter via PendingInvocation.
	Input func(ctx Ctx) any
	// OnDone, if non-nil, maps a successful result to an event the machine then
	// processes. If nil, a successful resolution is dropped.
	OnDone func(output any) Evt
	// OnError, if non-nil, maps a failure to an event the machine then
	// processes. If nil, a failure is dropped.
	OnError func(err error) Evt
}

// PendingInvocation is what an adapter reads from [Actor.PendingInvocations] to
// learn which work to run. ID is passed back to ResolveInvocation /
// RejectInvocation when the work settles.
type PendingInvocation struct {
	// ID is the invocation's stable identifier.
	ID InvokeID
	// Src is the opaque work name declared on the Invocation.
	Src string
	// Input is the payload built from context at arm time (nil if no Input fn).
	Input any
}

// invokeBinding records an armed invocation so a later resolve/reject can map
// the outcome to an event and confirm the owning state is still active.
type invokeBinding[Ctx any, Evt any] struct {
	node  *stateNode[Ctx, Evt]
	inv   Invocation[Ctx, Evt]
	input any
}

// makeInvokeID derives the deterministic [InvokeID] for an invocation declared
// at the given state path with the given local ID.
func makeInvokeID(path []string, localID string) InvokeID {
	var b strings.Builder
	b.WriteString(strings.Join(path, "."))
	b.WriteString("#invoke#")
	b.WriteString(localID)
	return InvokeID(b.String())
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

// ResolveInvocation reports successful completion of the invocation with the
// given id. If it is still armed (its state still active) and declares OnDone,
// the mapped event is processed as an internal step. Resolving an unknown or
// already-settled id is a safe no-op.
func (a *Actor[Ctx, Evt]) ResolveInvocation(id InvokeID, output any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, ok := a.settleInvokeLocked(id)
	if !ok || b.inv.OnDone == nil {
		return
	}
	a.deliverInvokeEventLocked(b.inv.OnDone(output))
}

// RejectInvocation reports failure of the invocation with the given id. If it is
// still armed and declares OnError, the mapped event is processed as an internal
// step. Rejecting an unknown or already-settled id is a safe no-op.
func (a *Actor[Ctx, Evt]) RejectInvocation(id InvokeID, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, ok := a.settleInvokeLocked(id)
	if !ok || b.inv.OnError == nil {
		return
	}
	a.deliverInvokeEventLocked(b.inv.OnError(err))
}

// settleInvokeLocked removes an armed invocation and confirms its state is still
// active. Returns (binding, true) when the outcome should be delivered.
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

// deliverInvokeEventLocked processes an invocation outcome event exactly like a
// sent event: handle, drain raised events, settle finals, notify observers.
func (a *Actor[Ctx, Evt]) deliverInvokeEventLocked(evt Evt) {
	a.handleEventLocked(evt)
	a.drainQueueLocked()
	a.settleFinalLocked(evt)
	a.notifyLocked()
}
