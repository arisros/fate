package temporal

import (
	"context"
	"sort"

	"go.temporal.io/sdk/workflow"

	"github.com/arisros/fate"
)

// Options configures how a WorkflowActor maps invocations and events onto Temporal.
type Options struct {
	ActivityOptions workflow.ActivityOptions
	SignalName      string
}

// WorkflowActor hosts a fate Actor inside a Temporal workflow.
type WorkflowActor[Ctx any, Evt any] struct {
	ctx   workflow.Context
	actor *fate.Actor[Ctx, Evt]
	opts  Options
}

// NewWorkflowActor wraps a fresh actor for the given machine and starts it.
func NewWorkflowActor[Ctx any, Evt any](
	ctx workflow.Context,
	m *fate.Machine[Ctx, Evt],
	opts Options,
) (*WorkflowActor[Ctx, Evt], error) {
	a := fate.NewActor(m)
	if err := a.Start(context.Background()); err != nil {
		return nil, err
	}
	return &WorkflowActor[Ctx, Evt]{ctx: ctx, actor: a, opts: opts}, nil
}

// NewWorkflowActorFromSnapshot resumes an actor from a persisted snapshot — for example after continue-as-new.
func NewWorkflowActorFromSnapshot[Ctx any, Evt any](
	ctx workflow.Context,
	m *fate.Machine[Ctx, Evt],
	snapshot []byte,
	opts Options,
) (*WorkflowActor[Ctx, Evt], error) {
	a, err := fate.NewActorFromSnapshot[Ctx, Evt](m, snapshot)
	if err != nil {
		return nil, err
	}
	return &WorkflowActor[Ctx, Evt]{ctx: ctx, actor: a, opts: opts}, nil
}

// Start runs the actor's initial entry.
func (w *WorkflowActor[Ctx, Evt]) Start() error {
	return w.actor.Start(context.Background())
}

// Stop halts the hosted actor.
func (w *WorkflowActor[Ctx, Evt]) Stop() {
	w.actor.Stop()
}

// Send delivers an event to the hosted actor from workflow code.
func (w *WorkflowActor[Ctx, Evt]) Send(evt Evt) error {
	return w.actor.Send(context.Background(), evt)
}

// Snapshot returns the hosted actor's current snapshot.
func (w *WorkflowActor[Ctx, Evt]) Snapshot() fate.Snapshot[Ctx] { return w.actor.Snapshot() }

// Persist returns the hosted actor's persisted snapshot, e.g. to pass to continue-as-new.
func (w *WorkflowActor[Ctx, Evt]) Persist() ([]byte, error) { return w.actor.Persist() }

type inflight struct {
	future workflow.Future
	cancel workflow.CancelFunc
}

// Run drives the hosted actor until it completes (reaches a top-level final state) or the machine can make no further progress, reconciling Temporal timers and activities against the actor's pending effects after every step and consuming signals if configured.
func (w *WorkflowActor[Ctx, Evt]) Run() (fate.Snapshot[Ctx], error) {
	ctx := w.ctx
	actCtx := workflow.WithActivityOptions(ctx, w.opts.ActivityOptions)

	timers := map[fate.TimerID]inflight{}
	invokes := map[fate.InvokeID]inflight{}

	var signalCh workflow.ReceiveChannel
	if w.opts.SignalName != "" {
		signalCh = workflow.GetSignalChannel(ctx, w.opts.SignalName)
	}

	for w.actor.Snapshot().Status == fate.StatusRunning {
		w.reconcileTimers(ctx, timers)
		w.reconcileInvocations(actCtx, invokes)

		if len(timers) == 0 && len(invokes) == 0 && signalCh == nil {
			break
		}

		sel := workflow.NewSelector(ctx)
		w.addTimerBranches(ctx, sel, timers)
		w.addInvokeBranches(ctx, sel, invokes)
		if signalCh != nil {
			sel.AddReceive(signalCh, func(c workflow.ReceiveChannel, _ bool) {
				var evt Evt
				c.Receive(ctx, &evt)
				_ = w.actor.Send(context.Background(), evt)
			})
		}
		sel.Select(ctx)
	}

	return w.actor.Snapshot(), nil
}

func (w *WorkflowActor[Ctx, Evt]) reconcileTimers(ctx workflow.Context, timers map[fate.TimerID]inflight) {
	pending := w.actor.PendingTimers()
	want := make(map[fate.TimerID]fate.PendingTimer, len(pending))
	for _, pt := range pending {
		want[pt.ID] = pt
	}
	for _, id := range sortedTimerIDs(timers) {
		if _, ok := want[id]; !ok {
			timers[id].cancel()
			delete(timers, id)
		}
	}
	for _, pt := range pending {
		if _, ok := timers[pt.ID]; ok {
			continue
		}
		tctx, cancel := workflow.WithCancel(ctx)
		timers[pt.ID] = inflight{future: workflow.NewTimer(tctx, pt.Delay), cancel: cancel}
	}
}

func (w *WorkflowActor[Ctx, Evt]) reconcileInvocations(actCtx workflow.Context, invokes map[fate.InvokeID]inflight) {
	pending := w.actor.PendingInvocations()
	want := make(map[fate.InvokeID]fate.PendingInvocation, len(pending))
	for _, pi := range pending {
		want[pi.ID] = pi
	}
	for _, id := range sortedInvokeIDs(invokes) {
		if _, ok := want[id]; !ok {
			invokes[id].cancel()
			delete(invokes, id)
		}
	}
	for _, pi := range pending {
		if _, ok := invokes[pi.ID]; ok {
			continue
		}
		ictx, cancel := workflow.WithCancel(actCtx)
		invokes[pi.ID] = inflight{future: workflow.ExecuteActivity(ictx, pi.Src, pi.Input), cancel: cancel}
	}
}

func (w *WorkflowActor[Ctx, Evt]) addTimerBranches(ctx workflow.Context, sel workflow.Selector, timers map[fate.TimerID]inflight) {
	for _, id := range sortedTimerIDs(timers) {
		id := id
		fl := timers[id]
		sel.AddFuture(fl.future, func(f workflow.Future) {
			if err := f.Get(ctx, nil); err == nil {
				w.actor.FireTimer(id)
			}
			delete(timers, id)
		})
	}
}

func (w *WorkflowActor[Ctx, Evt]) addInvokeBranches(ctx workflow.Context, sel workflow.Selector, invokes map[fate.InvokeID]inflight) {
	for _, id := range sortedInvokeIDs(invokes) {
		id := id
		fl := invokes[id]
		sel.AddFuture(fl.future, func(f workflow.Future) {
			var out interface{}
			if err := f.Get(ctx, &out); err != nil {
				w.actor.RejectInvocation(id, err)
			} else {
				w.actor.ResolveInvocation(id, out)
			}
			delete(invokes, id)
		})
	}
}

func sortedTimerIDs(m map[fate.TimerID]inflight) []fate.TimerID {
	ids := make([]fate.TimerID, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func sortedInvokeIDs(m map[fate.InvokeID]inflight) []fate.InvokeID {
	ids := make([]fate.InvokeID, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
