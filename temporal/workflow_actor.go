// Package temporal hosts a fate statechart actor inside a Temporal workflow.
package temporal

import (
	"context"
	"sort"

	"go.temporal.io/sdk/workflow"

	"github.com/arisros/fate/effect"
	"github.com/arisros/fate/engine"
	"github.com/arisros/fate/persist"
)

// Options configures how a WorkflowActor maps invocations and events onto Temporal.
type Options struct {
	ActivityOptions   workflow.ActivityOptions
	InvocationOptions func(inv effect.PendingInvocation) workflow.ActivityOptions
	DecodeResult      func(inv effect.PendingInvocation, f workflow.Future) (any, error)
	SignalName        string
}

// WorkflowActor hosts a fate Actor inside a Temporal workflow.
type WorkflowActor[Ctx any, Evt any] struct {
	ctx    workflow.Context
	actor  *engine.Actor[Ctx, Evt]
	opts   Options
	exited map[string]struct{}
}

func host[Ctx any, Evt any](ctx workflow.Context, a *engine.Actor[Ctx, Evt], opts Options) *WorkflowActor[Ctx, Evt] {
	w := &WorkflowActor[Ctx, Evt]{ctx: ctx, actor: a, opts: opts, exited: map[string]struct{}{}}
	a.SubscribeSteps(func(s engine.Step) {
		for _, state := range s.Exited {
			w.exited[state] = struct{}{}
		}
	})
	return w
}

// NewWorkflowActor wraps a fresh actor for the given machine and starts it.
func NewWorkflowActor[Ctx any, Evt any](
	ctx workflow.Context,
	m *engine.Machine[Ctx, Evt],
	opts Options,
) (*WorkflowActor[Ctx, Evt], error) {
	a := engine.NewActor(m)
	if err := a.Start(context.Background()); err != nil {
		return nil, err
	}
	return host(ctx, a, opts), nil
}

// NewWorkflowActorFromSnapshot resumes an actor from a persisted snapshot — for example after continue-as-new.
func NewWorkflowActorFromSnapshot[Ctx any, Evt any](
	ctx workflow.Context,
	m *engine.Machine[Ctx, Evt],
	snapshot []byte,
	opts Options,
) (*WorkflowActor[Ctx, Evt], error) {
	a, err := engine.NewActorFromSnapshot[Ctx, Evt](m, snapshot)
	if err != nil {
		return nil, err
	}
	return host(ctx, a, opts), nil
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
func (w *WorkflowActor[Ctx, Evt]) Snapshot() persist.Snapshot[Ctx] { return w.actor.Snapshot() }

// Persist returns the hosted actor's persisted snapshot, e.g. to pass to continue-as-new.
func (w *WorkflowActor[Ctx, Evt]) Persist() ([]byte, error) { return w.actor.Persist() }

type inflight struct {
	future workflow.Future
	cancel workflow.CancelFunc
	state  string
	inv    effect.PendingInvocation
}

// Run drives the hosted actor until it completes (reaches a top-level final state) or the machine can make no further progress, reconciling Temporal timers and activities against the actor's pending effects after every step and consuming signals if configured.
func (w *WorkflowActor[Ctx, Evt]) Run() (persist.Snapshot[Ctx], error) {
	ctx := w.ctx

	timers := map[effect.TimerID]inflight{}
	invokes := map[effect.InvokeID]inflight{}

	var signalCh workflow.ReceiveChannel
	if w.opts.SignalName != "" {
		signalCh = workflow.GetSignalChannel(ctx, w.opts.SignalName)
	}

	for w.actor.Snapshot().Status == persist.StatusRunning {
		w.reconcileTimers(ctx, timers)
		w.reconcileInvocations(ctx, invokes)
		clear(w.exited)

		if len(timers) == 0 && len(invokes) == 0 && signalCh == nil {
			break
		}

		var sendErr error
		sel := workflow.NewSelector(ctx)
		w.addTimerBranches(ctx, sel, timers)
		w.addInvokeBranches(ctx, sel, invokes)
		if signalCh != nil {
			sel.AddReceive(signalCh, func(c workflow.ReceiveChannel, _ bool) {
				var evt Evt
				c.Receive(ctx, &evt)
				sendErr = w.actor.Send(context.Background(), evt)
			})
		}
		sel.Select(ctx)
		if sendErr != nil {
			return w.actor.Snapshot(), sendErr
		}
	}

	return w.actor.Snapshot(), nil
}

func (w *WorkflowActor[Ctx, Evt]) reconcileTimers(ctx workflow.Context, timers map[effect.TimerID]inflight) {
	pending := w.actor.PendingTimers()
	want := make(map[effect.TimerID]effect.PendingTimer, len(pending))
	for _, pt := range pending {
		want[pt.ID] = pt
	}
	for _, id := range sortedTimerIDs(timers) {
		_, wanted := want[id]
		if _, left := w.exited[timers[id].state]; left || !wanted {
			timers[id].cancel()
			delete(timers, id)
		}
	}
	for _, pt := range pending {
		if _, ok := timers[pt.ID]; ok {
			continue
		}
		tctx, cancel := workflow.WithCancel(ctx)
		timers[pt.ID] = inflight{future: workflow.NewTimer(tctx, pt.Delay), cancel: cancel, state: pt.State}
	}
}

func (w *WorkflowActor[Ctx, Evt]) reconcileInvocations(ctx workflow.Context, invokes map[effect.InvokeID]inflight) {
	pending := w.actor.PendingInvocations()
	want := make(map[effect.InvokeID]effect.PendingInvocation, len(pending))
	for _, pi := range pending {
		want[pi.ID] = pi
	}
	for _, id := range sortedInvokeIDs(invokes) {
		_, wanted := want[id]
		if _, left := w.exited[invokes[id].state]; left || !wanted {
			invokes[id].cancel()
			delete(invokes, id)
		}
	}
	for _, pi := range pending {
		if _, ok := invokes[pi.ID]; ok {
			continue
		}
		ao := w.opts.ActivityOptions
		if w.opts.InvocationOptions != nil {
			ao = w.opts.InvocationOptions(pi)
		}
		ictx, cancel := workflow.WithCancel(workflow.WithActivityOptions(ctx, ao))
		invokes[pi.ID] = inflight{
			future: workflow.ExecuteActivity(ictx, pi.Src, pi.Input),
			cancel: cancel, state: pi.State, inv: pi,
		}
	}
}

func (w *WorkflowActor[Ctx, Evt]) addTimerBranches(ctx workflow.Context, sel workflow.Selector, timers map[effect.TimerID]inflight) {
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

func (w *WorkflowActor[Ctx, Evt]) addInvokeBranches(ctx workflow.Context, sel workflow.Selector, invokes map[effect.InvokeID]inflight) {
	for _, id := range sortedInvokeIDs(invokes) {
		id := id
		fl := invokes[id]
		sel.AddFuture(fl.future, func(f workflow.Future) {
			var out any
			var err error
			if w.opts.DecodeResult != nil {
				out, err = w.opts.DecodeResult(fl.inv, f)
			} else {
				err = f.Get(ctx, &out)
			}
			if err != nil {
				w.actor.RejectInvocation(id, err)
			} else {
				w.actor.ResolveInvocation(id, out)
			}
			delete(invokes, id)
		})
	}
}

func sortedTimerIDs(m map[effect.TimerID]inflight) []effect.TimerID {
	ids := make([]effect.TimerID, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func sortedInvokeIDs(m map[effect.InvokeID]inflight) []effect.InvokeID {
	ids := make([]effect.InvokeID, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
