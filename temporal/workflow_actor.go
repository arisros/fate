// Package temporal hosts a fate statechart actor inside a Temporal workflow.
//
// It is a thin, generic adapter over the clock-agnostic fate core: it drives the
// actor's pending effects — delayed transitions and invocations — by mapping
// them onto Temporal primitives inside the single workflow coroutine, and feeds
// the results back through the core's pull API. The fate engine itself stays
// dependency-free; this module is where (and the only place where) the Temporal
// SDK enters.
//
// Mapping:
//
//   - PendingTimers   → workflow.NewTimer; on fire, Actor.FireTimer.
//   - PendingInvocations → workflow.ExecuteActivity (Src is the activity name,
//     Input the activity argument); on completion, Actor.ResolveInvocation, or
//     Actor.RejectInvocation on failure.
//   - external events → a Temporal signal channel; on receive, Actor.Send.
//   - a state exited and entered again → its timers and activities are
//     cancelled and started afresh, read from the actor's step records.
//
// Determinism: every effect is created and selected in a deterministic order
// (fate's pending lists are sorted by ID), and all actor calls happen on the
// workflow goroutine via the selector, so workflow replay reproduces identical
// transitions. The hosted actor must never be driven from another goroutine.
package temporal

import (
	"context"
	"sort"

	"go.temporal.io/sdk/workflow"

	"github.com/arisros/fate/effect"
	"github.com/arisros/fate/engine"
	"github.com/arisros/fate/persist"
)

// Options configures how a WorkflowActor maps invocations and events onto
// Temporal.
type Options struct {
	// ActivityOptions are applied to every invocation activity. At minimum a
	// StartToCloseTimeout (or ScheduleToCloseTimeout) is required by Temporal.
	ActivityOptions workflow.ActivityOptions
	// InvocationOptions, if set, returns the activity options for one
	// invocation, replacing ActivityOptions for it. Use it to give a slow
	// activity its own timeout, retry policy or task queue.
	InvocationOptions func(inv effect.PendingInvocation) workflow.ActivityOptions
	// DecodeResult, if set, reads a completed activity's result into the value
	// the invocation's OnDone receives, so OnDone sees a typed value:
	//
	//	DecodeResult: func(inv effect.PendingInvocation, f workflow.Future) (any, error) {
	//	    var score Score
	//	    err := f.Get(ctx, &score)
	//	    return score, err
	//	}
	//
	// An error rejects the invocation. Left nil, the result is decoded into an
	// interface{}: float64 for numbers, map[string]interface{} for objects.
	DecodeResult func(inv effect.PendingInvocation, f workflow.Future) (any, error)
	// SignalName, if non-empty, is the signal channel the actor consumes
	// external events from. Each signal payload is decoded into Evt and sent to
	// the actor. Leave empty to drive events only via WorkflowActor.Send from
	// workflow code.
	SignalName string
}

// WorkflowActor hosts a fate Actor inside a Temporal workflow. Construct with
// NewWorkflowActor (or NewWorkflowActorFromSnapshot to resume), then call Run to
// drive the machine to completion.
type WorkflowActor[Ctx any, Evt any] struct {
	ctx   workflow.Context
	actor *engine.Actor[Ctx, Evt]
	opts  Options
	// exited holds the states left since the last reconcile, whose in-flight
	// effects must be restarted even when the actor re-armed the same ids.
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

// NewWorkflowActorFromSnapshot resumes an actor from a persisted snapshot — for
// example after continue-as-new. The actor's pending effects are re-derived from
// its restored configuration, so Run re-arms them.
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

// Start runs the actor's initial entry. NewWorkflowActor already starts the
// actor, so Start is idempotent and returns nil when the actor is already
// running. It is provided for callers that drive the actor manually (one Send
// per workflow callback, reading Snapshot between them) and prefer an explicit
// start in their own control flow.
func (w *WorkflowActor[Ctx, Evt]) Start() error {
	return w.actor.Start(context.Background())
}

// Stop halts the hosted actor. A subsequent Send returns an error. Manual
// drivers call this when the workflow's task lifecycle ends.
func (w *WorkflowActor[Ctx, Evt]) Stop() {
	w.actor.Stop()
}

// Send delivers an event to the hosted actor from workflow code. It must be
// called on the workflow goroutine.
func (w *WorkflowActor[Ctx, Evt]) Send(evt Evt) error {
	return w.actor.Send(context.Background(), evt)
}

// Snapshot returns the hosted actor's current snapshot.
func (w *WorkflowActor[Ctx, Evt]) Snapshot() persist.Snapshot[Ctx] { return w.actor.Snapshot() }

// Persist returns the hosted actor's persisted snapshot, e.g. to pass to
// continue-as-new.
func (w *WorkflowActor[Ctx, Evt]) Persist() ([]byte, error) { return w.actor.Persist() }

// inflight holds a started Temporal future together with the cancel func that
// disarms it when the owning state exits.
type inflight struct {
	future workflow.Future
	cancel workflow.CancelFunc
	state  string
	inv    effect.PendingInvocation
}

// Run drives the hosted actor until it completes (reaches a top-level final
// state) or the machine can make no further progress, reconciling Temporal
// timers and activities against the actor's pending effects after every step
// and consuming signals if configured. It returns the final snapshot, or the
// error from delivering a signalled event to the actor.
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

		// Nothing can advance the machine: no pending effects and no signal
		// source. Stop rather than block forever.
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

// reconcileTimers cancels Temporal timers whose state has exited, including one
// that was entered again, and starts a timer for every pending timer that has
// none. Iteration is deterministic.
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
	for _, pt := range pending { // already sorted by ID
		if _, ok := timers[pt.ID]; ok {
			continue
		}
		tctx, cancel := workflow.WithCancel(ctx)
		timers[pt.ID] = inflight{future: workflow.NewTimer(tctx, pt.Delay), cancel: cancel, state: pt.State}
	}
}

// reconcileInvocations cancels activities whose state has exited, including one
// that was entered again, and starts an activity for every pending invocation
// that has none. Iteration is deterministic.
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
	for _, pi := range pending { // already sorted by ID
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

// addTimerBranches registers every in-flight timer with the selector in
// deterministic (sorted) order; on fire it delivers the elapsed delay to the
// actor and removes the timer from the in-flight set.
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

// addInvokeBranches registers every in-flight activity with the selector in
// deterministic (sorted) order; on completion it resolves or rejects the
// invocation and removes it from the in-flight set.
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
