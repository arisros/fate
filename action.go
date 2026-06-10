package fate

// Action is something executed as part of a transition or on state entry /
// exit. Actions may update the context, raise internal events, or log;
// I/O is forbidden (see ADR-002). All actions are pure with respect to
// time and randomness.
//
// Action is an interface because we want polymorphic concrete types
// (assignAction, raiseAction, etc.) while keeping the API ergonomic.
type Action[Ctx any, Evt any] interface {
	apply(ctx Ctx, evt Evt, sink actionSink[Ctx, Evt]) Ctx
}

// actionSink is the surface actions use to express side effects on the
// actor's internal queues. Implemented by the actor; not exposed to users.
type actionSink[Ctx any, Evt any] interface {
	raise(Evt)
	log(string)
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

func (a assignAction[Ctx, Evt]) apply(c Ctx, e Evt, _ actionSink[Ctx, Evt]) Ctx {
	if a.fn == nil {
		return c
	}
	return a.fn(c, e)
}

// Raise returns an action that places an event onto the actor's internal
// queue. The event is processed before Send returns control to the caller.
// Equivalent to XState's `raise()`.
func Raise[Ctx any, Evt any](evt Evt) Action[Ctx, Evt] {
	return raiseAction[Ctx, Evt]{evt: evt}
}

type raiseAction[Ctx any, Evt any] struct {
	evt Evt
}

func (a raiseAction[Ctx, Evt]) apply(c Ctx, _ Evt, sink actionSink[Ctx, Evt]) Ctx {
	sink.raise(a.evt)
	return c
}

// Log returns an action that emits a log message. The actor routes log
// messages to its configured logger (default: discard).
func Log[Ctx any, Evt any](msg string) Action[Ctx, Evt] {
	return logAction[Ctx, Evt]{msg: msg}
}

type logAction[Ctx any, Evt any] struct {
	msg string
}

func (a logAction[Ctx, Evt]) apply(c Ctx, _ Evt, sink actionSink[Ctx, Evt]) Ctx {
	sink.log(a.msg)
	return c
}

// Enqueuer is the surface inside an EnqueueActions block. It batches a series
// of context updates, raises, and logs into one atomic application — the
// raised events accumulate but are not processed until the whole batch's
// context updates are committed.
type Enqueuer[Ctx any, Evt any] struct {
	ctx     Ctx
	evt     Evt
	sink    actionSink[Ctx, Evt]
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
	e.sink.log(msg)
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

func (a enqueueAction[Ctx, Evt]) apply(c Ctx, e Evt, sink actionSink[Ctx, Evt]) Ctx {
	if a.fn == nil {
		return c
	}
	enq := &Enqueuer[Ctx, Evt]{ctx: c, evt: e, sink: sink}
	a.fn(enq)
	for _, raised := range enq.pending {
		sink.raise(raised)
	}
	return enq.ctx
}
