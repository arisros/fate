package fate

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// MachineConfig declares an immutable statechart. Pass to CreateMachine to
// validate and obtain a *Machine.
//
// Generics: Ctx is the user's context (data accumulated as the machine runs);
// Evt is the user's event type (typically a sealed interface).
type MachineConfig[Ctx any, Evt any] struct {
	// ID is a human-readable identifier used in inspection output and as the
	// stable prefix for spawn IDs (per ADR-002).
	ID string

	// Initial is the starting child state name. Required.
	Initial string

	// Context is the seed value for the actor's running context.
	Context Ctx

	// States is the map of immediate child state nodes. Keys are local state
	// names (e.g. "idle"); values describe each node.
	States map[string]StateNodeConfig[Ctx, Evt]
}

// StateNodeConfig declares one state node within a machine. State nodes
// nest via the States field to form compound hierarchies.
type StateNodeConfig[Ctx any, Evt any] struct {
	// Type is the node kind. If zero, it is inferred: NodeAtomic when States
	// is empty; NodeCompound otherwise.
	Type NodeType

	// Initial is the starting child state name. Required when Type is
	// NodeCompound and States is non-empty.
	Initial string

	// States declares immediate child state nodes (compound nesting).
	States map[string]StateNodeConfig[Ctx, Evt]

	// On maps event names to ordered transition candidates. The first
	// candidate whose guard passes (or has no guard) is selected.
	On map[string][]TransitionConfig[Ctx, Evt]

	// After declares delayed transitions, keyed by delay. When this state is
	// entered the actor records one pending timer per delay; exiting the state
	// disarms them. The core never fires a timer itself (it is clock-agnostic):
	// an adapter discovers armed timers via Actor.PendingTimers and delivers an
	// elapsed delay via Actor.FireTimer, at which point the first transition in
	// that delay's slice whose Guard and Cond pass fires (as an internal step
	// with the zero Evt). Mirrors XState's `after`. See ADR-0003.
	After map[time.Duration][]TransitionConfig[Ctx, Evt]

	// Invoke declares external work run while this state is active (XState's
	// invoke). On entry each invocation is recorded as pending; on exit it is
	// disarmed. The core never executes an invocation — an adapter pulls them
	// via Actor.PendingInvocations and reports outcomes via
	// Actor.ResolveInvocation / Actor.RejectInvocation. See ADR-0004.
	Invoke []Invocation[Ctx, Evt]

	// Entry actions run, in declaration order, when this state is entered.
	// For a compound node, Entry runs before the child's Entry.
	Entry []Action[Ctx, Evt]

	// Exit actions run, in declaration order, when this state is exited.
	// For a compound node, Exit runs after the child's Exit (deepest first).
	Exit []Action[Ctx, Evt]

	// OnDone declares transitions to fire when this compound node's active
	// child reaches a final state. Only meaningful for Type=NodeCompound
	// (or NodeParallel in P5 follow-up). Empty for atomic / final nodes.
	OnDone []TransitionConfig[Ctx, Evt]

	// History selects HistoryShallow or HistoryDeep when Type is NodeHistory.
	// Ignored for other node types.
	History History

	// Default is the fallback target for a NodeHistory pseudo-state when
	// no prior memory exists. Optional; if empty, the parent compound's
	// initial child is used.
	Default string

	// Output, set only on a NodeFinal state, builds the machine's output value
	// from the final context when a top-level final state is reached. The
	// result is JSON-marshaled into the snapshot's Output field. Mirrors
	// XState's final-state output.
	Output func(ctx Ctx) any
}

// TransitionConfig declares one possible transition for an event.
type TransitionConfig[Ctx any, Evt any] struct {
	// Target is the destination state, named by its local name (sibling)
	// or by a dot-separated descendant path (e.g. "parent.child").
	// An empty Target means the transition is internal (no state change).
	Target string

	// Internal, when true, suppresses exit/re-entry of the source state for
	// targets that are descendants of the source (matches XState's
	// `internal: true`). Default false (external transition).
	Internal bool

	// Guard, if non-nil, must return true for the transition to be selected.
	// Otherwise the next candidate in the slice is tried, then ancestors are
	// consulted. A Guard is a pure predicate over context and event.
	Guard Guard[Ctx, Evt]

	// Cond, if non-nil, is a structural condition over the active state
	// configuration (see Cond / StateIn / InState). When both Guard and Cond
	// are set, the transition is selected only if both pass. Use Cond for
	// "in state X" checks that a context/event Guard cannot express.
	Cond Cond

	// Actions run after exit actions and before entry actions when the
	// transition fires. Order: declaration order.
	Actions []Action[Ctx, Evt]
}

// Machine is an immutable, validated statechart. Safe to share across
// goroutines and across multiple Actor instances. Construct via
// CreateMachine; never mutate.
type Machine[Ctx any, Evt any] struct {
	id      string
	context Ctx
	root    *stateNode[Ctx, Evt]
}

// stateNode is the post-validation in-memory representation. It mirrors
// StateNodeConfig but adds resolved pointers (parent, child lookup) and
// uses the local name as a field.
type stateNode[Ctx any, Evt any] struct {
	name         string   // local name (empty for the synthetic root)
	path         []string // dot-separated path from root, excluding root
	typ          NodeType // Atomic, Compound, Final, or History
	initial      string   // child name; empty for atomic/final/history
	parent       *stateNode[Ctx, Evt]
	children     map[string]*stateNode[Ctx, Evt]
	on           map[string][]TransitionConfig[Ctx, Evt]
	entryActions []Action[Ctx, Evt]
	exitActions  []Action[Ctx, Evt]
	onDone       []TransitionConfig[Ctx, Evt]
	history      History // valid when typ == NodeHistory
	defaultTgt   string  // valid when typ == NodeHistory
	// after holds this node's delayed transitions, sorted by delay ascending
	// (ties keep map-insertion-independent order via stable delay sort) so
	// timer arming is deterministic.
	after []afterEntry[Ctx, Evt]
	// invokes holds this node's invocations in declaration order.
	invokes []Invocation[Ctx, Evt]
	// outputFn builds the machine output when this final state completes at the
	// top level. nil unless typ == NodeFinal and an Output fn was configured.
	outputFn func(Ctx) any
}

// afterEntry is one delay bucket of a state's delayed transitions.
type afterEntry[Ctx any, Evt any] struct {
	delay       time.Duration
	transitions []TransitionConfig[Ctx, Evt]
}

// ID returns the machine's configured identifier.
func (m *Machine[Ctx, Evt]) ID() string { return m.id }

// initialContext returns a fresh copy of the configured starting context.
// Used by NewActor.
func (m *Machine[Ctx, Evt]) initialContext() Ctx { return m.context }

// initialValue returns the StateValue corresponding to the machine's
// initial state, recursively descending into the initial child of any
// compound nodes.
func (m *Machine[Ctx, Evt]) initialValue() StateValue {
	return m.root.initialValue()
}

func (n *stateNode[Ctx, Evt]) initialValue() StateValue {
	// Atomic, Final, and History states are leaves from the configuration's
	// perspective. (History should never actually appear in a committed
	// value — it is redirected at entry time — but returning a safe value
	// here protects against ill-formed configs.)
	if n.typ == NodeAtomic || n.typ == NodeFinal || n.typ == NodeHistory {
		return AtomicValue(n.name)
	}
	if n.typ == NodeParallel {
		return StateValue{Children: map[string]StateValue{n.name: n.initialInner()}}
	}
	if n.name == "" { // synthetic root
		child := n.children[n.initial]
		return child.initialValue()
	}
	child := n.children[n.initial]
	return StateValue{Children: map[string]StateValue{n.name: child.initialValue()}}
}

// initialInner returns the StateValue *inside* this node's wrap — i.e.,
// what would sit at a parent's Children[n.name] entry.
//
//   - Atomic / final / history: AtomicValue(n.name) — bare leaf string.
//   - Compound: the active child's initialValue() (which carries its own
//     self-wrap, becoming the next Children key as the structure stacks).
//   - Parallel: a multi-key Children map, one entry per region, value =
//     region.initialInner().
//
// Symmetric with initialValue: initialValue(n) returns
// `{Children: {n.name: n.initialInner()}}` for non-atomic nodes.
func (n *stateNode[Ctx, Evt]) initialInner() StateValue {
	if n.typ == NodeAtomic || n.typ == NodeFinal || n.typ == NodeHistory {
		return AtomicValue(n.name)
	}
	if n.typ == NodeParallel {
		regions := make(map[string]StateValue, len(n.children))
		for childName, child := range n.children {
			regions[childName] = child.initialInner()
		}
		return StateValue{Children: regions}
	}
	// Compound: descend into initial. The active child's initialValue()
	// includes its own self-wrap, which becomes the Children key at this
	// node's level.
	child := n.children[n.initial]
	if child == nil {
		return AtomicValue(n.name)
	}
	return child.initialValue()
}

// CreateMachine validates a MachineConfig and returns an immutable *Machine.
// Returns ErrInvalidConfig (with a descriptive wrapped error) for malformed
// configurations.
func CreateMachine[Ctx any, Evt any](cfg MachineConfig[Ctx, Evt]) (*Machine[Ctx, Evt], error) {
	if cfg.ID == "" {
		return nil, fmt.Errorf("%w: ID is required", ErrInvalidConfig)
	}
	if cfg.Initial == "" {
		return nil, fmt.Errorf("%w: machine has no Initial state", ErrNoInitial)
	}
	if _, ok := cfg.States[cfg.Initial]; !ok {
		return nil, fmt.Errorf("%w: initial state %q is not in States", ErrUnknownInitial, cfg.Initial)
	}

	root := &stateNode[Ctx, Evt]{
		name:     "",
		path:     nil,
		typ:      NodeCompound,
		initial:  cfg.Initial,
		children: make(map[string]*stateNode[Ctx, Evt], len(cfg.States)),
	}

	for name, child := range sortedStates(cfg.States) {
		built, err := buildNode[Ctx, Evt](name, child, root, []string{name})
		if err != nil {
			return nil, err
		}
		if _, dup := root.children[name]; dup {
			return nil, fmt.Errorf("%w: %q at root", ErrDuplicateState, name)
		}
		root.children[name] = built
	}

	// Second pass: validate every transition target resolves.
	if err := validateTargets[Ctx, Evt](root); err != nil {
		return nil, err
	}

	return &Machine[Ctx, Evt]{id: cfg.ID, context: cfg.Context, root: root}, nil
}

// buildNode recursively constructs the post-validation node tree.
func buildNode[Ctx any, Evt any](
	name string,
	cfg StateNodeConfig[Ctx, Evt],
	parent *stateNode[Ctx, Evt],
	path []string,
) (*stateNode[Ctx, Evt], error) {
	typ := cfg.Type
	if typ == NodeAtomic && len(cfg.States) > 0 {
		typ = NodeCompound
	}
	switch typ {
	case NodeAtomic, NodeCompound, NodeFinal, NodeHistory, NodeParallel:
		// OK as of P5.
	default:
		return nil, fmt.Errorf("%w: state %q has type %s", ErrInvalidNodeType, name, typ)
	}
	if typ == NodeFinal && len(cfg.States) > 0 {
		return nil, fmt.Errorf("%w: final state %q must not have nested States", ErrInvalidConfig, name)
	}
	if typ == NodeHistory && len(cfg.States) > 0 {
		return nil, fmt.Errorf("%w: history state %q must not have nested States", ErrInvalidConfig, name)
	}
	if typ == NodeParallel && len(cfg.States) == 0 {
		return nil, fmt.Errorf("%w: parallel state %q must declare at least one child region", ErrInvalidConfig, name)
	}
	if typ == NodeParallel && cfg.Initial != "" {
		return nil, fmt.Errorf("%w: parallel state %q must not declare Initial (all regions are active simultaneously)", ErrInvalidConfig, name)
	}

	node := &stateNode[Ctx, Evt]{
		name:         name,
		path:         append([]string(nil), path...),
		typ:          typ,
		initial:      cfg.Initial,
		parent:       parent,
		children:     make(map[string]*stateNode[Ctx, Evt], len(cfg.States)),
		on:           cfg.On,
		entryActions: cfg.Entry,
		exitActions:  cfg.Exit,
		onDone:       cfg.OnDone,
		history:      cfg.History,
		defaultTgt:   cfg.Default,
		after:        buildAfterEntries(cfg.After),
		invokes:      cfg.Invoke,
		outputFn:     cfg.Output,
	}

	if err := validateInvocations(strings.Join(path, "."), cfg.Invoke); err != nil {
		return nil, err
	}

	if typ == NodeCompound {
		if cfg.Initial == "" {
			return nil, fmt.Errorf("%w: state %q", ErrNoInitial, strings.Join(path, "."))
		}
		if _, ok := cfg.States[cfg.Initial]; !ok {
			return nil, fmt.Errorf("%w: state %q initial %q", ErrUnknownInitial, strings.Join(path, "."), cfg.Initial)
		}
	}
	if typ == NodeCompound || typ == NodeParallel {
		for childName, child := range sortedStates(cfg.States) {
			builtChild, err := buildNode[Ctx, Evt](childName, child, node, append(append([]string(nil), path...), childName))
			if err != nil {
				return nil, err
			}
			if _, dup := node.children[childName]; dup {
				return nil, fmt.Errorf("%w: %q in %q", ErrDuplicateState, childName, strings.Join(path, "."))
			}
			node.children[childName] = builtChild
		}
	}

	return node, nil
}

// buildAfterEntries converts a config's delay→transitions map into a
// delay-sorted slice, so timer arming order is deterministic regardless of Go
// map iteration order. Returns nil for an empty/absent map.
func buildAfterEntries[Ctx any, Evt any](m map[time.Duration][]TransitionConfig[Ctx, Evt]) []afterEntry[Ctx, Evt] {
	if len(m) == 0 {
		return nil
	}
	delays := make([]time.Duration, 0, len(m))
	for d := range m {
		delays = append(delays, d)
	}
	sort.Slice(delays, func(i, j int) bool { return delays[i] < delays[j] })
	out := make([]afterEntry[Ctx, Evt], 0, len(delays))
	for _, d := range delays {
		out = append(out, afterEntry[Ctx, Evt]{delay: d, transitions: m[d]})
	}
	return out
}

// validateInvocations checks a state's invocations: each must have a non-empty
// Src and a unique, non-empty ID within the state.
func validateInvocations[Ctx any, Evt any](statePath string, invs []Invocation[Ctx, Evt]) error {
	if len(invs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(invs))
	for i, inv := range invs {
		if inv.ID == "" {
			return fmt.Errorf("%w: state %q invoke %d has empty ID", ErrInvalidConfig, statePath, i)
		}
		if inv.Src == "" {
			return fmt.Errorf("%w: state %q invoke %q has empty Src", ErrInvalidConfig, statePath, inv.ID)
		}
		if _, dup := seen[inv.ID]; dup {
			return fmt.Errorf("%w: state %q has duplicate invoke ID %q", ErrInvalidConfig, statePath, inv.ID)
		}
		seen[inv.ID] = struct{}{}
	}
	return nil
}

// validateTargets walks every node and confirms each transition's Target
// resolves to a known node. Targets are resolved relative to the node's
// siblings first, then descendants of the same parent.
func validateTargets[Ctx any, Evt any](root *stateNode[Ctx, Evt]) error {
	var walk func(n *stateNode[Ctx, Evt]) error
	walk = func(n *stateNode[Ctx, Evt]) error {
		for event, transitions := range n.on {
			for i, t := range transitions {
				if t.Target == "" {
					continue // internal / no-op
				}
				if resolveTarget(n, t.Target) == nil {
					return fmt.Errorf("%w: state %q event %q candidate %d target %q",
						ErrUnknownTarget, strings.Join(n.path, "."), event, i, t.Target)
				}
			}
		}
		for _, ae := range n.after {
			for i, t := range ae.transitions {
				if t.Target == "" {
					continue // internal / no-op
				}
				if resolveTarget(n, t.Target) == nil {
					return fmt.Errorf("%w: state %q after %s candidate %d target %q",
						ErrUnknownTarget, strings.Join(n.path, "."), ae.delay, i, t.Target)
				}
			}
		}
		for _, child := range n.children {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(root)
}

// resolveTarget looks up a target name relative to the source node.
//
// Resolution order (matching XState v5 semantics for unqualified targets):
//  1. Among the source's own children (a target that descends into source).
//  2. Among siblings of the source, then siblings of each ancestor in turn —
//     walk up the chain looking for a node whose name matches the target's
//     first segment. This lets a deeply-nested state target an ancestor's
//     sibling without writing the full path.
//  3. Absolute lookup from the machine root.
//
// Returns nil if not found.
func resolveTarget[Ctx any, Evt any](source *stateNode[Ctx, Evt], target string) *stateNode[Ctx, Evt] {
	head, tail, _ := strings.Cut(target, ".")

	// 1) descend into a child of the source itself.
	if child, ok := source.children[head]; ok {
		return descend(child, tail)
	}

	// 2) walk up ancestors looking for a sibling match.
	for cursor := source.parent; cursor != nil; cursor = cursor.parent {
		if sibling, ok := cursor.children[head]; ok {
			return descend(sibling, tail)
		}
	}

	// 3) absolute lookup from the machine root.
	root := source
	for root.parent != nil {
		root = root.parent
	}
	if abs, ok := root.children[head]; ok {
		return descend(abs, tail)
	}
	return nil
}

// descend follows a dotted path of immediate child names. An empty path
// returns the node itself.
func descend[Ctx any, Evt any](n *stateNode[Ctx, Evt], path string) *stateNode[Ctx, Evt] {
	if path == "" {
		return n
	}
	head, tail, _ := strings.Cut(path, ".")
	child, ok := n.children[head]
	if !ok {
		return nil
	}
	return descend(child, tail)
}

// sortedStates iterates a state map in deterministic (alphabetical) order.
// Required for ADR-002 determinism in any code path that observes ordering.
func sortedStates[Ctx any, Evt any](m map[string]StateNodeConfig[Ctx, Evt]) func(yield func(string, StateNodeConfig[Ctx, Evt]) bool) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return func(yield func(string, StateNodeConfig[Ctx, Evt]) bool) {
		for _, k := range keys {
			if !yield(k, m[k]) {
				return
			}
		}
	}
}
