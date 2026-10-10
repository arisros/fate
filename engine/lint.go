package engine

import (
	"cmp"
	"maps"
	"slices"
	"strings"
)

// FindingKind names what Lint found.
type FindingKind string

// The kinds of Finding that Lint reports.
const (
	// FindingUnreachable is a state no initial chain or transition enters.
	FindingUnreachable FindingKind = "unreachable"
	// FindingDeadEnd is a state that is not final and that no transition, on
	// it or on an ancestor, leaves.
	FindingDeadEnd FindingKind = "dead_end"
	// FindingOnDoneNeverFires is a state that declares OnDone and can never
	// complete: a compound state with no final child, or a parallel state with
	// a region that cannot complete.
	FindingOnDoneNeverFires FindingKind = "on_done_never_fires"
	// FindingShadowedTransition is a transition that can never fire, because
	// a candidate before it for the same trigger has no Guard or Cond.
	FindingShadowedTransition FindingKind = "shadowed_transition"
)

// Finding is one problem Lint found in a machine.
type Finding struct {
	Kind FindingKind `json:"kind"`
	// State is the dot path of the state the finding is about.
	State   string `json:"state"`
	Message string `json:"message"`
}

// Lint reports states that are legal but probably mistakes: unreachable
// states, dead ends, OnDone transitions that can never fire, and transitions
// shadowed by an unconditional one before them. CreateMachine
// accepts all of them, so call Lint from a test to keep a machine clean.
//
// The analysis reads structure only. Guards are assumed able to pass, so a
// state behind a guard that is never true is still counted as reachable.
// Findings are sorted by state path, then kind; a clean machine returns nil.
func (m *Machine[Ctx, Evt]) Lint() []Finding {
	reached := reachableStates(m.root)
	var out []Finding
	var walk func(n *stateNode[Ctx, Evt])
	walk = func(n *stateNode[Ctx, Evt]) {
		if n.name != "" {
			if !reached[n] {
				out = append(out, Finding{FindingUnreachable, strings.Join(n.path, "."), "no initial chain or transition enters this state"})
				return
			}
			if n.typ == NodeAtomic && !hasWayOut(n) {
				out = append(out, Finding{FindingDeadEnd, strings.Join(n.path, "."), "not a final state, and no transition on it or an ancestor leaves it"})
			}
			if len(n.onDone) > 0 && !canComplete(n) {
				out = append(out, Finding{FindingOnDoneNeverFires, strings.Join(n.path, "."), "declares OnDone but can never complete"})
			}
			for _, trigger := range shadowedTriggers(n) {
				out = append(out, Finding{FindingShadowedTransition, strings.Join(n.path, "."), trigger + " lists a transition after one with no Guard or Cond, so it can never fire"})
			}
		}
		for _, name := range slices.Sorted(maps.Keys(n.children)) {
			walk(n.children[name])
		}
	}
	walk(m.root)
	slices.SortFunc(out, func(a, b Finding) int {
		return cmp.Or(cmp.Compare(a.State, b.State), cmp.Compare(a.Kind, b.Kind))
	})
	return out
}

// reachableStates returns every state some initial chain or transition can
// enter, ignoring guards.
func reachableStates[Ctx any, Evt any](root *stateNode[Ctx, Evt]) map[*stateNode[Ctx, Evt]]bool {
	reached := map[*stateNode[Ctx, Evt]]bool{}
	var pending []*stateNode[Ctx, Evt]
	mark := func(n *stateNode[Ctx, Evt]) bool {
		if n == nil || n.name == "" || reached[n] {
			return false
		}
		reached[n] = true
		pending = append(pending, n)
		return true
	}
	var enter func(n *stateNode[Ctx, Evt])
	enter = func(n *stateNode[Ctx, Evt]) {
		if !mark(n) {
			return
		}
		switch n.typ {
		case NodeCompound:
			enter(n.children[n.initial])
		case NodeParallel:
			for _, region := range n.children {
				enter(region)
			}
		case NodeHistory:
			if n.defaultTgt != "" {
				enter(resolveTarget(n, n.defaultTgt))
			} else if n.parent != nil {
				enter(n.parent.children[n.parent.initial])
			}
		}
	}
	// target enters the state a transition names: the state itself with its
	// initial content, its ancestors, and the other regions of any parallel
	// ancestor.
	target := func(n *stateNode[Ctx, Evt]) {
		enter(n)
		for a := n.parent; a != nil && a.name != ""; a = a.parent {
			if a.typ == NodeParallel {
				enter(a)
			} else {
				mark(a)
			}
		}
	}
	enter(root.children[root.initial])
	for len(pending) > 0 {
		n := pending[0]
		pending = pending[1:]
		for _, t := range allTransitions(n) {
			if t.Target != "" {
				if tgt := resolveTarget(n, t.Target); tgt != nil {
					target(tgt)
				}
			}
		}
	}
	return reached
}

// firstUnconditional returns the index of the first candidate with no Guard
// and no Cond, or -1. Every candidate after it can never fire.
func firstUnconditional[Ctx any, Evt any](ts []TransitionConfig[Ctx, Evt]) int {
	return slices.IndexFunc(ts, func(t TransitionConfig[Ctx, Evt]) bool { return t.Guard == nil && t.Cond == nil })
}

// shadowedTriggers names, in a stable order, the triggers of n whose candidate
// list continues past an unconditional transition.
func shadowedTriggers[Ctx any, Evt any](n *stateNode[Ctx, Evt]) []string {
	shadowed := func(ts []TransitionConfig[Ctx, Evt]) bool {
		open := firstUnconditional(ts)
		return open >= 0 && open < len(ts)-1
	}
	var out []string
	for _, event := range slices.Sorted(maps.Keys(n.on)) {
		if shadowed(n.on[event]) {
			out = append(out, "event "+event)
		}
	}
	for _, ae := range n.after {
		if shadowed(ae.transitions) {
			out = append(out, "After "+ae.delay.String())
		}
	}
	if shadowed(n.onDone) {
		out = append(out, "OnDone")
	}
	return out
}

func allTransitions[Ctx any, Evt any](n *stateNode[Ctx, Evt]) []TransitionConfig[Ctx, Evt] {
	out := slices.Clone(n.onDone)
	for _, ts := range n.on {
		out = append(out, ts...)
	}
	for _, ae := range n.after {
		out = append(out, ae.transitions...)
	}
	return out
}

// hasWayOut reports whether a transition on n or on an ancestor leaves n.
// OnDone transitions of an ancestor count, since completing it exits n.
func hasWayOut[Ctx any, Evt any](n *stateNode[Ctx, Evt]) bool {
	for cursor := n; cursor != nil && cursor.name != ""; cursor = cursor.parent {
		for _, t := range allTransitions(cursor) {
			if t.Target != "" {
				return true
			}
		}
	}
	return false
}

// canComplete reports whether n can reach the done condition OnDone waits for.
func canComplete[Ctx any, Evt any](n *stateNode[Ctx, Evt]) bool {
	switch n.typ {
	case NodeCompound:
		for _, child := range n.children {
			if child.typ == NodeFinal {
				return true
			}
		}
	case NodeParallel:
		for _, region := range n.children {
			if region.typ != NodeFinal && !canComplete(region) {
				return false
			}
		}
		return len(n.children) > 0
	}
	return false
}
