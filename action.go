package fate

// Action is something executed as part of a transition or on state entry / exit.
type Action[Ctx any, Evt any] interface {
	apply(ctx Ctx, evt Evt, sink actionSink[Ctx, Evt]) Ctx
}

type actionSink[Ctx any, Evt any] interface {
	raise(Evt)
	log(string)
}

// Assign returns an action that replaces the context with the result of fn.
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

// ImplName reports the label this action carries in a [MachineDescriptor], and through it in every rendered diagram.
func (a assignAction[Ctx, Evt]) ImplName() string { return "assign" }

// Raise returns an action that places an event onto the actor's internal queue.
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

// ImplName reports the label this action carries in a [MachineDescriptor].
func (a raiseAction[Ctx, Evt]) ImplName() string {
	if name := eventNameOf(a.evt); name != "" {
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

func (a logAction[Ctx, Evt]) apply(c Ctx, _ Evt, sink actionSink[Ctx, Evt]) Ctx {
	sink.log(a.msg)
	return c
}

// ImplName reports the label this action carries in a [MachineDescriptor].
func (a logAction[Ctx, Evt]) ImplName() string { return "log" }

// Named labels an action so it appears under that name in a [MachineDescriptor], and through it in every rendered diagram.
func Named[Ctx any, Evt any](name string, a Action[Ctx, Evt]) Action[Ctx, Evt] {
	return namedAction[Ctx, Evt]{name: name, inner: a}
}

type namedAction[Ctx any, Evt any] struct {
	name  string
	inner Action[Ctx, Evt]
}

func (a namedAction[Ctx, Evt]) apply(c Ctx, e Evt, sink actionSink[Ctx, Evt]) Ctx {
	if a.inner == nil {
		return c
	}
	return a.inner.apply(c, e, sink)
}

func (a namedAction[Ctx, Evt]) ImplName() string { return a.name }

// Enqueuer is the surface inside an EnqueueActions block.
type Enqueuer[Ctx any, Evt any] struct {
	ctx     Ctx
	evt     Evt
	sink    actionSink[Ctx, Evt]
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
	e.sink.log(msg)
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

// ImplName reports the label this action carries in a [MachineDescriptor].
func (a enqueueAction[Ctx, Evt]) ImplName() string { return "enqueue" }
