package engine

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/arisros/fate/action"
	"github.com/arisros/fate/effect"
	"github.com/arisros/fate/internal"
	"github.com/arisros/fate/persist"
)

const maxQueueDrain = 1024

// Actor is the runtime instance of a statechart Machine.
type Actor[Ctx any, Evt any] struct {
	machine *Machine[Ctx, Evt]

	mu     sync.Mutex
	value  persist.StateValue
	ctx    Ctx
	status persist.ActorStatus
	queue  internal.EventQueue[Evt]
	logger func(string)

	output  json.RawMessage
	errText string

	historyMemory map[*stateNode[Ctx, Evt]]string

	historyDeepMemory map[*stateNode[Ctx, Evt]]persist.StateValue

	pendingDeepSplice *deepHistorySplice[Ctx, Evt]

	armed map[effect.TimerID]afterBinding[Ctx, Evt]

	pendingInvokes map[effect.InvokeID]invokeBinding[Ctx, Evt]

	subscribers []func(persist.Snapshot[Ctx])
}

type afterBinding[Ctx any, Evt any] struct {
	node  *stateNode[Ctx, Evt]
	entry afterEntry[Ctx, Evt]
}

type deepHistorySplice[Ctx any, Evt any] struct {
	parent  *stateNode[Ctx, Evt]
	subtree persist.StateValue
}

// ActorOption configures a new Actor.
type ActorOption func(*actorOpts)

type actorOpts struct {
	initialSnapshot *snapshotRestore
	logger          func(string)
}

type snapshotRestore struct {
	value persist.StateValue
}

// WithInitialValue overrides the actor's starting state.
func WithInitialValue[Ctx any, Evt any](v persist.StateValue) ActorOption {
	return func(o *actorOpts) {
		o.initialSnapshot = &snapshotRestore{value: v}
	}
}

// WithLogger sets the function called by Log actions and internal warnings.
func WithLogger(fn func(string)) ActorOption {
	return func(o *actorOpts) { o.logger = fn }
}

// NewActor constructs a fresh Actor in the Stopped status.
func NewActor[Ctx any, Evt any](m *Machine[Ctx, Evt], opts ...ActorOption) *Actor[Ctx, Evt] {
	o := &actorOpts{}
	for _, opt := range opts {
		opt(o)
	}
	a := &Actor[Ctx, Evt]{
		machine:           m,
		ctx:               m.initialContext(),
		status:            persist.StatusStopped,
		logger:            o.logger,
		armed:             map[effect.TimerID]afterBinding[Ctx, Evt]{},
		pendingInvokes:    map[effect.InvokeID]invokeBinding[Ctx, Evt]{},
		historyMemory:     map[*stateNode[Ctx, Evt]]string{},
		historyDeepMemory: map[*stateNode[Ctx, Evt]]persist.StateValue{},
	}
	if o.initialSnapshot != nil {
		a.value = o.initialSnapshot.value
	} else {
		a.value = m.initialValue()
	}
	return a
}

// Start moves the actor into Running and executes entry actions for the initial configuration chain (deepest entry's Entry runs last).
func (a *Actor[Ctx, Evt]) Start(_ context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.status == persist.StatusRunning {
		return nil
	}
	a.status = persist.StatusRunning

	var zeroEvt Evt
	for _, node := range initialEntryChain[Ctx, Evt](a.machine.root) {
		a.runActions(node.entry(), zeroEvt)
		a.armAfterLocked(node)
		a.armInvokesLocked(node)
	}
	a.drainQueueLocked()
	a.settleFinalLocked(zeroEvt)
	a.notifyLocked()
	return nil
}

// Send dispatches an event to the actor synchronously.
func (a *Actor[Ctx, Evt]) Send(_ context.Context, evt Evt) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.status == persist.StatusStopped {
		return ErrActorStopped
	}
	if a.status == persist.StatusDone {
		return nil
	}
	if a.status != persist.StatusRunning {
		return ErrActorNotStarted
	}
	a.handleEventLocked(evt)
	a.drainQueueLocked()
	a.settleFinalLocked(evt)
	a.notifyLocked()
	return nil
}

// Can reports whether evt would be handled by the current configuration: that is, whether at least one transition selects for it once guards are evaluated against the current context.
func (a *Actor[Ctx, Evt]) Can(evt Evt) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.status != persist.StatusRunning {
		return false
	}
	selections := selectTransitions[Ctx, Evt](a.machine.root, a.value, a.ctx, evt, internal.EventName(evt))
	return len(selections) > 0
}

// Snapshot returns the actor's current state.
func (a *Actor[Ctx, Evt]) Snapshot() persist.Snapshot[Ctx] {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.snapshotLocked()
}

// Subscribe registers an observer that is called with a snapshot after every Send (and once on Start, after entry actions).
func (a *Actor[Ctx, Evt]) Subscribe(obs func(persist.Snapshot[Ctx])) func() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.subscribers = append(a.subscribers, obs)
	idx := len(a.subscribers) - 1
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if idx < len(a.subscribers) {
			a.subscribers[idx] = nil
		}
	}
}

// Stop terminates the actor and cancels any pending delayed transitions; subsequent Send returns ErrActorStopped.
func (a *Actor[Ctx, Evt]) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status = persist.StatusStopped
	a.cancelAllAfterLocked()
	a.pendingInvokes = map[effect.InvokeID]invokeBinding[Ctx, Evt]{}
}

func (a *Actor[Ctx, Evt]) handleEventLocked(evt Evt) {
	eventName := internal.EventName(evt)
	selections := selectTransitions[Ctx, Evt](a.machine.root, a.value, a.ctx, evt, eventName)
	for _, sel := range selections {
		t := sel.Config
		if t.Target == "" {
			a.runActions(t.Actions, evt)
			continue
		}
		target := resolveTarget(sel.Source, t.Target)
		if target == nil {
			continue
		}
		target = a.resolveHistoryRedirect(target)
		if target == nil {
			continue
		}
		a.runTransitionLocked(sel.Source, target, t, evt)
	}
}

func (a *Actor[Ctx, Evt]) runTransitionLocked(
	source, target *stateNode[Ctx, Evt],
	t TransitionConfig[Ctx, Evt],
	evt Evt,
) {
	exit := computeExitSet[Ctx, Evt](a.machine.root, a.value, source, target, t.Internal)
	entry := computeEntrySet[Ctx, Evt](source, target, t.Internal, a.pendingDeepSplice)

	for _, n := range exit {
		if n.typ == NodeCompound {
			a.recordHistoryLocked(n)
		}
		a.runActions(n.exit(), evt)
		a.cancelAfterLocked(n)
		a.cancelInvokesLocked(n)
	}
	a.runActions(t.Actions, evt)
	for _, n := range entry {
		a.runActions(n.entry(), evt)
		a.armAfterLocked(n)
		a.armInvokesLocked(n)
	}
	a.value = commitValue[Ctx, Evt](a.machine.root, a.value, target)

	if a.pendingDeepSplice != nil {
		a.value = spliceValueAt[Ctx, Evt](
			a.machine.root, a.value,
			a.pendingDeepSplice.parent, a.pendingDeepSplice.subtree,
		)
		a.pendingDeepSplice = nil
	}
}

func (a *Actor[Ctx, Evt]) resolveHistoryRedirect(target *stateNode[Ctx, Evt]) *stateNode[Ctx, Evt] {
	if target == nil || target.typ != NodeHistory {
		return target
	}
	parent := target.parent
	if parent == nil {
		return nil
	}
	if target.history == HistoryDeep {
		if sub, ok := a.historyDeepMemory[parent]; ok {
			leaves := resolveLeaves[Ctx, Evt](parent, sub)
			if len(leaves) > 0 {
				a.pendingDeepSplice = &deepHistorySplice[Ctx, Evt]{
					parent:  parent,
					subtree: sub,
				}
				return leaves[0]
			}
		}
	} else if memChild, ok := a.historyMemory[parent]; ok {
		if real, ok := parent.children[memChild]; ok {
			return real
		}
	}
	if target.defaultTgt != "" {
		if def := resolveTarget(target, target.defaultTgt); def != nil {
			if def.typ != NodeHistory {
				return def
			}
		}
	}
	if init, ok := parent.children[parent.initial]; ok {
		return init
	}
	return nil
}

func (a *Actor[Ctx, Evt]) recordHistoryLocked(parent *stateNode[Ctx, Evt]) {
	// Every leaf: under parallel regions the first may not be under parent.
	for _, leaf := range resolveLeaves[Ctx, Evt](a.machine.root, a.value) {
		if !isDescendant(leaf, parent) {
			continue
		}
		for cursor := leaf; cursor != nil; cursor = cursor.parent {
			if cursor.parent == parent {
				a.historyMemory[parent] = cursor.name
				break
			}
		}
		break
	}
	if sub, ok := extractValueAt[Ctx, Evt](a.machine.root, a.value, parent); ok {
		a.historyDeepMemory[parent] = sub
	}
}

// Bounded so a cyclic onDone cannot loop forever.
func (a *Actor[Ctx, Evt]) settleFinalLocked(triggerEvt Evt) {
	for i := 0; i < maxQueueDrain; i++ {
		leaf := resolveLeaf[Ctx, Evt](a.machine.root, a.value)
		if leaf == nil || leaf.typ != NodeFinal {
			return
		}
		parent := leaf.parent
		if parent == nil || parent.name == "" {
			a.captureOutputLocked(leaf)
			a.status = persist.StatusDone
			return
		}
		var chosen TransitionConfig[Ctx, Evt]
		matched := false
		for _, t := range parent.onDone {
			if transitionPasses(t, a.ctx, triggerEvt, a.value) {
				chosen = t
				matched = true
				break
			}
		}
		if !matched {
			if parent.parent == nil || parent.parent.name == "" {
				a.captureOutputLocked(leaf)
				a.status = persist.StatusDone
			}
			return
		}
		target := resolveTarget(parent, chosen.Target)
		if target == nil {
			return
		}
		target = a.resolveHistoryRedirect(target)
		if target == nil {
			return
		}
		a.runTransitionLocked(parent, target, chosen, triggerEvt)
	}
	if a.logger != nil {
		a.logger("statechart: onDone settle cap reached; configuration may be ill-formed")
	}
}

func (a *Actor[Ctx, Evt]) drainQueueLocked() {
	for i := 0; i < maxQueueDrain; i++ {
		evt, ok := a.queue.Pop()
		if !ok {
			return
		}
		a.handleEventLocked(evt)
	}
	if a.logger != nil {
		a.logger("statechart: queue drain cap reached; events dropped")
	}
}

func (a *Actor[Ctx, Evt]) runActions(actions []action.Action[Ctx, Evt], evt Evt) {
	if len(actions) == 0 {
		return
	}
	sink := actorSink[Ctx, Evt]{a: a}
	for _, act := range actions {
		if act == nil {
			continue
		}
		a.ctx = act.Apply(a.ctx, evt, sink)
	}
}

func (a *Actor[Ctx, Evt]) snapshotLocked() persist.Snapshot[Ctx] {
	return persist.Snapshot[Ctx]{
		Version: persist.SnapshotVersion,
		Value:   a.value,
		Context: a.ctx,
		Status:  a.status,
		Output:  a.output,
		Error:   a.errText,
	}
}

func (a *Actor[Ctx, Evt]) captureOutputLocked(finalLeaf *stateNode[Ctx, Evt]) {
	if finalLeaf == nil || finalLeaf.outputFn == nil {
		return
	}
	raw, err := json.Marshal(finalLeaf.outputFn(a.ctx))
	if err != nil {
		a.errText = "fate: marshal final output: " + err.Error()
		return
	}
	a.output = raw
}

func (a *Actor[Ctx, Evt]) notifyLocked() {
	snap := a.snapshotLocked()
	for _, obs := range a.subscribers {
		if obs != nil {
			obs(snap)
		}
	}
}

type actorSink[Ctx any, Evt any] struct {
	a *Actor[Ctx, Evt]
}

func (s actorSink[Ctx, Evt]) Raise(e Evt) {
	s.a.queue.Push(e)
}

func (s actorSink[Ctx, Evt]) Log(msg string) {
	if s.a.logger != nil {
		s.a.logger(msg)
	}
}

func initialEntryChain[Ctx any, Evt any](root *stateNode[Ctx, Evt]) []*stateNode[Ctx, Evt] {
	var chain []*stateNode[Ctx, Evt]
	if init := root.children[root.initial]; init != nil {
		appendInitialEntry[Ctx, Evt](init, &chain)
	}
	return chain
}

func appendInitialEntry[Ctx any, Evt any](n *stateNode[Ctx, Evt], out *[]*stateNode[Ctx, Evt]) {
	*out = append(*out, n)
	switch n.typ {
	case NodeCompound:
		if init := n.children[n.initial]; init != nil {
			appendInitialEntry[Ctx, Evt](init, out)
		}
	case NodeParallel:
		names := make([]string, 0, len(n.children))
		for name := range n.children {
			names = append(names, name)
		}
		sortStrings(names)
		for _, name := range names {
			appendInitialEntry[Ctx, Evt](n.children[name], out)
		}
	}
}

func (n *stateNode[Ctx, Evt]) entry() []action.Action[Ctx, Evt] { return n.entryActions }

func (n *stateNode[Ctx, Evt]) exit() []action.Action[Ctx, Evt] { return n.exitActions }
