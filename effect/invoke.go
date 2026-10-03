package effect

// InvokeID identifies one armed invocation instance for as long as its state is
// active. It is derived deterministically from the owning state's path and the
// invocation's local ID, so the same logical invocation has the same ID across
// runs and across persistence.
type InvokeID string

// Invocation declares external work a state runs while it is active — XState's
// invoke. The fate core treats Src as an opaque name and never executes it: on
// entering the state the core records a pending invocation; on exit it disarms
// it. An adapter discovers pending invocations via engine.Actor.PendingInvocations,
// runs the work named by Src, and reports the outcome via
// engine.Actor.ResolveInvocation or engine.Actor.RejectInvocation. The core then maps the
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

// PendingInvocation is what an adapter reads from engine.Actor.PendingInvocations to
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
