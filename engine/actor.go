package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
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

// NextEvents returns the names of the events the active configuration declares a transition for, sorted.
func (a *Actor[Ctx, Evt]) NextEvents() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.status != persist.StatusRunning {
		return nil
	}
	names := map[string]struct{}{}
	for _, leaf := range resolveLeaves[Ctx, Evt](a.machine.root, a.value) {
		for cursor := leaf; cursor != nil && cursor.name != ""; cursor = cursor.parent {
			for name := range cursor.on {
				if name != "*" {
					names[name] = struct{}{}
				}
			}
		}
	}
	return slices.Sorted(maps.Keys(names))
}

// Enabled returns the names from [Actor.NextEvents] whose event would fire a transition now, guards evaluated, sorted.
func (a *Actor[Ctx, Evt]) Enabled(byName func(name string) (Evt, bool)) []string {
	var enabled []string
	for _, name := range a.NextEvents() {
		if evt, ok := byName(name); ok && a.Can(evt) {
			enabled = append(enabled, name)
		}
	}
	return enabled
}

// Preview returns the snapshot [Actor.Send] would leave behind for evt, without changing the actor.
func (a *Actor[Ctx, Evt]) Preview(evt Evt) (persist.Snapshot[Ctx], error) {
	var zero persist.Snapshot[Ctx]
	trial, err := a.copyForPreview()
	if err != nil {
		return zero, fmt.Errorf("statechart: preview: %w", err)
	}
	if err := trial.Send(context.Background(), evt); err != nil {
		return zero, err
	}
	return trial.Snapshot(), nil
}

func (a *Actor[Ctx, Evt]) copyForPreview() (*Actor[Ctx, Evt], error) {
	if a.machine.clone == nil {
		blob, err := a.Persist()
		if err != nil {
			return nil, err
		}
		return NewActorFromSnapshot[Ctx, Evt](a.machine, blob)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	trial := &Actor[Ctx, Evt]{
		machine:           a.machine,
		value:             cloneValue(a.value),
		ctx:               a.machine.clone(a.ctx),
		status:            a.status,
		output:            a.output,
		errText:           a.errText,
		historyMemory:     maps.Clone(a.historyMemory),
		historyDeepMemory: make(map[*stateNode[Ctx, Evt]]persist.StateValue, len(a.historyDeepMemory)),
		armed:             maps.Clone(a.armed),
		pendingInvokes:    maps.Clone(a.pendingInvokes),
	}
	for node, sub := range a.historyDeepMemory {
		trial.historyDeepMemory[node] = cloneValue(sub)
	}
	trial.queue.Restore(a.queue.Snapshot())
	return trial, nil
}

func cloneValue(v persist.StateValue) persist.StateValue {
	if v.Children == nil {
		return v
	}
	out := persist.StateValue{Leaf: v.Leaf, Children: make(map[string]persist.StateValue, len(v.Children))}
	for name, child := range v.Children {
		out.Children[name] = cloneValue(child)
	}
	return out
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
		leaves := resolveLeaves[Ctx, Evt](a.machine.root, a.value)
		source, target, chosen, ok := a.nextDoneLocked(leaves, triggerEvt)
		if !ok {
			a.completeLocked(leaves)
			return
		}
		a.runTransitionLocked(source, target, chosen, triggerEvt)
	}
	if a.logger != nil {
		a.logger("statechart: onDone settle cap reached; configuration may be ill-formed")
	}
}

func (a *Actor[Ctx, Evt]) nextDoneLocked(
	leaves []*stateNode[Ctx, Evt],
	triggerEvt Evt,
) (source, target *stateNode[Ctx, Evt], chosen TransitionConfig[Ctx, Evt], ok bool) {
	for _, leaf := range leaves {
		if leaf.typ != NodeFinal {
			continue
		}
		for n := leaf.parent; n != nil && n.name != "" && isDone(n, leaves); n = n.parent {
			for _, t := range n.onDone {
				if !transitionPasses(t, a.ctx, triggerEvt, a.value) {
					continue
				}
				if tgt := a.resolveHistoryRedirect(resolveTarget(n, t.Target)); tgt != nil {
					return n, tgt, t, true
				}
				break
			}
		}
	}
	return nil, nil, chosen, false
}

func (a *Actor[Ctx, Evt]) completeLocked(leaves []*stateNode[Ctx, Evt]) {
	if len(leaves) == 0 {
		return
	}
	top := leaves[0]
	for top.parent != nil && top.parent.name != "" {
		top = top.parent
	}
	if top.typ != NodeFinal && !isDone(top, leaves) {
		return
	}
	for _, leaf := range leaves {
		if leaf.typ == NodeFinal && leaf.outputFn != nil {
			a.captureOutputLocked(leaf)
			break
		}
	}
	a.status = persist.StatusDone
}

func isDone[Ctx any, Evt any](n *stateNode[Ctx, Evt], leaves []*stateNode[Ctx, Evt]) bool {
	switch n.typ {
	case NodeCompound:
		for _, leaf := range leaves {
			if leaf.typ == NodeFinal && leaf.parent == n {
				return true
			}
		}
	case NodeParallel:
		for _, region := range n.children {
			if !isDone(region, leaves) {
				return false
			}
		}
		return len(n.children) > 0
	}
	return false
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
