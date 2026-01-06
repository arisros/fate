package fate

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// MachineConfig declares an immutable statechart.
type MachineConfig[Ctx any, Evt any] struct {
	ID string

	Initial string

	Context Ctx

	States map[string]StateNodeConfig[Ctx, Evt]
}

// StateNodeConfig declares one state node within a machine.
type StateNodeConfig[Ctx any, Evt any] struct {
	Type NodeType

	Initial string

	States map[string]StateNodeConfig[Ctx, Evt]

	On map[string][]TransitionConfig[Ctx, Evt]

	After map[time.Duration][]TransitionConfig[Ctx, Evt]

	Invoke []Invocation[Ctx, Evt]

	Entry []Action[Ctx, Evt]

	Exit []Action[Ctx, Evt]

	OnDone []TransitionConfig[Ctx, Evt]

	History History

	Default string

	Output func(ctx Ctx) any
}

// TransitionConfig declares one possible transition for an event.
type TransitionConfig[Ctx any, Evt any] struct {
	Target string

	Internal bool

	Guard Guard[Ctx, Evt]

	Cond Cond

	Actions []Action[Ctx, Evt]
}

// Machine is an immutable, validated statechart.
type Machine[Ctx any, Evt any] struct {
	id      string
	context Ctx
	root    *stateNode[Ctx, Evt]
}

type stateNode[Ctx any, Evt any] struct {
	name         string
	path         []string
	typ          NodeType
	initial      string
	parent       *stateNode[Ctx, Evt]
	children     map[string]*stateNode[Ctx, Evt]
	on           map[string][]TransitionConfig[Ctx, Evt]
	entryActions []Action[Ctx, Evt]
	exitActions  []Action[Ctx, Evt]
	onDone       []TransitionConfig[Ctx, Evt]
	history      History
	defaultTgt   string
	after        []afterEntry[Ctx, Evt]
	invokes      []Invocation[Ctx, Evt]
	outputFn     func(Ctx) any
}

type afterEntry[Ctx any, Evt any] struct {
	delay       time.Duration
	transitions []TransitionConfig[Ctx, Evt]
}

// ID returns the machine's configured identifier.
func (m *Machine[Ctx, Evt]) ID() string { return m.id }

func (m *Machine[Ctx, Evt]) initialContext() Ctx { return m.context }

func (m *Machine[Ctx, Evt]) initialValue() StateValue {
	return m.root.initialValue()
}

func (n *stateNode[Ctx, Evt]) initialValue() StateValue {
	if n.typ == NodeAtomic || n.typ == NodeFinal || n.typ == NodeHistory {
		return AtomicValue(n.name)
	}
	if n.typ == NodeParallel {
		return StateValue{Children: map[string]StateValue{n.name: n.initialInner()}}
	}
	if n.name == "" {
		child := n.children[n.initial]
		return child.initialValue()
	}
	child := n.children[n.initial]
	return StateValue{Children: map[string]StateValue{n.name: child.initialValue()}}
}

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
	child := n.children[n.initial]
	if child == nil {
		return AtomicValue(n.name)
	}
	return child.initialValue()
}

// CreateMachine validates a MachineConfig and returns an immutable *Machine.
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

	if err := validateTargets[Ctx, Evt](root); err != nil {
		return nil, err
	}

	return &Machine[Ctx, Evt]{id: cfg.ID, context: cfg.Context, root: root}, nil
}

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

func validateTargets[Ctx any, Evt any](root *stateNode[Ctx, Evt]) error {
	var walk func(n *stateNode[Ctx, Evt]) error
	walk = func(n *stateNode[Ctx, Evt]) error {
		for event, transitions := range n.on {
			for i, t := range transitions {
				if t.Target == "" {
					continue
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
					continue
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

func resolveTarget[Ctx any, Evt any](source *stateNode[Ctx, Evt], target string) *stateNode[Ctx, Evt] {
	head, tail, _ := strings.Cut(target, ".")

	if child, ok := source.children[head]; ok {
		return descend(child, tail)
	}

	for cursor := source.parent; cursor != nil; cursor = cursor.parent {
		if sibling, ok := cursor.children[head]; ok {
			return descend(sibling, tail)
		}
	}

	root := source
	for root.parent != nil {
		root = root.parent
	}
	if abs, ok := root.children[head]; ok {
		return descend(abs, tail)
	}
	return nil
}

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
