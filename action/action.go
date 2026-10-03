package action

import "github.com/arisros/fate/internal"

// Action is something executed as part of a transition or on state entry /
// exit. Actions may update the context, raise internal events, or log;
// I/O is forbidden (see ADR-002). All actions are pure with respect to
// time and randomness.
//
// Action is an interface because we want polymorphic concrete types
// (assignAction, raiseAction, etc.) while keeping the API ergonomic. The
// constructors in this package cover the built-in kinds; the engine is the
// only caller of Apply.
type Action[Ctx any, Evt any] interface {
	// Apply runs the action against ctx for evt and returns the new context.
	Apply(ctx Ctx, evt Evt, sink Sink[Evt]) Ctx
}

// Sink is the surface actions use to express side effects on the actor's
// internal queues. The actor implements it and passes it to [Action.Apply].
type Sink[Evt any] interface {
	// Raise places evt on the actor's internal queue.
	Raise(evt Evt)
	// Log routes msg to the actor's logger.
	Log(msg string)
}

// Assign returns an action that replaces the context with the result of fn.
// The function must be pure with respect to time, randomness, and I/O.
//
// Note: fn returns a whole new Ctx rather than patching in place. For struct
// contexts, the idiomatic pattern is `func(c Ctx, _ Evt) Ctx { c.Field = v; return c }`
// which leverages Go's value semantics.
func Assign[Ctx any, Evt any](fn func(ctx Ctx, evt Evt) Ctx) Action[Ctx, Evt] {
	return assignAction[Ctx, Evt]{fn: fn}
}

type assignAction[Ctx any, Evt any] struct {
	fn func(Ctx, Evt) Ctx
}

// Apply implements [Action].
func (a assignAction[Ctx, Evt]) Apply(c Ctx, e Evt, _ Sink[Evt]) Ctx {
	if a.fn == nil {
		return c
	}
	return a.fn(c, e)
}

// ImplName reports the label this action carries in a describe.MachineDescriptor, and
// through it in every rendered diagram. An assignment is an opaque closure, so
// the label names the kind rather than the effect; wrap it in [Named] to say
// what the assignment does.
func (a assignAction[Ctx, Evt]) ImplName() string { return "assign" }

// Raise returns an action that places an event onto the actor's internal
// queue. The event is processed before Send returns control to the caller.
// Equivalent to XState's `raise()`.
func Raise[Ctx any, Evt any](evt Evt) Action[Ctx, Evt] {
	return raiseAction[Ctx, Evt]{evt: evt}
}

type raiseAction[Ctx any, Evt any] struct {
	evt Evt
}

// Apply implements [Action].
func (a raiseAction[Ctx, Evt]) Apply(c Ctx, _ Evt, sink Sink[Evt]) Ctx {
	sink.Raise(a.evt)
	return c
}

// ImplName reports the label this action carries in a describe.MachineDescriptor. The
// raised event is known statically, so the label names it: "raise:CANCEL". An
// event whose name cannot be resolved degrades to a bare "raise".
func (a raiseAction[Ctx, Evt]) ImplName() string {
	if name := internal.EventName(a.evt); name != "" {
		return "raise:" + name
	}
	return "raise"
}

// Log returns an action that emits a log message. The actor routes log
// messages to its configured logger (default: discard).
func Log[Ctx any, Evt any](msg string) Action[Ctx, Evt] {
	return logAction[Ctx, Evt]{msg: msg}
}

type logAction[Ctx any, Evt any] struct {
	msg string
}

// Apply implements [Action].
func (a logAction[Ctx, Evt]) Apply(c Ctx, _ Evt, sink Sink[Evt]) Ctx {
	sink.Log(a.msg)
	return c
}

// ImplName reports the label this action carries in a describe.MachineDescriptor. The
// message is not included, because a log line is often long enough to overwhelm
// a diagram edge.
func (a logAction[Ctx, Evt]) ImplName() string { return "log" }

// Named labels an action so it appears under that name in a
// describe.MachineDescriptor, and through it in every rendered diagram. The built-in
// actions name their kind ("assign", "raise:CANCEL", "log"), which says what an
// action is but not what it does; Named is how a machine says the latter:
//
//	action.Named("lockApplication", action.Assign(func(c Ctx, _ Evt) Ctx {
//	    c.Locked = true
//	    return c
//	}))
//
// Wrapping changes nothing about how the action runs. A nil action is accepted
// and does nothing, so a name may be attached before the behaviour exists.
func Named[Ctx any, Evt any](name string, a Action[Ctx, Evt]) Action[Ctx, Evt] {
	return namedAction[Ctx, Evt]{name: name, inner: a}
}

type namedAction[Ctx any, Evt any] struct {
	name  string
	inner Action[Ctx, Evt]
}

// Apply implements [Action].
func (a namedAction[Ctx, Evt]) Apply(c Ctx, e Evt, sink Sink[Evt]) Ctx {
	if a.inner == nil {
		return c
	}
	return a.inner.Apply(c, e, sink)
}

// ImplName reports the caller-chosen label.
func (a namedAction[Ctx, Evt]) ImplName() string { return a.name }

// Enqueuer is the surface inside an EnqueueActions block. It batches a series
// of context updates, raises, and logs into one atomic application — the
// raised events accumulate but are not processed until the whole batch's
// context updates are committed.
type Enqueuer[Ctx any, Evt any] struct {
	ctx     Ctx
	evt     Evt
	sink    Sink[Evt]
	pending []Evt
}

// Assign applies fn to the running context. Subsequent calls compose.
func (e *Enqueuer[Ctx, Evt]) Assign(fn func(c Ctx, evt Evt) Ctx) {
	if fn != nil {
		e.ctx = fn(e.ctx, e.evt)
	}
}

// Raise schedules an event for processing after this batch's assigns commit.
func (e *Enqueuer[Ctx, Evt]) Raise(evt Evt) {
	e.pending = append(e.pending, evt)
}

// Log emits a log message immediately.
func (e *Enqueuer[Ctx, Evt]) Log(msg string) {
	e.sink.Log(msg)
}

// Context returns the in-progress context value. Useful for reading
// mid-batch.
func (e *Enqueuer[Ctx, Evt]) Context() Ctx { return e.ctx }

// EnqueueActions returns an action that runs fn against an Enqueuer.
// All context updates are applied; raises are deferred until the batch
// completes (then enqueued into the actor's internal queue in order).
func EnqueueActions[Ctx any, Evt any](fn func(enq *Enqueuer[Ctx, Evt])) Action[Ctx, Evt] {
	return enqueueAction[Ctx, Evt]{fn: fn}
}

type enqueueAction[Ctx any, Evt any] struct {
	fn func(*Enqueuer[Ctx, Evt])
}

// Apply implements [Action].
func (a enqueueAction[Ctx, Evt]) Apply(c Ctx, e Evt, sink Sink[Evt]) Ctx {
	if a.fn == nil {
		return c
	}
	enq := &Enqueuer[Ctx, Evt]{ctx: c, evt: e, sink: sink}
	a.fn(enq)
	for _, raised := range enq.pending {
		sink.Raise(raised)
	}
	return enq.ctx
}

// ImplName reports the label this action carries in a describe.MachineDescriptor. The
// batch is an opaque closure, so the label names the kind; wrap it in [Named] to
// say what the batch does.
func (a enqueueAction[Ctx, Evt]) ImplName() string { return "enqueue" }
