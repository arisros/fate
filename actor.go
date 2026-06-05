package fate

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"

	"github.com/arisros/fate/internal"
)

// maxQueueDrain caps the number of internally-raised events processed in
// a single Send call. Guards against runaway raise loops.
const maxQueueDrain = 1024

// Actor is the runtime instance of a statechart Machine. One Actor is
// instantiated per workflow execution / unit test. NOT safe for use inside
// Temporal workflows — use WorkflowActor (P6) for that.
type Actor[Ctx any, Evt any] struct {
	machine *Machine[Ctx, Evt]

	mu     sync.Mutex
	value  StateValue
	ctx    Ctx
	status ActorStatus
	queue  internal.EventQueue[Evt]
	logger func(string)

	// output holds the machine's final output (JSON) once it reaches a
	// top-level final state that declares an Output function; nil otherwise.
	output json.RawMessage
	// errText holds an error description when status is StatusError.
	errText string

	// historyMemory remembers, for each compound state, the name of the
	// immediate child that was active when the compound was last exited.
	// Used to redirect transitions targeting NodeHistory pseudo-states with
	// History=HistoryShallow.
	historyMemory map[*stateNode[Ctx, Evt]]string

	// historyDeepMemory remembers, for each compound state, the full
	// value-inside subtree active at exit time. Used by HistoryDeep
	// pseudo-states to restore the entire descendant configuration on
	// re-entry. Populated unconditionally on every compound exit (cost is
	// O(saved subtree size) per exit, which is bounded by the configuration
	// depth and trivial in practice).
	historyDeepMemory map[*stateNode[Ctx, Evt]]StateValue

	// pendingDeepSplice carries the saved subtree from resolveHistoryRedirect
	// to runTransitionLocked, which applies it after commitValue. Cleared
	// after each transition. Nil when the current transition is not a deep-
	// history restoration.
	pendingDeepSplice *deepHistorySplice[Ctx, Evt]

	// armed tracks every pending after-timer by ID. The core never fires these
	// itself; it only records them so an adapter can pull them via
	// PendingTimers and drive them via FireTimer, and so they can be cancelled
	// on state exit or actor stop.
	armed map[TimerID]afterBinding[Ctx, Evt]

	// pendingInvokes tracks every armed invocation by ID, for the same
	// effects-as-data reason as armed timers (see invoke.go / ADR-0004).
	pendingInvokes map[InvokeID]invokeBinding[Ctx, Evt]

	subscribers []func(Snapshot[Ctx])
}

// afterBinding records which state and delay bucket an armed timer belongs to,
// so FireTimer can re-select the delay's transitions when the adapter fires it.
type afterBinding[Ctx any, Evt any] struct {
	node  *stateNode[Ctx, Evt]
	entry afterEntry[Ctx, Evt]
}

// deepHistorySplice carries the parent compound and saved subtree from
// resolveHistoryRedirect to the post-commit splice step.
type deepHistorySplice[Ctx any, Evt any] struct {
	parent  *stateNode[Ctx, Evt]
	subtree StateValue
}

// ActorOption configures a new Actor.
type ActorOption func(*actorOpts)

type actorOpts struct {
	initialSnapshot *snapshotRestore
	logger          func(string)
}

type snapshotRestore struct {
	value StateValue
	// context restore added in P6 with persisted snapshot
}

// WithInitialValue overrides the actor's starting state. Used by
// NewActorFromSnapshot (P6) and by tests that need to seed mid-flight.
// The value must be a valid configuration of the machine; this is not
// re-validated in the skeleton.
func WithInitialValue[Ctx any, Evt any](v StateValue) ActorOption {
	return func(o *actorOpts) {
		o.initialSnapshot = &snapshotRestore{value: v}
	}
}

// WithLogger sets the function called by Log actions and internal warnings.
// Default: a no-op (logs are discarded).
func WithLogger(fn func(string)) ActorOption {
	return func(o *actorOpts) { o.logger = fn }
}

// NewActor constructs a fresh Actor in the Stopped status. Call Start to
// transition it to Running and observe the initial entry actions.
func NewActor[Ctx any, Evt any](m *Machine[Ctx, Evt], opts ...ActorOption) *Actor[Ctx, Evt] {
	o := &actorOpts{}
	for _, opt := range opts {
		opt(o)
	}
	a := &Actor[Ctx, Evt]{
		machine:           m,
		ctx:               m.initialContext(),
		status:            StatusStopped,
		logger:            o.logger,
		armed:             map[TimerID]afterBinding[Ctx, Evt]{},
		pendingInvokes:    map[InvokeID]invokeBinding[Ctx, Evt]{},
		historyMemory:     map[*stateNode[Ctx, Evt]]string{},
		historyDeepMemory: map[*stateNode[Ctx, Evt]]StateValue{},
	}
	if o.initialSnapshot != nil {
		a.value = o.initialSnapshot.value
	} else {
		a.value = m.initialValue()
	}
	return a
}

// Start moves the actor into Running and executes entry actions for the
// initial configuration chain (deepest entry's Entry runs last). Idempotent.
// If the initial configuration already lands in a top-level final state,
// the actor immediately transitions to StatusDone.
func (a *Actor[Ctx, Evt]) Start(_ context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.status == StatusRunning {
		return nil
	}
	a.status = StatusRunning

	// Walk the active chain from the root's initial child down into the
	// initial-descendant chain, executing each node's Entry in order and
	// arming any delayed transitions it declares.
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

// Send dispatches an event to the actor synchronously. Returns after the
// event (and any events the transition raised internally) have been
// processed. Events that no transition handles are silently dropped.
//
// If processing the event causes the actor to reach a top-level final
// state, its status transitions to StatusDone. Subsequent Sends are
// silently dropped (matching XState v5 semantics).
func (a *Actor[Ctx, Evt]) Send(_ context.Context, evt Evt) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.status == StatusStopped {
		return ErrActorStopped
	}
	if a.status == StatusDone {
		return nil // silently drop events to a completed actor
	}
	if a.status != StatusRunning {
		return ErrActorNotStarted
	}
	a.handleEventLocked(evt)
	a.drainQueueLocked()
	a.settleFinalLocked(evt)
	a.notifyLocked()
	return nil
}

// Snapshot returns the actor's current state. Safe to call concurrently.
func (a *Actor[Ctx, Evt]) Snapshot() Snapshot[Ctx] {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.snapshotLocked()
}

// Subscribe registers an observer that is called with a snapshot after
// every Send (and once on Start, after entry actions). Returns an
// unsubscribe func.
func (a *Actor[Ctx, Evt]) Subscribe(obs func(Snapshot[Ctx])) func() {
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

// Stop terminates the actor and cancels any pending delayed transitions;
// subsequent Send returns ErrActorStopped.
func (a *Actor[Ctx, Evt]) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status = StatusStopped
	a.cancelAllAfterLocked()
	a.pendingInvokes = map[InvokeID]invokeBinding[Ctx, Evt]{}
}

// handleEventLocked processes a single event: selects transitions (one per
// active region in parallel configurations), computes exit/entry sets, and
// runs actions in the SCXML-defined order. Each selected transition is
// applied in the order returned by selectTransitions (deterministic across
// runs, since leaves are visited in alphabetical path order).
func (a *Actor[Ctx, Evt]) handleEventLocked(evt Evt) {
	eventName := eventNameOf(evt)
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

// runTransitionLocked is the SCXML transition apply step factored out so
// settleFinalLocked (and future internal transition sources) can reuse it.
func (a *Actor[Ctx, Evt]) runTransitionLocked(
	source, target *stateNode[Ctx, Evt],
	t TransitionConfig[Ctx, Evt],
	evt Evt,
) {
	exit := computeExitSet[Ctx, Evt](a.machine.root, a.value, source, target, t.Internal)
	entry := computeEntrySet[Ctx, Evt](source, target, t.Internal)

	// 1) Record history for any compound about to exit, then run exit
	//    actions deepest first, and cancel that state's pending after-timers.
	for _, n := range exit {
		if n.typ == NodeCompound {
			a.recordHistoryLocked(n)
		}
		a.runActions(n.exit(), evt)
		a.cancelAfterLocked(n)
		a.cancelInvokesLocked(n)
	}
	// 2) Transition actions.
	a.runActions(t.Actions, evt)
	// 3) Entry actions, outermost first, arming each entered state's delayed
	//    transitions and invocations.
	for _, n := range entry {
		a.runActions(n.entry(), evt)
		a.armAfterLocked(n)
		a.armInvokesLocked(n)
	}
	// 4) Commit the new value, preserving parallel-region siblings.
	a.value = commitValue[Ctx, Evt](a.machine.root, a.value, target)

	// 5) Deep-history restoration: if resolveHistoryRedirect saw a deep
	//    pseudo-state with saved memory, splice the saved subtree under the
	//    parent compound. commitValue would otherwise have re-expanded the
	//    parent via its initial chain.
	if a.pendingDeepSplice != nil {
		a.value = spliceValueAt[Ctx, Evt](
			a.machine.root, a.value,
			a.pendingDeepSplice.parent, a.pendingDeepSplice.subtree,
		)
		a.pendingDeepSplice = nil
	}
}

// resolveHistoryRedirect translates a history pseudo-state into a real node.
//
// For HistoryShallow:
//   - If memory exists for the parent compound, return the remembered
//     immediate child. Entry then proceeds via that child's normal initial
//     chain.
//
// For HistoryDeep:
//   - If memory exists for the parent compound (full subtree), choose the
//     subtree's deepest active leaf as the entry target so exit/entry sets
//     compute correctly, and stash the saved subtree on pendingDeepSplice
//     for runTransitionLocked to apply after commitValue. This restores the
//     entire saved descendant configuration, not just the immediate child.
//
// Common fallbacks (apply to both depths when memory is absent):
//   - Use the history node's Default target.
//   - Otherwise, use the parent's Initial child.
//   - Otherwise, return nil and the transition is silently aborted.
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
			// Resolve the saved subtree's deepest leaf, evaluated as a
			// value-inside `parent`. resolveLeaves walks a StateValue
			// against a parent node — pass `parent` as the walk's parent.
			leaves := resolveLeaves[Ctx, Evt](parent, sub)
			if len(leaves) > 0 {
				a.pendingDeepSplice = &deepHistorySplice[Ctx, Evt]{
					parent:  parent,
					subtree: sub,
				}
				return leaves[0]
			}
		}
		// Fall through to defaults below when no deep memory exists yet.
	} else if memChild, ok := a.historyMemory[parent]; ok {
		if real, ok := parent.children[memChild]; ok {
			return real
		}
	}
	if target.defaultTgt != "" {
		if def := resolveTarget(target, target.defaultTgt); def != nil {
			// Defensive: if the default itself is a history node, fall
			// back to parent.initial to avoid loops.
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

// recordHistoryLocked snapshots two views of the active subtree under
// `parent` into the history memory maps, so a later transition to a history
// pseudo-state can restore it:
//
//   - Shallow: the local name of the immediate child on the active path.
//   - Deep:   the full value-inside `parent` (preserves nested compound
//     and parallel-region configurations).
//
// Both are saved unconditionally on every compound exit so that switching
// a NodeHistory's History flag from Shallow to Deep (or vice versa) without
// otherwise altering the machine still works deterministically.
func (a *Actor[Ctx, Evt]) recordHistoryLocked(parent *stateNode[Ctx, Evt]) {
	leaf := resolveLeaf[Ctx, Evt](a.machine.root, a.value)
	if leaf == nil {
		return
	}
	for cursor := leaf; cursor != nil; cursor = cursor.parent {
		if cursor.parent == parent {
			a.historyMemory[parent] = cursor.name
			break
		}
	}
	if sub, ok := extractValueAt[Ctx, Evt](a.machine.root, a.value, parent); ok {
		a.historyDeepMemory[parent] = sub
	}
}

// settleFinalLocked propagates final-state completion upward through the
// hierarchy. After each transition (or initial entry) we may land in a
// final leaf; the enclosing compound's onDone (if any) fires immediately,
// which may transition the actor to another state, which may also land
// in a final leaf, and so on.
//
// When a final state is reached at the top level (parent == synthetic root)
// and no further onDone consumes it, the actor's status becomes
// StatusDone.
//
// The bounded loop guards against ill-formed configurations that could
// otherwise loop forever (e.g. onDone targeting a final state of the same
// parent).
func (a *Actor[Ctx, Evt]) settleFinalLocked(triggerEvt Evt) {
	for i := 0; i < maxQueueDrain; i++ {
		leaf := resolveLeaf[Ctx, Evt](a.machine.root, a.value)
		if leaf == nil || leaf.typ != NodeFinal {
			return
		}
		parent := leaf.parent
		if parent == nil || parent.name == "" {
			// Reached a final state at the top level — actor is done.
			a.captureOutputLocked(leaf)
			a.status = StatusDone
			return
		}
		// Evaluate parent.onDone candidates.
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
			// No onDone wired; this region is permanently done but the
			// surrounding configuration carries on. Set status only when
			// it's the top-level child that completed.
			if parent.parent == nil || parent.parent.name == "" {
				a.captureOutputLocked(leaf)
				a.status = StatusDone
			}
			return
		}
		// Run the onDone transition through the shared apply path so
		// history recording and any future SCXML-related concerns stay
		// in one place.
		target := resolveTarget(parent, chosen.Target)
		if target == nil {
			// Validated at construction time, so unreachable.
			return
		}
		target = a.resolveHistoryRedirect(target)
		if target == nil {
			return
		}
		a.runTransitionLocked(parent, target, chosen, triggerEvt)
		// Loop: the new value may itself land in a final state.
	}
	if a.logger != nil {
		a.logger("statechart: onDone settle cap reached; configuration may be ill-formed")
	}
}

// drainQueueLocked processes raised events until the queue is empty or the
// drain cap is reached. The cap prevents an infinite Raise loop from
// hanging the actor.
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

// runActions evaluates a slice of actions against the current context and
// event. Each action may update context and/or queue events via the sink.
func (a *Actor[Ctx, Evt]) runActions(actions []Action[Ctx, Evt], evt Evt) {
	if len(actions) == 0 {
		return
	}
	sink := actorSink[Ctx, Evt]{a: a}
	for _, act := range actions {
		if act == nil {
			continue
		}
		a.ctx = act.apply(a.ctx, evt, sink)
	}
}

func (a *Actor[Ctx, Evt]) snapshotLocked() Snapshot[Ctx] {
	return Snapshot[Ctx]{
		Version: SnapshotVersion,
		Value:   a.value,
		Context: a.ctx,
		Status:  a.status,
		Output:  a.output,
		Error:   a.errText,
	}
}

// captureOutputLocked records the machine output from a completing top-level
// final state, if it declares an Output function. A marshal failure is recorded
// as the actor's error text rather than surfaced (the actor still completes).
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

// actorSink implements actionSink by routing into the actor's queue + logger.
type actorSink[Ctx any, Evt any] struct {
	a *Actor[Ctx, Evt]
}

func (s actorSink[Ctx, Evt]) raise(e Evt) {
	s.a.queue.Push(e)
}

func (s actorSink[Ctx, Evt]) log(msg string) {
	if s.a.logger != nil {
		s.a.logger(msg)
	}
}

// initialEntryChain returns the ordered list of nodes that are "entered" when
// the actor starts — the full initial configuration, outermost first. Compound
// nodes descend into their initial child; parallel nodes descend into every
// region (visited in sorted order for determinism). The synthetic root is
// excluded. This must match the active configuration that NewActorFromSnapshot
// re-derives, so Start and restore arm the same entry effects.
func initialEntryChain[Ctx any, Evt any](root *stateNode[Ctx, Evt]) []*stateNode[Ctx, Evt] {
	var chain []*stateNode[Ctx, Evt]
	if init := root.children[root.initial]; init != nil {
		appendInitialEntry[Ctx, Evt](init, &chain)
	}
	return chain
}

// appendInitialEntry appends n and its initial-descendant configuration to out,
// outermost first, descending through compound initials and all parallel
// regions.
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

// entry returns the node's entry actions from the underlying config. The
// stateNode struct doesn't store actions directly (kept lean); they live
// alongside the on-event map. For P4 we store them on the node.
func (n *stateNode[Ctx, Evt]) entry() []Action[Ctx, Evt] { return n.entryActions }

// exit returns the node's exit actions.
func (n *stateNode[Ctx, Evt]) exit() []Action[Ctx, Evt] { return n.exitActions }

// eventNameOf extracts a string tag for an event. The convention is:
//
//  1. If Evt is a string (or string-typed), it is the name directly.
//  2. If Evt has an EventName() method, that is used.
//  3. Otherwise, reflection takes the concrete struct type's name and
//     strips conventional suffixes ("T", "Event") used by codegen.
//
// Codegen-emitted typed events (per ADR-006) implement EventName() so they
// don't pay the reflection cost.
func eventNameOf(evt any) string {
	if s, ok := evt.(string); ok {
		return s
	}
	if named, ok := evt.(interface{ EventName() string }); ok {
		return named.EventName()
	}
	t := reflect.TypeOf(evt)
	if t == nil {
		return ""
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	name := t.Name()
	for _, suffix := range []string{"T", "Event"} {
		if len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix {
			return name[:len(name)-len(suffix)]
		}
	}
	return name
}
