package action

import "github.com/arisros/fate/internal"

// Action is something executed as part of a transition or on state entry / exit.
// Runs locked: must not call the actor or block.
type Action[Ctx any, Evt any] interface {
	Apply(ctx Ctx, evt Evt, sink Sink[Evt]) Ctx
}

// Sink is the surface actions use to express side effects on the actor's internal queues.
type Sink[Evt any] interface {
	Raise(evt Evt)
	Log(msg string)
}

// Assign returns an action that replaces the context with the result of fn.
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

// ImplName reports the label this action carries in a describe.MachineDescriptor, and through it in every rendered diagram.
func (a assignAction[Ctx, Evt]) ImplName() string { return "assign" }

// Raise returns an action that places an event onto the actor's internal queue.
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

// ImplName reports the label this action carries in a describe.MachineDescriptor.
func (a raiseAction[Ctx, Evt]) ImplName() string {
	if name := internal.EventName(a.evt); name != "" {
		return "raise:" + name
	}
	return "raise"
}

// Log returns an action that emits a log message.
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

// ImplName reports the label this action carries in a describe.MachineDescriptor.
func (a logAction[Ctx, Evt]) ImplName() string { return "log" }

// Named labels an action so it appears under that name in a describe.MachineDescriptor, and through it in every rendered diagram.
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

// Enqueuer is the surface inside an EnqueueActions block.
type Enqueuer[Ctx any, Evt any] struct {
	ctx     Ctx
	evt     Evt
	sink    Sink[Evt]
	pending []Evt
}

// Assign applies fn to the running context.
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

// Context returns the in-progress context value.
func (e *Enqueuer[Ctx, Evt]) Context() Ctx { return e.ctx }

// EnqueueActions returns an action that runs fn against an Enqueuer.
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

// ImplName reports the label this action carries in a describe.MachineDescriptor.
func (a enqueueAction[Ctx, Evt]) ImplName() string { return "enqueue" }
